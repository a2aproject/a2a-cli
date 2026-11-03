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
	"crypto/x509"
	"fmt"
	"net"
	"net/http/httptest"
	"os"
	"os/exec"
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
	// SetFallbackRoots changes the whole process's trust store.
	const child = "A2ACLI_TEST_GRPC_ROOTS"
	if os.Getenv(child) != "1" {
		cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestGRPCTransportSecurity$")
		cmd.Env = append(os.Environ(), child+"=1", "GODEBUG=x509usefallbackroots=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("gRPC transport tests: %v\n%s", err, out)
		}
		return
	}

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

	_, err = runCMD(t, "send", "--endpoint", secured.URL, "--transport", "grpc", "--timeout", "3s", "hi")
	if err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Fatalf("send with untrusted certificate error = %v, want certificate verification failure", err)
	}

	roots := x509.NewCertPool()
	roots.AddCert(secured.Certificate())
	x509.SetFallbackRoots(roots)
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
		{name: "TLS endpoint", args: []string{"--endpoint", secured.URL}},
		{name: "TLS Agent Card", args: []string{"--agent-card", cardPath(secured.URL)}},
		{name: "TLS 0.3 endpoint", args: []string{"--endpoint", secured.URL, "--a2a-version", "0.3"}},
		{name: "TLS Agent Card with bare address", args: []string{"--agent-card", cardPath(secured.Listener.Addr().String())}},
		{name: "plaintext Agent Card", args: []string{"--agent-card", cardPath(plainAddr), "--insecure"}},
		{name: "explicit plaintext", args: []string{"--endpoint", "http://" + plainAddr, "--insecure"}},
		{name: "mismatched hostname", args: []string{"--endpoint", wrongHost}, wantErr: "certificate"},
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
