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
	"context"
	"encoding/json"
	"iter"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"github.com/google/go-cmp/cmp"
)

// authRecorder records the Authorization header of every request.
type authRecorder struct {
	mu   sync.Mutex
	seen []string
}

func (r *authRecorder) wrap(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		r.seen = append(r.seen, req.Header.Get("Authorization"))
		r.mu.Unlock()
		h.ServeHTTP(w, req)
	})
}

func (r *authRecorder) all() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.seen...)
}

func startAuthRecordingServer(t *testing.T) (string, *authRecorder) {
	t.Helper()
	capabilities := a2a.AgentCapabilities{Streaming: true}
	handler := a2asrv.NewHandler(newEchoExecutor(), a2asrv.WithCapabilityChecks(&capabilities))
	mux := http.NewServeMux()
	mux.Handle("/", a2asrv.NewRESTHandler(handler))
	rec := &authRecorder{}
	server := httptest.NewServer(rec.wrap(mux))
	t.Cleanup(server.Close)
	mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(newAgentCard(server.URL, capabilities)))
	return server.URL, rec
}

// writeCredentialHelper writes a helper that answers "get" with the
// Authorization header "Bearer <uri>".
func writeCredentialHelper(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("helper scripts need a POSIX shell")
	}
	path := filepath.Join(t.TempDir(), "helper")
	script := `#!/bin/sh
[ "$1" = get ] || exit 2
uri=$(sed 's/.*"uri":"\([^"]*\)".*/\1/')
printf '{"headers":{"Authorization":["Bearer %s"]}}' "$uri"
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCredentialHelper(t *testing.T) {
	t.Parallel()
	helper := writeCredentialHelper(t)
	tests := []struct {
		name     string
		args     func(url string) []string
		wantAuth func(url string) string
		wantErr  string
	}{
		{
			name:     "send",
			args:     func(url string) []string { return []string{"send", "-a", url, "--credential-helper", helper, "hello"} },
			wantAuth: func(url string) string { return "Bearer " + url },
		},
		{
			name:     "card get positional",
			args:     func(url string) []string { return []string{"card", "get", url, "--credential-helper", helper} },
			wantAuth: func(url string) string { return "Bearer " + url },
		},
		{
			name: "scoped to the agent host",
			args: func(url string) []string {
				return []string{"send", "-a", url, "--credential-helper", "127.0.0.1=" + helper, "hello"}
			},
			wantAuth: func(url string) string { return "Bearer " + url },
		},
		{
			name: "scoped to another host",
			args: func(url string) []string {
				return []string{"send", "-a", url, "--credential-helper", "other.example.com=" + helper, "hello"}
			},
			wantAuth: func(string) string { return "" },
		},
		{
			name: "explicit auth wins",
			args: func(url string) []string {
				return []string{"send", "-a", url, "--credential-helper", helper, "--auth", "Bearer flag", "hello"}
			},
			wantAuth: func(string) string { return "Bearer flag" },
		},
		{
			name:    "failing helper",
			args:    func(url string) []string { return []string{"send", "-a", url, "--credential-helper", "false", "hello"} },
			wantErr: "credential helper",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			url, rec := startAuthRecordingServer(t)
			args := tc.args(url)
			_, err := runCMD(t, args...)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("a2a %s error = %v, want %q", strings.Join(args, " "), err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("a2a %s error = %v", strings.Join(args, " "), err)
			}
			seen := rec.all()
			if len(seen) == 0 {
				t.Fatalf("a2a %s sent no requests", strings.Join(args, " "))
			}
			for _, got := range seen {
				if want := tc.wantAuth(url); got != want {
					t.Fatalf("a2a %s Authorization = %q, want %q", strings.Join(args, " "), got, want)
				}
			}
		})
	}
}

func TestAuthGet(t *testing.T) {
	t.Parallel()
	helper := writeCredentialHelper(t)
	tests := []struct {
		name    string
		args    []string
		want    map[string]any
		wantErr string
	}{
		{
			name: "redacted",
			args: []string{"auth", "get", "https://agent.example.com", "--credential-helper", helper, "-o", "json"},
			want: map[string]any{"url": "https://agent.example.com", "helper": helper, "headers": map[string]any{"Authorization": []any{"<redacted>"}}, "expires": ""},
		},
		{
			name: "revealed",
			args: []string{"auth", "get", "https://agent.example.com", "--credential-helper", helper, "--reveal", "-o", "json"},
			want: map[string]any{"url": "https://agent.example.com", "helper": helper, "headers": map[string]any{"Authorization": []any{"Bearer https://agent.example.com"}}, "expires": ""},
		},
		{
			name:    "no helper for the host",
			args:    []string{"auth", "get", "https://agent.example.com", "--credential-helper", "other.example.com=" + helper},
			wantErr: "no credential helper",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, err := runCMD(t, tc.args...)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("a2a %s error = %v, want %q", strings.Join(tc.args, " "), err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("a2a %s error = %v", strings.Join(tc.args, " "), err)
			}
			var got map[string]any
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("json.Unmarshal(%q) error = %v", out, err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("a2a %s wrong result (-want +got) diff = %s", strings.Join(tc.args, " "), diff)
			}
		})
	}
}

func TestSendOverUnixSocket(t *testing.T) {
	t.Parallel()
	dir, err := os.MkdirTemp("", "a2a")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("removing %s: %v", dir, err)
		}
	})
	sock := filepath.Join(dir, "a2a.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	const base = "http://agent.invalid"
	mux := http.NewServeMux()
	mux.Handle("/.well-known/agent-card.json", a2asrv.NewStaticAgentCardHandler(&a2a.AgentCard{
		Name:                "socket",
		SupportedInterfaces: []*a2a.AgentInterface{a2a.NewAgentInterface(base, a2a.TransportProtocolHTTPJSON)},
		Capabilities:        a2a.AgentCapabilities{Streaming: true},
	}))
	mux.Handle("/", a2asrv.NewRESTHandler(a2asrv.NewHandler(
		a2asrv.AgentExecutorFunc(func(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
			return func(yield func(a2a.Event, error) bool) {
				yield(a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart("over the socket")), nil)
			}
		}),
	)))
	server := &httptest.Server{Listener: ln, Config: &http.Server{Handler: mux}}
	server.Start()
	t.Cleanup(server.Close)

	tests := []struct {
		name    string
		socket  string
		args    []string
		wantErr bool
	}{
		{name: "agent card", socket: sock, args: []string{"-a", base}},
		{name: "endpoint", socket: sock, args: []string{"-e", base, "--transport", "rest"}},
		{name: "scoped to the host", socket: "agent.invalid=" + sock, args: []string{"-a", base}},
		{name: "scoped to another host", socket: "other.invalid=" + sock, args: []string{"-a", base, "--timeout", "2s"}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			args := append(append([]string{"send", "--unix-socket", tc.socket}, tc.args...), "hello")
			out, err := runCMD(t, args...)
			if (err != nil) != tc.wantErr {
				t.Fatalf("a2a %s error = %v, want error %v", strings.Join(args, " "), err, tc.wantErr)
			}
			if !tc.wantErr && !strings.Contains(out, "over the socket") {
				t.Fatalf("a2a %s = %q, want the agent's answer", strings.Join(args, " "), out)
			}
		})
	}
}
