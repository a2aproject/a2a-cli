// Copyright 2026 The A2A Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package discover

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestAddressClassification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		addr         string
		wantBlocked  string
		wantInternal bool
	}{
		{addr: "127.0.0.1", wantInternal: true},
		{addr: "::1", wantInternal: true},
		{addr: "10.1.2.3", wantInternal: true},
		{addr: "192.168.0.1", wantInternal: true},
		{addr: "fd00::1", wantInternal: true},
		{addr: "100.100.1.1", wantInternal: true},
		{addr: "169.254.169.254", wantBlocked: "link-local address"},
		{addr: "::ffff:169.254.169.254", wantBlocked: "link-local address"},
		{addr: "fe80::1", wantBlocked: "link-local address"},
		{addr: "0.0.0.0", wantBlocked: "unspecified address"},
		{addr: "224.0.0.1", wantBlocked: "link-local address"},
		{addr: "239.1.1.1", wantBlocked: "multicast address"},
		{addr: "8.8.8.8"},
		{addr: "2001:4860:4860::8888"},
	}

	for _, tc := range tests {
		t.Run(tc.addr, func(t *testing.T) {
			t.Parallel()
			a := netip.MustParseAddr(tc.addr)
			if got := blockedReason(a); got != tc.wantBlocked {
				t.Fatalf("blockedReason(%s) = %q, want %q", tc.addr, got, tc.wantBlocked)
			}
			if got := isInternal(a); got != tc.wantInternal {
				t.Fatalf("isInternal(%s) = %v, want %v", tc.addr, got, tc.wantInternal)
			}
		})
	}
}

var testHosts = map[string]string{
	"public.example":   "93.184.216.34",
	"public2.example":  "93.184.216.35",
	"internal.example": "10.0.0.5",
	"metadata.example": "169.254.169.254",
	"mixed.example":    "93.184.216.36,10.0.0.6",
}

// fakeGuard resolves testHosts and records dialed addresses without opening connections.
func fakeGuard(allowHosts ...string) (Guard, *[]string) {
	var dialed []string
	g := Guard{
		AllowHosts: allowHosts,
		lookup: func(_ context.Context, host string) ([]netip.Addr, error) {
			ips, ok := testHosts[host]
			if !ok {
				return nil, fmt.Errorf("no such host %s", host)
			}
			var addrs []netip.Addr
			for ip := range strings.SplitSeq(ips, ",") {
				addrs = append(addrs, netip.MustParseAddr(ip))
			}
			return addrs, nil
		},
		dial: func(_ context.Context, _, addr string) (net.Conn, error) {
			dialed = append(dialed, addr)
			client, server := net.Pipe()
			if err := server.Close(); err != nil {
				return nil, err
			}
			return client, nil
		},
	}
	return g, &dialed
}

func TestGuardDialContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		allowHosts []string
		chain      bool
		hosts      []string
		wantDialed []string
		wantReason string
	}{
		{
			name:       "internal host without redirects",
			chain:      true,
			hosts:      []string{"internal.example"},
			wantDialed: []string{"10.0.0.5:80"},
		},
		{
			name:       "internal redirects to public",
			chain:      true,
			hosts:      []string{"internal.example", "public.example"},
			wantDialed: []string{"10.0.0.5:80", "93.184.216.34:80"},
		},
		{
			name:       "public redirects to public",
			chain:      true,
			hosts:      []string{"public.example", "public2.example"},
			wantDialed: []string{"93.184.216.34:80", "93.184.216.35:80"},
		},
		{
			name:       "public redirects to internal",
			chain:      true,
			hosts:      []string{"public.example", "internal.example"},
			wantDialed: []string{"93.184.216.34:80"},
			wantReason: "redirect from a public one",
		},
		{
			name:       "public redirects to internal literal",
			chain:      true,
			hosts:      []string{"public.example", "127.0.0.1"},
			wantDialed: []string{"93.184.216.34:80"},
			wantReason: "redirect from a public one",
		},
		{
			name:       "host with public and internal addresses redirects to internal",
			chain:      true,
			hosts:      []string{"mixed.example", "internal.example"},
			wantDialed: []string{"93.184.216.36:80"},
			wantReason: "redirect from a public one",
		},
		{
			name:       "public redirects to host with public and internal addresses",
			chain:      true,
			hosts:      []string{"public.example", "mixed.example"},
			wantDialed: []string{"93.184.216.34:80"},
			wantReason: "redirect from a public one",
		},
		{
			name:       "separate requests are not a chain",
			hosts:      []string{"public.example", "internal.example"},
			wantDialed: []string{"93.184.216.34:80", "10.0.0.5:80"},
		},
		{
			name:       "link-local refused",
			chain:      true,
			hosts:      []string{"metadata.example"},
			wantReason: "link-local address",
		},
		{
			name:       "allowed host admitted after public",
			allowHosts: []string{"INTERNAL.example"},
			chain:      true,
			hosts:      []string{"public.example", "internal.example"},
			wantDialed: []string{"93.184.216.34:80", "internal.example:80"},
		},
		{
			name:       "allowed link-local host admitted",
			allowHosts: []string{"metadata.example"},
			chain:      true,
			hosts:      []string{"metadata.example"},
			wantDialed: []string{"metadata.example:80"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, dialed := fakeGuard(tc.allowHosts...)
			ctx := t.Context()
			if tc.chain {
				ctx = withRedirectChain(ctx)
			}

			var gotErr error
			for _, host := range tc.hosts {
				conn, err := g.DialContext(ctx, "tcp", net.JoinHostPort(host, "80"))
				if err != nil {
					gotErr = err
					break
				}
				if err := conn.Close(); err != nil {
					t.Fatalf("Conn.Close() error = %v", err)
				}
			}

			if diff := cmp.Diff(tc.wantDialed, *dialed); diff != "" {
				t.Fatalf("Guard.DialContext() dialed wrong addresses (-want +got) diff = %s", diff)
			}
			if tc.wantReason == "" {
				if gotErr != nil {
					t.Fatalf("Guard.DialContext() error = %v, want nil", gotErr)
				}
				return
			}
			var blocked *BlockedError
			if !errors.As(gotErr, &blocked) || !strings.Contains(blocked.Reason, tc.wantReason) {
				t.Fatalf("Guard.DialContext() error = %v, want BlockedError containing %q", gotErr, tc.wantReason)
			}
		})
	}
}
