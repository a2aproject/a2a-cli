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
	"fmt"
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

	"github.com/a2aproject/a2a-cli/internal/clicfg"
	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-cli/internal/localsrv"
)

type recordedRequest struct {
	Path string
	Auth string
}

// authRecorder records the path and Authorization header of every request.
type authRecorder struct {
	mu   sync.Mutex
	seen []recordedRequest
}

func (r *authRecorder) wrap(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		r.seen = append(r.seen, recordedRequest{Path: req.URL.Path, Auth: req.Header.Get("Authorization")})
		r.mu.Unlock()
		h.ServeHTTP(w, req)
	})
}

func (r *authRecorder) all() []recordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedRequest(nil), r.seen...)
}

func startAuthRecordingServer(t *testing.T) (string, *authRecorder) {
	t.Helper()
	return startAuthRecordingServerWithInterface(t, func(serverURL string) string { return serverURL })
}

// startAuthRecordingServerWithInterface serves a card advertising the
// interface URL that iface derives from the server URL.
func startAuthRecordingServerWithInterface(t *testing.T, iface func(serverURL string) string) (string, *authRecorder) {
	t.Helper()
	capabilities := a2a.AgentCapabilities{Streaming: true}
	handler := a2asrv.NewHandler(localsrv.NewEchoExecutor(), a2asrv.WithCapabilityChecks(&capabilities))
	mux := http.NewServeMux()
	mux.Handle("/", a2asrv.NewRESTHandler(handler))
	rec := &authRecorder{}
	server := httptest.NewServer(rec.wrap(mux))
	t.Cleanup(server.Close)
	mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(newAgentCard(iface(server.URL), capabilities)))
	return server.URL, rec
}

// credentialHelperPath is a helper that answers "get" with the Authorization
// header "Bearer <uri>". TestMain writes it before tests run in parallel:
// executing a file written while other tests fork can fail with ETXTBSY
// (golang/go#22315).
var credentialHelperPath string

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	dir, err := os.MkdirTemp("", "a2a-cli-test")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}()
	credentialHelperPath = filepath.Join(dir, "helper")
	script := `#!/bin/sh
[ "$1" = get ] || exit 2
uri=$(sed 's/.*"uri":"\([^"]*\)".*/\1/')
printf '{"headers":{"Authorization":["Bearer %s"]}}' "$uri"
`
	if err := os.WriteFile(credentialHelperPath, []byte(script), 0o700); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return m.Run()
}

func credentialHelper(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("helper scripts need a POSIX shell")
	}
	return credentialHelperPath
}

func TestCredentialHelper(t *testing.T) {
	t.Parallel()
	helper := credentialHelper(t)
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
				if want := tc.wantAuth(url); got.Auth != want {
					t.Fatalf("a2a %s Authorization for %s = %q, want %q", strings.Join(args, " "), got.Path, got.Auth, want)
				}
			}
		})
	}
}

func TestCredentialHelperFromConfigFile(t *testing.T) {
	t.Parallel()
	url, rec := startAuthRecordingServer(t)
	dir := t.TempDir()
	userPath := filepath.Join(dir, "config.yaml")
	yaml := fmt.Sprintf("credential-helper:\n  - scope: 127.0.0.1\n    command: [sh, %q]\n", credentialHelper(t))
	if err := os.WriteFile(userPath, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	loader := cfgLoaderFunc(func(opts clicfg.LoadOpts) (*clicfg.Store, error) {
		opts.WorkingDir = dir
		opts.GlobalPath = filepath.Join(dir, "none.env")
		opts.UserPath = userPath
		opts.LookupEnv = func(string) (string, bool) { return "", false }
		return clicfg.Load(opts)
	})
	args := []string{"send", "-a", url, "hello"}
	if _, err := runCMDWithConfig(t, deps{cfgLoader: loader}, args...); err != nil {
		t.Fatalf("a2a %s error = %v", strings.Join(args, " "), err)
	}
	want := []recordedRequest{
		{Path: a2asrv.WellKnownAgentCardPath, Auth: "Bearer " + url},
		{Path: "/message:send", Auth: "Bearer " + url},
	}
	if diff := cmp.Diff(want, rec.all()); diff != "" {
		t.Fatalf("a2a %s wrong result (-want +got) diff = %s", strings.Join(args, " "), diff)
	}
}

func TestCredentialHelperScopedToRequestHost(t *testing.T) {
	t.Parallel()
	helper := credentialHelper(t)
	url, rec := startAuthRecordingServerWithInterface(t, func(serverURL string) string {
		return strings.Replace(serverURL, "127.0.0.1", "localhost", 1)
	})
	args := []string{"send", "-a", url, "--credential-helper", "127.0.0.1=" + helper, "hello"}
	if _, err := runCMD(t, args...); err != nil {
		t.Fatalf("a2a %s error = %v", strings.Join(args, " "), err)
	}
	want := []recordedRequest{
		{Path: a2asrv.WellKnownAgentCardPath, Auth: "Bearer " + url},
		{Path: "/message:send", Auth: ""},
	}
	if diff := cmp.Diff(want, rec.all()); diff != "" {
		t.Fatalf("a2a %s wrong result (-want +got) diff = %s", strings.Join(args, " "), diff)
	}
}

func TestCredentialHelperOnlyRunsForRequests(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args []string
	}{
		{name: "config list", args: []string{"config", "list", "-a", "https://agent.example.com", "--credential-helper", "false"}},
		{name: "transport list", args: []string{"transport", "list", "-a", "https://agent.example.com", "--credential-helper", "false"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := runCMD(t, tc.args...); err != nil {
				t.Fatalf("a2a %s error = %v", strings.Join(tc.args, " "), err)
			}
		})
	}
}

func TestCredentialHelperInvalidSpec(t *testing.T) {
	t.Parallel()
	args := []string{"send", "-a", "https://agent.example.com", "--credential-helper", "example.com=", "hello"}
	_, err := runCMD(t, args...)
	if ce := clierr.Classify(err); ce.Code != clierr.CodeUsage {
		t.Fatalf("a2a %s error = %v, want a usage error", strings.Join(args, " "), err)
	}
}

func TestCredentialFlagsIgnoredFromDiscoveredEnv(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		env  string
		flag string
	}{
		{name: "credential helper", env: "A2ACLI_CREDENTIAL_HELPER=false", flag: "credential-helper"},
		{name: "unix socket", env: "A2ACLI_UNIX_SOCKET=/nonexistent/a2a.sock", flag: "unix-socket"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			url, rec := startAuthRecordingServer(t)
			dir := t.TempDir()
			envPath := filepath.Join(dir, ".env")
			if err := os.WriteFile(envPath, []byte(tc.env+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			loader := cfgLoaderFunc(func(opts clicfg.LoadOpts) (*clicfg.Store, error) {
				opts.WorkingDir = dir
				opts.GlobalPath = filepath.Join(dir, "none.env")
				opts.UserPath = filepath.Join(dir, "none.yaml")
				opts.LookupEnv = func(string) (string, bool) { return "", false }
				return clicfg.Load(opts)
			})
			args := []string{"card", "get", url}
			_, stderr, err := runCMDCapturingStderrWithConfig(t, deps{cfgLoader: loader}, args...)
			if err != nil {
				t.Fatalf("a2a %s error = %v", strings.Join(args, " "), err)
			}
			if want := "warning: ignoring " + tc.flag + " from " + envPath; !strings.Contains(stderr, want) {
				t.Fatalf("a2a %s stderr = %q, want %q", strings.Join(args, " "), stderr, want)
			}
			if len(rec.all()) == 0 {
				t.Fatalf("a2a %s sent no requests", strings.Join(args, " "))
			}
		})
	}
}

func TestAuthGet(t *testing.T) {
	t.Parallel()
	helper := credentialHelper(t)
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
			name: "ignores the helper of the configured agent",
			args: []string{"auth", "get", "https://other.example.com", "-a", "https://agent.example.com", "--credential-helper", "agent.example.com=false", "--credential-helper", "other.example.com=" + helper, "-o", "json"},
			want: map[string]any{"url": "https://other.example.com", "helper": "other.example.com=" + helper, "headers": map[string]any{"Authorization": []any{"<redacted>"}}, "expires": ""},
		},
		{
			name:    "local file",
			args:    []string{"auth", "get", "./card.json", "--credential-helper", helper},
			wantErr: "http(s) agent URL",
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
		name string
		args []string
	}{
		{name: "agent card", args: []string{"-a", base}},
		{name: "endpoint", args: []string{"-e", base, "--transport", "rest"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			args := append(append([]string{"send", "--unix-socket", sock}, tc.args...), "hello")
			out, err := runCMD(t, args...)
			if err != nil {
				t.Fatalf("a2a %s error = %v", strings.Join(args, " "), err)
			}
			if !strings.Contains(out, "over the socket") {
				t.Fatalf("a2a %s = %q, want the agent's answer", strings.Join(args, " "), out)
			}
		})
	}
}
