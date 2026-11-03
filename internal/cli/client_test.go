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

package cli

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/grpc"

	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-cli/internal/localsrv"
	"github.com/a2aproject/a2a-cli/internal/testutil"
	"github.com/a2aproject/a2a-go/v2/a2a"
	a2agrpcv0 "github.com/a2aproject/a2a-go/v2/a2agrpc/v0"
	a2agrpc "github.com/a2aproject/a2a-go/v2/a2agrpc/v1"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

func TestGRPCTransportSecurity(t *testing.T) {
	t.Parallel()
	handler := a2asrv.NewHandler(localsrv.NewEchoExecutor())
	srv := grpc.NewServer()
	t.Cleanup(srv.Stop)
	a2agrpc.NewHandler(handler).RegisterWith(srv)
	a2agrpcv0.NewHandler(handler).RegisterWith(srv)

	secured := httptest.NewUnstartedServer(srv)
	secured.EnableHTTP2 = true
	secured.StartTLS()
	t.Cleanup(secured.Close)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	go func() {
		if err := srv.Serve(lis); err != nil {
			t.Errorf("srv.Serve() error = %v", err)
		}
	}()
	plainAddr := lis.Addr().String()

	caCert := testutil.MustWriteTmpCertFile(t, secured.Certificate())
	wrongHost := fmt.Sprintf("passthrough:///localhost:%d", secured.Listener.Addr().(*net.TCPAddr).Port)
	cardPath := func(address string) string {
		return testutil.MustWriteTmpCardFile(t, &a2a.AgentCard{
			Name: "gRPC Echo", Version: "1.0.0",
			SupportedInterfaces: []*a2a.AgentInterface{a2a.NewAgentInterface(address, a2a.TransportProtocolGRPC)},
		})
	}

	tests := []struct {
		name     string
		args     []string
		wantErr  string
		wantHint string
	}{
		{name: "TLS endpoint", args: []string{"--endpoint", secured.URL, "--ca-cert", caCert}},
		{name: "TLS Agent Card", args: []string{"--agent-card", cardPath(secured.URL), "--ca-cert", caCert}},
		{name: "TLS 0.3 endpoint", args: []string{"--endpoint", secured.URL, "--a2a-version", "0.3", "--ca-cert", caCert}},
		{name: "TLS Agent Card with bare address", args: []string{"--agent-card", cardPath(secured.Listener.Addr().String()), "--ca-cert", caCert}},
		{name: "plaintext Agent Card", args: []string{"--agent-card", cardPath(plainAddr), "--insecure"}},
		{name: "explicit plaintext", args: []string{"--endpoint", "http://" + plainAddr, "--insecure"}},
		{name: "explicit plaintext with a CA certificate", args: []string{"--endpoint", "http://" + plainAddr, "--insecure", "--ca-cert", caCert}},
		{name: "untrusted certificate", args: []string{"--endpoint", secured.URL}, wantErr: "certificate"},
		{name: "mismatched hostname", args: []string{"--endpoint", wrongHost, "--ca-cert", caCert}, wantErr: "certificate"},
		{name: "plaintext server without --insecure", args: []string{"--endpoint", "http://" + plainAddr}, wantErr: "TLS handshake", wantHint: "--insecure"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			args := append([]string{"send", "--transport", "grpc", "--timeout", "3s", "-o", "json", "hello"}, tc.args...)
			out, err := runCMD(t, args...)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("a2a %s error = %v, want %q", strings.Join(args, " "), err, tc.wantErr)
				}
				if hint := clierr.Classify(err).Hint; !strings.Contains(hint, tc.wantHint) {
					t.Fatalf("a2a %s hint = %q, want %q", strings.Join(args, " "), hint, tc.wantHint)
				}
				return
			}
			if err != nil {
				t.Fatalf("a2a %s error = %v", strings.Join(args, " "), err)
			}
			if got := testutil.AllArtifactText(mustDecodeTask(t, out)); got != "hello" {
				t.Fatalf("a2a %s artifact text = %q, want %q", strings.Join(args, " "), got, "hello")
			}
		})
	}
}

func TestCACert(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.Handle("/", a2asrv.NewRESTHandler(a2asrv.NewHandler(localsrv.NewEchoExecutor())))
	server := httptest.NewTLSServer(mux)
	t.Cleanup(server.Close)
	mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(newAgentCard(server.URL, a2a.AgentCapabilities{})))
	caCert := testutil.MustWriteTmpCertFile(t, server.Certificate())

	// t.TempDir paths can exceed the Unix socket path length limit.
	dir, err := os.MkdirTemp("", "a2a")
	if err != nil {
		t.Fatalf("os.MkdirTemp() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("removing %s: %v", dir, err)
		}
	})
	sock := filepath.Join(dir, "a2a.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("net.Listen(%q) error = %v", sock, err)
	}
	overSocket := &httptest.Server{Listener: ln, Config: &http.Server{Handler: mux}}
	overSocket.StartTLS()
	t.Cleanup(overSocket.Close)

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "send with Agent Card", args: []string{"send", "--agent-card", server.URL, "--ca-cert", caCert, "hello"}},
		{name: "send to endpoint", args: []string{"send", "--endpoint", server.URL, "--transport", "rest", "--ca-cert", caCert, "hello"}},
		{name: "card get", args: []string{"card", "get", server.URL, "--ca-cert", caCert}},
		{name: "send over a Unix socket", args: []string{"send", "--unix-socket", sock, "--agent-card", "https://127.0.0.1", "--ca-cert", caCert, "hello"}},
		{name: "untrusted certificate", args: []string{"send", "--agent-card", server.URL, "hello"}, wantErr: "certificate"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runCMD(t, tc.args...)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("a2a %s error = %v, want %q", strings.Join(tc.args, " "), err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("a2a %s error = %v", strings.Join(tc.args, " "), err)
			}
		})
	}
}

func TestCACertFile(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "missing.pem")
	empty := testutil.MustWriteTmpCertFile(t)
	tests := []struct {
		name         string
		args         []string
		wantUsageErr bool
	}{
		{name: "send with a missing file", args: []string{"send", "-a", "https://agent.example.com", "--ca-cert", missing, "hello"}, wantUsageErr: true},
		{name: "send with no certificates", args: []string{"send", "-a", "https://agent.example.com", "--ca-cert", empty, "hello"}, wantUsageErr: true},
		{name: "card get with a missing file", args: []string{"card", "get", "https://agent.example.com", "--ca-cert", missing}, wantUsageErr: true},
		{name: "config show with a missing file", args: []string{"config", "show", "--ca-cert", missing}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runCMD(t, tc.args...)
			if !tc.wantUsageErr {
				if err != nil {
					t.Fatalf("a2a %s error = %v, want nil", strings.Join(tc.args, " "), err)
				}
				return
			}
			if err == nil || clierr.Classify(err).Code != clierr.CodeUsage {
				t.Fatalf("a2a %s error = %v, want a usage error", strings.Join(tc.args, " "), err)
			}
		})
	}
}
