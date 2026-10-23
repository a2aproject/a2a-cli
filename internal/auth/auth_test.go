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

package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/a2aproject/a2a-cli/internal/channel"
	"github.com/a2aproject/a2a-cli/internal/network"
	"github.com/a2aproject/a2a-cli/internal/store"
)

func openStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "server.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return s
}

type fakeNet struct {
	network.Provider
	peers map[string]*network.Peer
}

func (f fakeNet) WhoIs(_ context.Context, addr string) (*network.Peer, error) {
	if p, ok := f.peers[addr]; ok {
		return p, nil
	}
	return nil, network.ErrNoIdentity
}

func header(kv ...string) http.Header {
	h := http.Header{}
	for i := 0; i+1 < len(kv); i += 2 {
		h.Set(kv[i], kv[i+1])
	}
	return h
}

func TestChainAuthenticate(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	ctx := context.Background()
	code, _, err := MintCode(ctx, s, PurposePair, "grok", RoleMember)
	if err != nil {
		t.Fatalf("MintCode() error = %v", err)
	}
	c, err := s.TakeCode(ctx, NormalizeCode(strings.ToLower(code)), PurposePair)
	if err != nil {
		t.Fatalf("TakeCode() error = %v", err)
	}
	token, _, err := IssueToken(ctx, s, c, "https://agent")
	if err != nil {
		t.Fatalf("IssueToken() error = %v", err)
	}

	chain := &Chain{
		Store:      s,
		Identities: map[string]string{"anna@example.com": RoleMember},
		Net: fakeNet{peers: map[string]*network.Peer{
			"100.1.1.1:5000": {ID: "me@example.com", Self: true},
			"100.1.1.2:5000": {ID: "anna@example.com"},
			"100.1.1.3:5000": {ID: "bob@example.com", Caps: map[string][]json.RawMessage{TailscaleCap: {json.RawMessage(`{"role":"member"}`)}}},
			"100.1.1.4:5000": {ID: "eve@example.com"},
		}},
	}
	tests := []struct {
		name   string
		req    Request
		want   Principal
		wantOK bool
	}{
		{
			name:   "tailnet self",
			req:    Request{Listener: network.ListenerPrivate, RemoteAddr: "100.1.1.1:5000", Header: header(HeaderOnBehalfOf, "user:owner", HeaderHops, "1")},
			want:   Principal{ID: OwnerID, Roles: []string{RoleOwner}, OnBehalfOf: "user:owner", Hops: 1},
			wantOK: true,
		},
		{
			name:   "tailnet identity from config",
			req:    Request{Listener: network.ListenerPrivate, RemoteAddr: "100.1.1.2:5000", Header: http.Header{}},
			want:   Principal{ID: "user:anna@example.com", Roles: []string{RoleMember}},
			wantOK: true,
		},
		{
			name:   "tailnet capability",
			req:    Request{Listener: network.ListenerPrivate, RemoteAddr: "100.1.1.3:5000", Header: http.Header{}},
			want:   Principal{ID: "user:bob@example.com", Roles: []string{RoleMember}},
			wantOK: true,
		},
		{
			name:   "on-behalf-of ignored from other tailnet users",
			req:    Request{Listener: network.ListenerPrivate, RemoteAddr: "100.1.1.2:5000", Header: header(HeaderOnBehalfOf, "user:owner", HeaderHops, "2")},
			want:   Principal{ID: "user:anna@example.com", Roles: []string{RoleMember}, Hops: 2},
			wantOK: true,
		},
		{
			name:   "negative hops ignored",
			req:    Request{Listener: network.ListenerPrivate, RemoteAddr: "100.1.1.1:5000", Header: header(HeaderHops, "-1000")},
			want:   Principal{ID: OwnerID, Roles: []string{RoleOwner}},
			wantOK: true,
		},
		{
			name: "unknown tailnet user",
			req:  Request{Listener: network.ListenerPrivate, RemoteAddr: "100.1.1.4:5000", Header: http.Header{}},
		},
		{
			name: "network identity ignored on public listener",
			req:  Request{Listener: network.ListenerPublic, RemoteAddr: "100.1.1.1:5000", Header: http.Header{}},
		},
		{
			name:   "pairing token",
			req:    Request{Listener: network.ListenerPublic, Header: header("Authorization", "Bearer "+token, HeaderOnBehalfOf, "user:owner")},
			want:   Principal{ID: "pair:grok", Roles: []string{RoleMember}},
			wantOK: true,
		},
		{
			name: "unknown token",
			req:  Request{Listener: network.ListenerLocal, Header: http.Header{"Authorization": {"Bearer a2p_nope"}}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok, err := chain.Authenticate(ctx, tc.req)
			if err != nil {
				t.Fatalf("Authenticate() error = %v", err)
			}
			if ok != tc.wantOK {
				t.Fatalf("Authenticate() ok = %v, want %v", ok, tc.wantOK)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("Authenticate() wrong result (-want +got) diff = %s", diff)
			}
		})
	}

}

func TestLink(t *testing.T) {
	t.Parallel()
	s := openStore(t)
	ctx := context.Background()
	chain := &Chain{Store: s}
	in := &channel.Inbound{Channel: "telegram", Identity: "tg:42", Conv: "42", Direct: true, Authentic: true}

	if _, ok, err := chain.Authenticate(ctx, Request{Channel: in}); err != nil || ok {
		t.Fatalf("Authenticate() before link = %v, %v, want false, nil", ok, err)
	}
	code, _, err := MintCode(ctx, s, PurposePair, "owner", RoleOwner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Link(ctx, s, &channel.Inbound{Channel: "telegram", Identity: "tg:43"}, code); err == nil {
		t.Fatalf("Link() unauthentic sender error = nil, want error")
	}
	p, err := Link(ctx, s, in, code)
	if err != nil {
		t.Fatalf("Link() error = %v", err)
	}
	if p.ID != OwnerID {
		t.Fatalf("Link() = %v, want %v", p.ID, OwnerID)
	}
	got, ok, err := chain.Authenticate(ctx, Request{Channel: in})
	if err != nil || !ok {
		t.Fatalf("Authenticate() after link = %v, %v, want true, nil", ok, err)
	}
	if diff := cmp.Diff(Owner(), got); diff != "" {
		t.Fatalf("Authenticate() wrong result (-want +got) diff = %s", diff)
	}
	links, err := s.LinksOf(ctx, OwnerID)
	if err != nil || len(links) != 1 || links[0].Conv != "42" {
		t.Fatalf("LinksOf() = %v, %v, want one link with conv 42", links, err)
	}
}

func TestRevokeRemovesLinksOfPairing(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		role string
	}{
		{name: "member", role: RoleMember},
		{name: "owner", role: RoleOwner},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := openStore(t)
			ctx := context.Background()
			chain := &Chain{Store: s}
			in := &channel.Inbound{Channel: "telegram", Identity: "tg:42", Conv: "42", Direct: true, Authentic: true}
			code, _, err := MintCode(ctx, s, PurposePair, "phone", tc.role)
			if err != nil {
				t.Fatalf("MintCode() error = %v", err)
			}
			if _, err := Link(ctx, s, in, code); err != nil {
				t.Fatalf("Link() error = %v", err)
			}

			n, err := s.RevokeTokens(ctx, "phone", PrincipalForCode("phone", RoleMember))
			if err != nil {
				t.Fatalf("RevokeTokens() error = %v", err)
			}
			if n != 1 {
				t.Fatalf("RevokeTokens() = %d, want 1", n)
			}
			if _, ok, err := chain.Authenticate(ctx, Request{Channel: in}); err != nil || ok {
				t.Fatalf("Authenticate() after revoke = %v, %v, want false, nil", ok, err)
			}
		})
	}
}

func TestLinkCode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		text   string
		want   string
		wantOK bool
	}{
		{text: "link K7Q2-9XF4", want: "K7Q2-9XF4", wantOK: true},
		{text: "/start K7Q29XF4", want: "K7Q29XF4", wantOK: true},
		{text: "/link@my_bot k7q2-9xf4", want: "k7q2-9xf4", wantOK: true},
		{text: "please link K7Q2-9XF4 now"},
		{text: "/start"},
	}
	for _, tc := range tests {
		got, ok := LinkCode(tc.text)
		if ok != tc.wantOK || got != tc.want {
			t.Errorf("LinkCode(%q) = %q, %v, want %q, %v", tc.text, got, ok, tc.want, tc.wantOK)
		}
	}
}
