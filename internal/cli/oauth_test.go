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
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/a2aproject/a2a-cli/internal/clicfg"
	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-cli/internal/flagparse"
	"github.com/a2aproject/a2a-cli/internal/localsrv"
	"github.com/a2aproject/a2a-cli/internal/oauthstore"
	"github.com/a2aproject/a2a-cli/internal/output"
	"github.com/a2aproject/a2a-cli/internal/polling"
	"github.com/a2aproject/a2a-cli/internal/testutil"
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient/oauth"
	"github.com/a2aproject/a2a-go/v2/a2aclient/oauth/oauthtest"
	a2agrpcv1 "github.com/a2aproject/a2a-go/v2/a2agrpc/v1"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

const oauthTestMessage = "hello from oauth"

type oauthEnv struct {
	as    *oauthtest.AuthServer
	agent *oauthtest.ProtectedAgent
	deps  deps
}

func newOAuthEnv(t *testing.T, server oauthtest.ServerOptions, agent oauthtest.AgentOptions) *oauthEnv {
	t.Helper()
	as := oauthtest.NewAuthServer(t, server)
	return &oauthEnv{
		as:    as,
		agent: oauthtest.NewProtectedAgent(t, as, agent),
		deps: deps{
			poller:          polling.Stream,
			cfgLoader:       clicfg.LoadEmpty,
			authInteractive: func() bool { return true },
			openURL:         oauthtest.Browser(t),
			oauthDir:        t.TempDir(),
		},
	}
}

func (e *oauthEnv) clients() string {
	return fmt.Sprintf(`--oauth-clients={"authority":%q,"client_id":"cli-test"}`, e.as.URL)
}

// run runs a2a with OAuth turned on (--oauth auto); a later --oauth in args wins.
func (e *oauthEnv) run(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	return runCMDCapturingStderrWithConfig(t, e.deps, append([]string{"--oauth=auto"}, args...)...)
}

// runDefault runs a2a without choosing an OAuth mode.
func (e *oauthEnv) runDefault(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	return runCMDCapturingStderrWithConfig(t, e.deps, args...)
}

func (e *oauthEnv) mustRun(t *testing.T, args ...string) (stdout, stderr string) {
	t.Helper()
	stdout, stderr, err := e.run(t, args...)
	if err != nil {
		t.Fatalf("a2a %s error = %v\nstderr:\n%s", strings.Join(args, " "), err, stderr)
	}
	return stdout, stderr
}

func errCode(err error) string {
	if ce := clierr.Classify(err); ce != nil {
		return ce.Code
	}
	return ""
}

func TestOAuthSignInFromCard(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		agent oauthtest.AgentOptions
	}{
		{name: "metadata url", agent: oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL, Scopes: []string{"a2a.invoke"}}},
		{name: "inline authorization code", agent: oauthtest.AgentOptions{Card: oauthtest.CardAuthCode}},
		{name: "inline device code", agent: oauthtest.AgentOptions{Card: oauthtest.CardDeviceCode}},
		{name: "openid connect", agent: oauthtest.AgentOptions{Card: oauthtest.CardOIDC}},
		{name: "json-rpc", agent: oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL, JSONRPC: true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newOAuthEnv(t, oauthtest.ServerOptions{}, tc.agent)
			args := []string{"send", "-a", e.agent.URL, e.clients(), "-o", "json", oauthTestMessage}

			out, stderr := e.mustRun(t, args...)
			if !strings.Contains(stderr, "To authorize, open:") || !strings.Contains(stderr, "Authorized.") {
				t.Fatalf("first send stderr = %q, want sign-in instructions", stderr)
			}
			if !strings.Contains(out, oauthTestMessage) {
				t.Fatalf("first send output = %q, want the echoed message", out)
			}
			for _, h := range e.agent.AuthorizationHeaders() {
				if h == "" {
					t.Fatalf("the agent received a request without a token; the card requirement should be met before the first call")
				}
			}

			out, stderr = e.mustRun(t, args...)
			if strings.Contains(stderr, "To authorize") {
				t.Fatalf("second send stderr = %q, want the stored grant reused", stderr)
			}
			if !strings.Contains(out, oauthTestMessage) {
				t.Fatalf("second send output = %q, want the echoed message", out)
			}
		})
	}
}

func TestOAuthSignInFromChallenge(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		agent oauthtest.AgentOptions
		args  func(agentURL string) []string
	}{
		{name: "card without security", args: func(u string) []string { return []string{"send", "-a", u} }},
		{name: "protected card", agent: oauthtest.AgentOptions{ProtectCard: true}, args: func(u string) []string { return []string{"send", "-a", u} }},
		{name: "well-known probe", agent: oauthtest.AgentOptions{OmitResourceMetadata: true}, args: func(u string) []string { return []string{"send", "-a", u} }},
		{name: "endpoint", args: func(u string) []string { return []string{"send", "-e", u, "--transport", "rest"} }},
		{name: "json-rpc endpoint", agent: oauthtest.AgentOptions{JSONRPC: true}, args: func(u string) []string { return []string{"send", "-e", u, "--transport", "jsonrpc"} }},
		{name: "streaming", args: func(u string) []string { return []string{"send", "--stream", "-a", u} }},
		{name: "card get of a protected card", agent: oauthtest.AgentOptions{ProtectCard: true}, args: func(u string) []string { return []string{"card", "get", u} }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newOAuthEnv(t, oauthtest.ServerOptions{}, tc.agent)
			args := append(tc.args(e.agent.URL), e.clients(), oauthTestMessage)
			if tc.args(e.agent.URL)[0] == "card" {
				args = append(tc.args(e.agent.URL), e.clients())
			}
			out, stderr := e.mustRun(t, args...)
			if !strings.Contains(stderr, "Authorized.") {
				t.Fatalf("stderr = %q, want a sign-in", stderr)
			}
			if !strings.Contains(out, oauthTestMessage) && !strings.Contains(out, "Protected Echo") {
				t.Fatalf("output = %q, want the agent's answer", out)
			}
			if _, stderr = e.mustRun(t, args...); strings.Contains(stderr, "To authorize") {
				t.Fatalf("second run stderr = %q, want the stored grant reused", stderr)
			}
		})
	}
}

func TestOAuthCACert(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		card oauthtest.CardSecurity
	}{
		{name: "card requirement", card: oauthtest.CardMetadataURL},
		{name: "401 challenge", card: oauthtest.CardNone},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newOAuthEnv(t, oauthtest.ServerOptions{TLS: true}, oauthtest.AgentOptions{Card: tc.card, TLS: true})
			caCert := testutil.MustWriteTmpCertFile(t, e.as.Certificate(), e.agent.Certificate())
			roots := x509.NewCertPool()
			roots.AddCert(e.as.Certificate())
			browser := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots}}}
			e.deps.openURL = func(u string) {
				resp, err := browser.Get(u)
				if err != nil {
					t.Errorf("browser: GET %s error = %v", u, err)
					return
				}
				_ = resp.Body.Close()
			}
			args := []string{"send", "-a", e.agent.URL, e.clients(), "--ca-cert", caCert, "-o", "json", oauthTestMessage}

			if out, _ := e.mustRun(t, args...); !strings.Contains(out, oauthTestMessage) {
				t.Fatalf("first send output = %q, want the echoed message", out)
			}
			out, stderr := e.mustRun(t, args...)
			if strings.Contains(stderr, "To authorize") || !strings.Contains(out, oauthTestMessage) {
				t.Fatalf("second send = %q, stderr %q, want the stored grant reused", out, stderr)
			}
			e.mustRun(t, "auth", "logout", "--all", "--ca-cert", caCert)
			if got := len(e.as.Revoked()); got != 1 {
				t.Fatalf("revoked tokens = %d, want 1 revoked over TLS", got)
			}
		})
	}
}

func TestOAuthSignInNotAllowed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		interactive bool
		endpoint    bool
		extra       []string
	}{
		{name: "cached mode", interactive: true, extra: []string{"--oauth", "cached"}},
		{name: "no terminal", interactive: false},
		{name: "endpoint", interactive: false, endpoint: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL})
			e.deps.authInteractive = func() bool { return tc.interactive }
			agent, loginRef := []string{"-a", e.agent.URL}, e.agent.URL
			if tc.endpoint {
				agent = []string{"-e", e.agent.URL, "--transport", "rest"}
				loginRef = strings.Join(agent, " ")
			}
			args := append(append(append([]string{"send"}, agent...), e.clients(), oauthTestMessage), tc.extra...)
			_, stderr, err := e.run(t, args...)
			if got := errCode(err); got != clierr.CodeCredentialsMissing {
				t.Fatalf("a2a send error = %v (code %s), want %s", err, got, clierr.CodeCredentialsMissing)
			}
			if got := clierr.Classify(err).Exit; got != 4 {
				t.Fatalf("a2a send exit code = %d, want 4", got)
			}
			if strings.Contains(stderr, "To authorize") || e.as.Hits("/authorize") != 0 {
				t.Fatalf("a sign-in started although it is not allowed; stderr = %q", stderr)
			}

			// The hint names a login command that works for how the agent was called.
			hint := clierr.Classify(err).Hint
			login, ok := strings.CutPrefix(hint, "run `a2a ")
			login, _, found := strings.Cut(login, "`")
			if !ok || !found || login != "auth login "+loginRef {
				t.Fatalf("a2a send hint = %q, want it to suggest a2a auth login %s", hint, loginRef)
			}
			e.deps.authInteractive = func() bool { return false }
			e.mustRun(t, append(strings.Fields(login), e.clients())...)
			e.mustRun(t, args...)
		})
	}
}

func TestOAuthSkipped(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		extra      []string
		wantHeader string
	}{
		{name: "explicit --auth", extra: []string{"--auth", "Bearer user-supplied"}, wantHeader: "Bearer user-supplied"},
		{name: "explicit --svc-param", extra: []string{"--svc-param", "Authorization=Bearer from-param"}, wantHeader: "Bearer from-param"},
		{name: "oauth off", extra: []string{"--oauth", "off"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL})
			args := append([]string{"send", "-a", e.agent.URL, e.clients(), oauthTestMessage}, tc.extra...)
			_, stderr, err := e.run(t, args...)
			if err == nil {
				t.Fatalf("a2a send error = nil, want the agent to reject the request")
			}
			if code := errCode(err); code == clierr.CodeCredentialsMissing || code == clierr.CodeAuthFailed {
				t.Fatalf("a2a send error code = %s, want OAuth not involved", code)
			}
			if strings.Contains(stderr, "To authorize") || e.as.Hits("/.well-known/oauth-authorization-server") != 0 {
				t.Fatalf("OAuth discovery ran; stderr = %q", stderr)
			}
			if tc.wantHeader != "" && !slices.Contains(e.agent.AuthorizationHeaders(), tc.wantHeader) {
				t.Fatalf("agent Authorization headers = %q, want %q", e.agent.AuthorizationHeaders(), tc.wantHeader)
			}
		})
	}
}

func TestOAuthClientSelection(t *testing.T) {
	t.Parallel()
	t.Run("no client", func(t *testing.T) {
		t.Parallel()
		e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{})
		_, _, err := e.run(t, "send", "-a", e.agent.URL, oauthTestMessage)
		if got := errCode(err); got != clierr.CodeUsage {
			t.Fatalf("a2a send error = %v (code %s), want %s", err, got, clierr.CodeUsage)
		}
		if !strings.Contains(err.Error(), `"authority":"`+e.as.URL+`"`) {
			t.Fatalf("a2a send error = %v, want a ready-to-paste oauth-clients entry", err)
		}
	})
	t.Run("CIMD fallback", func(t *testing.T) {
		t.Parallel()
		e := newOAuthEnv(t, oauthtest.ServerOptions{CIMD: true}, oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL})
		e.mustRun(t, "send", "-a", e.agent.URL, oauthTestMessage)
		if got := e.as.LastAuthorizeQuery().Get("client_id"); got != oauthstore.DefaultClientID {
			t.Fatalf("client_id = %q, want %q", got, oauthstore.DefaultClientID)
		}
	})
	t.Run("CIMD fallback for an inline flow", func(t *testing.T) {
		t.Parallel()
		e := newOAuthEnv(t, oauthtest.ServerOptions{CIMD: true}, oauthtest.AgentOptions{Card: oauthtest.CardAuthCode})
		e.mustRun(t, "send", "-a", e.agent.URL, oauthTestMessage)
		if got := e.as.LastAuthorizeQuery().Get("client_id"); got != oauthstore.DefaultClientID {
			t.Fatalf("client_id = %q, want %q", got, oauthstore.DefaultClientID)
		}
	})
	t.Run("CIMD fallback configured", func(t *testing.T) {
		t.Parallel()
		e := newOAuthEnv(t, oauthtest.ServerOptions{CIMD: true}, oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL})
		const id = "https://tools.example.com/a2a-cli.json"
		e.mustRun(t, "send", "-a", e.agent.URL, "--oauth-default-client-id", id, oauthTestMessage)
		if got := e.as.LastAuthorizeQuery().Get("client_id"); got != id {
			t.Fatalf("client_id = %q, want %q", got, id)
		}
		_, _, err := e.run(t, "send", "-a", e.agent.URL, "--oauth-default-client-id=", "--oauth", "cached", oauthTestMessage)
		if got := errCode(err); got != clierr.CodeUsage {
			t.Fatalf("a2a send with the fallback disabled error = %v (code %s), want %s", err, got, clierr.CodeUsage)
		}
	})
	t.Run("from a yaml config file", func(t *testing.T) {
		t.Parallel()
		e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL})
		path := filepath.Join(t.TempDir(), "a2a.yaml")
		yaml := fmt.Sprintf("oauth-clients:\n  - authority: %s\n    client_id: from-yaml\n    scopes: [a2a.invoke]\n", e.as.URL)
		if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
			t.Fatal(err)
		}
		e.deps.cfgLoader = clicfg.Load
		e.mustRun(t, "send", "-a", e.agent.URL, "--config", path, oauthTestMessage)
		if got := e.as.LastAuthorizeQuery().Get("client_id"); got != "from-yaml" {
			t.Fatalf("client_id = %q, want %q", got, "from-yaml")
		}
	})
}

func TestOAuthRequests(t *testing.T) {
	t.Parallel()
	e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL})
	e.mustRun(t, "send", "-a", e.agent.URL, e.clients(), "--svc-param", "X-Trace=1", oauthTestMessage)

	if got := e.as.LastForm("/token").Get("resource"); got != e.agent.URL {
		t.Fatalf("resource sent to the token endpoint = %q, want %q", got, e.agent.URL)
	}
	if got := e.as.LastAuthorizeQuery().Get("resource"); got != e.agent.URL {
		t.Fatalf("resource sent to the authorization endpoint = %q, want %q", got, e.agent.URL)
	}
	if e.as.SawAuthorizationHeader() {
		t.Fatalf("the authorization server received an Authorization header")
	}

	e.as.Invalidate()
	_, stderr := e.mustRun(t, "send", "-a", e.agent.URL, e.clients(), oauthTestMessage)
	if strings.Contains(stderr, "To authorize") {
		t.Fatalf("send after the token was invalidated stderr = %q, want a silent refresh", stderr)
	}
}

func TestAuthCommands(t *testing.T) {
	t.Parallel()
	e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL, Scopes: []string{"a2a.invoke"}})

	out, stderr := e.mustRun(t, "auth", "login", e.agent.URL, e.clients(), "-o", "json")
	if !strings.Contains(stderr, "To authorize, open:") {
		t.Fatalf("auth login stderr = %q, want sign-in instructions", stderr)
	}
	var login grantView
	if err := json.Unmarshal([]byte(out), &login); err != nil {
		t.Fatalf("json.Unmarshal(auth login output %q) error = %v", out, err)
	}
	want := grantView{Resource: e.agent.URL, Issuer: e.as.URL, ClientID: "cli-test", ClientSource: oauth.ClientSourceConfig, Scopes: []string{"a2a.invoke"}, Refreshable: true}
	if diff := cmp.Diff(want, login, cmpIgnoreExpiry); diff != "" {
		t.Fatalf("auth login wrong result (-want +got) diff = %s", diff)
	}

	out, _ = e.mustRun(t, "auth", "list", "-o", "json")
	var listed []grantView
	if err := json.Unmarshal([]byte(out), &listed); err != nil {
		t.Fatalf("json.Unmarshal(auth list output %q) error = %v", out, err)
	}
	if diff := cmp.Diff([]grantView{want}, listed, cmpIgnoreExpiry); diff != "" {
		t.Fatalf("auth list wrong result (-want +got) diff = %s", diff)
	}

	out, _ = e.mustRun(t, "auth", "get", e.agent.URL, "-o", "json")
	var got struct {
		URL   string      `json:"url"`
		OAuth []grantView `json:"oauth"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json.Unmarshal(auth get output %q) error = %v", out, err)
	}
	if len(got.OAuth) != 1 || got.OAuth[0].AccessToken != "" {
		t.Fatalf("auth get = %s, want one grant with the token redacted", out)
	}
	out, _ = e.mustRun(t, "auth", "get", e.agent.URL, "--reveal", "-o", "json")
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.OAuth) != 1 || !e.as.Valid(got.OAuth[0].AccessToken, e.agent.URL, nil) {
		t.Fatalf("auth get --reveal = %s, want the stored access token", out)
	}

	out, _ = e.mustRun(t, "auth", "logout", e.agent.URL, "-o", "json")
	var logout struct {
		Removed []logoutView `json:"removed"`
	}
	if err := json.Unmarshal([]byte(out), &logout); err != nil {
		t.Fatalf("json.Unmarshal(auth logout output %q) error = %v", out, err)
	}
	if diff := cmp.Diff([]logoutView{{Resource: e.agent.URL, Issuer: e.as.URL, Revoked: true}}, logout.Removed); diff != "" {
		t.Fatalf("auth logout wrong result (-want +got) diff = %s", diff)
	}
	if len(e.as.Revoked()) != 1 {
		t.Fatalf("revoked tokens = %v, want the refresh token revoked", e.as.Revoked())
	}
	if out, _ = e.mustRun(t, "auth", "list"); !strings.Contains(out, "No stored OAuth grants") {
		t.Fatalf("auth list after logout = %q, want no grants", out)
	}

	_, stderr = e.mustRun(t, "auth", "login", e.agent.URL, e.clients(), "--device", "--scope", "a2a.invoke")
	if !strings.Contains(stderr, "Enter code:") {
		t.Fatalf("auth login --device stderr = %q, want a user code", stderr)
	}
	if out, _ = e.mustRun(t, "auth", "logout", "--all"); !strings.Contains(out, "Logged out of "+e.agent.URL+" (revoked)") {
		t.Fatalf("auth logout --all output = %q, want the grant revoked", out)
	}
}

func TestAuthLoginFromChallenge(t *testing.T) {
	t.Parallel()
	e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{})
	e.mustRun(t, "auth", "login", "-e", e.agent.URL, "--transport", "rest", e.clients())
	e.deps.authInteractive = func() bool { return false }
	e.mustRun(t, "send", "-e", e.agent.URL, "--transport", "rest", "--oauth", "cached", e.clients(), oauthTestMessage)
}

func TestAuthUsageErrors(t *testing.T) {
	t.Parallel()
	e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{})
	tests := [][]string{
		{"auth", "logout"},
		{"auth", "logout", e.agent.URL, "--all"},
		{"send", "-a", e.agent.URL, "--oauth", "sometimes", "x"},
		{"send", "-a", e.agent.URL, "--oauth-clients", "not json", "x"},
	}
	for _, args := range tests {
		if _, _, err := e.run(t, args...); errCode(err) != clierr.CodeUsage {
			t.Errorf("a2a %s error = %v, want a usage error", strings.Join(args, " "), err)
		}
	}
}

type memKeyring struct {
	mu      sync.Mutex
	secrets map[string]string
	reads   int
}

func (k *memKeyring) Get(service, user string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.reads++
	if s, ok := k.secrets[service+"/"+user]; ok {
		return s, nil
	}
	return "", oauthstore.ErrSecretNotFound
}

func (k *memKeyring) Set(service, user, secret string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.secrets == nil {
		k.secrets = map[string]string{}
	}
	k.secrets[service+"/"+user] = secret
	return nil
}

func (k *memKeyring) Delete(service, user string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.secrets, service+"/"+user)
	return nil
}

func TestOAuthKeychainStore(t *testing.T) {
	t.Parallel()
	e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL})
	kr := &memKeyring{}
	e.deps.keyring = kr
	args := []string{"send", "-a", e.agent.URL, e.clients(), "--oauth-store", "keychain", oauthTestMessage}
	e.mustRun(t, args...)

	index, err := os.ReadFile(filepath.Join(e.deps.oauthDir, oauthstore.KeyringIndexName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(index), "access_token") || strings.Contains(string(index), "refresh_token") {
		t.Fatalf("%s contains tokens: %s", oauthstore.KeyringIndexName, index)
	}
	if _, err := os.Stat(filepath.Join(e.deps.oauthDir, "grants.json")); !os.IsNotExist(err) {
		t.Fatalf("grants.json exists with --oauth-store keychain (stat error %v)", err)
	}
	if len(kr.secrets) != 2 {
		t.Fatalf("keychain entries = %d, want one per token", len(kr.secrets))
	}
	if _, stderr := e.mustRun(t, args...); strings.Contains(stderr, "To authorize") {
		t.Fatalf("second send stderr = %q, want the grant read from the keychain", stderr)
	}
	e.mustRun(t, "auth", "logout", "--all", "--oauth-store", "keychain")
	if len(kr.secrets) != 0 {
		t.Fatalf("keychain entries after logout = %d, want 0", len(kr.secrets))
	}
}

func (k *memKeyring) readCount() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.reads
}

// TestOAuthKeychainReadsSecretsOnlyWhenUsed checks that listing grants and
// calling an agent that no grant applies to don't read the keychain.
func TestOAuthKeychainReadsSecretsOnlyWhenUsed(t *testing.T) {
	t.Parallel()
	e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL})
	kr := &memKeyring{}
	e.deps.keyring = kr
	e.mustRun(t, "auth", "login", e.agent.URL, e.clients(), "--oauth-store", "keychain")
	plain, _ := startAuthRecordingServer(t)

	before := kr.readCount()
	out, _ := e.mustRun(t, "auth", "list", "--oauth-store", "keychain")
	if !strings.Contains(out, "(refreshable)") {
		t.Fatalf("auth list = %q, want the grant shown as refreshable", out)
	}
	if got := kr.readCount() - before; got != 0 {
		t.Fatalf("auth list read %d keychain entries, want none", got)
	}

	before = kr.readCount()
	e.mustRun(t, "send", "-a", plain, e.clients(), "--oauth-store", "keychain", "hello")
	if got := kr.readCount() - before; got != 0 {
		t.Fatalf("a2a send to an agent without OAuth read %d keychain entries, want none", got)
	}

	// --reveal reads the tokens the listing leaves out.
	out, _ = e.mustRun(t, "auth", "get", e.agent.URL, "--reveal", "--oauth-store", "keychain", "-o", "json")
	var got struct {
		OAuth []grantView `json:"oauth"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", out, err)
	}
	if len(got.OAuth) != 1 || !e.as.Valid(got.OAuth[0].AccessToken, e.agent.URL, nil) {
		t.Fatalf("auth get --reveal with the keychain = %+v, want the stored access token", got.OAuth)
	}
}

func TestOAuthKeychainGrantWithoutTokens(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*memKeyring)
		store  []string
	}{
		{name: "file store", store: nil},
		{name: "keychain entries removed", mutate: func(k *memKeyring) { k.secrets = nil }, store: []string{"--oauth-store", "keychain"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL})
			kr := &memKeyring{}
			e.deps.keyring = kr
			e.mustRun(t, "auth", "login", e.agent.URL, e.clients(), "--oauth-store", "keychain")
			if tc.mutate != nil {
				tc.mutate(kr)
			}
			sent := len(e.agent.AuthorizationHeaders())
			e.deps.authInteractive = func() bool { return false }
			args := append([]string{"send", "-a", e.agent.URL, e.clients(), oauthTestMessage}, tc.store...)
			_, _, err := e.run(t, args...)
			if got := errCode(err); got != clierr.CodeCredentialsMissing {
				t.Fatalf("a2a send error = %v (code %s), want %s", err, got, clierr.CodeCredentialsMissing)
			}
			if got := e.agent.AuthorizationHeaders()[sent:]; len(got) != 0 {
				t.Fatalf("the agent received %q, want no request without a usable token", got)
			}
		})
	}
}

// TestOAuthStoreChecked checks that an invalid --oauth-store is a usage error
// in every mode, like an invalid --oauth, even for commands that don't open the
// store.
func TestOAuthStoreChecked(t *testing.T) {
	t.Parallel()
	e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{})
	for _, run := range []func(*testing.T, ...string) (string, string, error){e.runDefault, e.run} {
		for _, args := range [][]string{{"version", "--oauth-store", "bogus"}, {"auth", "list", "--oauth-store", "bogus"}} {
			if _, _, err := run(t, args...); errCode(err) != clierr.CodeUsage {
				t.Errorf("a2a %s error = %v, want a usage error", strings.Join(args, " "), err)
			}
		}
	}
}

func TestOAuthOffByDefault(t *testing.T) {
	t.Parallel()
	metadataHits := func(e *oauthEnv) int {
		return e.as.Hits("/.well-known/oauth-authorization-server") + e.as.Hits("/authorize") + e.as.Hits("/token")
	}
	hint := "the agent asks for an OAuth token: run `a2a auth login "

	t.Run("other credentials reach an OAuth-only agent", func(t *testing.T) {
		t.Parallel()
		e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL})
		_, _, err := e.runDefault(t, "send", "-a", e.agent.URL, e.clients(), "--svc-param", "X-API-Key=k1", oauthTestMessage)
		if got := len(e.agent.AuthorizationHeaders()); got != 1 {
			t.Fatalf("requests reaching the agent = %d, want the request sent", got)
		}
		if got := metadataHits(e); got != 0 {
			t.Fatalf("requests to the authorization server = %d, want none with OAuth off", got)
		}
		ce := clierr.Classify(err)
		if ce == nil || ce.Exit != 1 || !strings.Contains(ce.Hint, hint+e.agent.URL+"`") || !strings.Contains(ce.Hint, "--oauth auto") {
			t.Fatalf("a2a send error = %#v, want the 401 (exit 1) with a hint to sign in and use --oauth auto", ce)
		}
	})

	t.Run("a JSON-RPC agent's 401 gets the hint", func(t *testing.T) {
		t.Parallel()
		e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="https://agent.example.com/.well-known/oauth-protected-resource"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		}))
		t.Cleanup(srv.Close)
		_, _, err := e.runDefault(t, "send", "-e", srv.URL, "--transport", "jsonrpc", oauthTestMessage)
		ce := clierr.Classify(err)
		if ce == nil || ce.Exit != 1 || !strings.Contains(ce.Hint, "the agent asks for an OAuth token") {
			t.Fatalf("a2a send error = %#v, want the 401 (exit 1) with a hint to sign in", ce)
		}
	})

	t.Run("auth login works and stored grants need opting in", func(t *testing.T) {
		t.Parallel()
		e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL})
		if _, stderr, err := e.runDefault(t, "auth", "login", e.agent.URL, e.clients()); err != nil {
			t.Fatalf("a2a auth login with the default mode error = %v\nstderr:\n%s", err, stderr)
		}
		if out, _, err := e.runDefault(t, "auth", "list"); err != nil || !strings.Contains(out, e.agent.URL) {
			t.Fatalf("a2a auth list = %q, %v, want the grant", out, err)
		}
		if _, _, err := e.runDefault(t, "send", "-a", e.agent.URL, e.clients(), oauthTestMessage); err == nil {
			t.Fatal("a2a send with the default mode error = nil, want the agent's 401")
		}
		if headers := e.agent.AuthorizationHeaders(); headers[len(headers)-1] != "" {
			t.Fatalf("Authorization sent with OAuth off = %q, want none", headers[len(headers)-1])
		}
		if _, stderr, err := e.runDefault(t, "send", "-a", e.agent.URL, e.clients(), "--oauth", "cached", oauthTestMessage); err != nil {
			t.Fatalf("a2a send --oauth cached error = %v\nstderr:\n%s", err, stderr)
		}
	})

	t.Run("a protected card's 401 gets the sign-in hint", func(t *testing.T) {
		t.Parallel()
		e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{ProtectCard: true})
		_, _, err := e.runDefault(t, "card", "get", e.agent.URL)
		ce := clierr.Classify(err)
		if ce == nil || ce.Code != clierr.CodeCardInvalid || !strings.Contains(ce.Hint, hint+e.agent.URL+"`") {
			t.Fatalf("a2a card get error = %#v, want %s with a hint to sign in to %s", ce, clierr.CodeCardInvalid, e.agent.URL)
		}
	})

	t.Run("a rejected --auth gets no hint", func(t *testing.T) {
		t.Parallel()
		e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{})
		_, _, err := e.runDefault(t, "send", "-a", e.agent.URL, "--auth", "Bearer stale", oauthTestMessage)
		if ce := clierr.Classify(err); ce == nil || strings.Contains(ce.Hint, hint) {
			t.Fatalf("a2a send --auth error = %#v, want the 401 without the OAuth hint", ce)
		}
	})

	t.Run("a protected card's 401 for --auth gets no hint", func(t *testing.T) {
		t.Parallel()
		e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{ProtectCard: true})
		_, _, err := e.runDefault(t, "card", "get", e.agent.URL, "--auth", "Bearer stale")
		if ce := clierr.Classify(err); ce == nil || strings.Contains(ce.Hint, hint) {
			t.Fatalf("a2a card get --auth error = %#v, want the 401 without the OAuth hint", ce)
		}
	})

	t.Run("oauth: auto in a config file", func(t *testing.T) {
		t.Parallel()
		e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL})
		path := filepath.Join(t.TempDir(), "a2a.yaml")
		yaml := fmt.Sprintf("oauth: auto\noauth-clients:\n  - authority: %s\n    client_id: from-yaml\n", e.as.URL)
		if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
			t.Fatal(err)
		}
		e.deps.cfgLoader = clicfg.Load
		if _, stderr, err := e.runDefault(t, "send", "-a", e.agent.URL, "--config", path, oauthTestMessage); err != nil || !strings.Contains(stderr, "Authorized. Grant saved") {
			t.Fatalf("a2a send with oauth: auto in the config error = %v\nstderr:\n%s", err, stderr)
		}
	})
}

// TestOAuthOffWithoutHome checks that commands other than a2a auth need no home
// directory while OAuth is off. It changes HOME, so it doesn't run in parallel.
func TestOAuthOffWithoutHome(t *testing.T) {
	t.Setenv("HOME", "")
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		http.Error(w, "reached", http.StatusTeapot)
	}))
	t.Cleanup(srv.Close)
	run := func(args ...string) error {
		cfg := &globalConfig{
			Printer:   output.NewPrinter(io.Discard, output.ModeText),
			svcParams: &flagparse.ServiceParams{},
			errOut:    io.Discard,
		}
		root, err := newRootCmd(cfg, deps{poller: polling.Stream, cfgLoader: clicfg.LoadEmpty})
		if err != nil {
			t.Fatalf("newRootCmd() error = %v", err)
		}
		root.SetArgs(args)
		return root.Execute()
	}

	if err := run("version"); err != nil {
		t.Fatalf("a2a version without HOME error = %v", err)
	}
	if err := run("send", "-e", srv.URL, "--transport", "rest", "hi"); err != nil && strings.Contains(err.Error(), "home") || hits.Load() != 1 {
		t.Fatalf("a2a send without HOME error = %v, requests = %d, want the request sent", err, hits.Load())
	}
	// auth get still prints a credential helper's headers without a grant store.
	if err := run("auth", "get", "https://agent.example.com", "--credential-helper", credentialHelper(t)); err != nil {
		t.Errorf("a2a auth get with a credential helper without HOME error = %v", err)
	}
	for _, args := range [][]string{{"auth", "list"}, {"--oauth", "auto", "version"}} {
		if err := run(args...); err == nil || !strings.Contains(err.Error(), "home directory") {
			t.Errorf("a2a %s without HOME error = %v, want the home directory error", strings.Join(args, " "), err)
		}
	}
}

var cmpIgnoreExpiry = cmp.FilterPath(func(p cmp.Path) bool { return p.Last().String() == ".ExpiresAt" }, cmp.Ignore())

// apiKeyOrOAuthAgent serves a card that accepts an API key or OAuth and counts
// the A2A requests that reach it.
func apiKeyOrOAuthAgent(t *testing.T) (string, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	card := &a2a.AgentCard{
		Name:                "apikey-or-oauth",
		Version:             "1.0.0",
		SupportedInterfaces: []*a2a.AgentInterface{a2a.NewAgentInterface(srv.URL, a2a.TransportProtocolHTTPJSON)},
		DefaultInputModes:   []string{"text"},
		DefaultOutputModes:  []string{"text"},
		Skills:              []a2a.AgentSkill{},
		SecuritySchemes: a2a.NamedSecuritySchemes{
			"key":   a2a.APIKeySecurityScheme{Location: a2a.APIKeySecuritySchemeLocationHeader, Name: "X-API-Key"},
			"oauth": a2a.OAuth2SecurityScheme{Flows: a2a.AuthorizationCodeOAuthFlow{AuthorizationURL: "https://as.example.com/authorize", TokenURL: "https://as.example.com/token"}},
		},
		SecurityRequirements: a2a.SecurityRequirementsOptions{{"key": {}}, {"oauth": {}}},
	}
	mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(card))
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		http.Error(w, "reached", http.StatusTeapot)
	})
	return srv.URL, &hits
}

func TestOAuthNotRequired(t *testing.T) {
	t.Parallel()
	t.Run("card also accepts an API key", func(t *testing.T) {
		t.Parallel()
		agent, hits := apiKeyOrOAuthAgent(t)
		d := deps{poller: polling.Stream, cfgLoader: clicfg.LoadEmpty, authInteractive: func() bool { return false }, oauthDir: t.TempDir()}
		for _, extra := range [][]string{nil, {`--oauth-clients={"authority":"https://as.example.com","client_id":"c"}`}} {
			args := append([]string{"send", "-a", agent, "--oauth", "auto", "--svc-param", "X-API-Key=k1", "hi"}, extra...)
			_, _, err := runCMDCapturingStderrWithConfig(t, d, args...)
			if code := errCode(err); code == clierr.CodeUsage || code == clierr.CodeCredentialsMissing {
				t.Fatalf("a2a %s error = %v (code %s), want the request sent", strings.Join(args, " "), err, code)
			}
		}
		if got := hits.Load(); got != 2 {
			t.Fatalf("requests reaching the agent = %d, want 2", got)
		}
	})
	t.Run("unreadable grant store", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "grants.json"), []byte("{not json"), 0o600); err != nil {
			t.Fatal(err)
		}
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hits.Add(1)
			http.Error(w, "reached", http.StatusTeapot)
		}))
		t.Cleanup(srv.Close)
		d := deps{poller: polling.Stream, cfgLoader: clicfg.LoadEmpty, oauthDir: dir}
		_, _, err := runCMDCapturingStderrWithConfig(t, d, "send", "--oauth", "auto", "-e", srv.URL, "--transport", "rest", "hi")
		if err != nil && strings.Contains(err.Error(), "grants.json") || hits.Load() != 1 {
			t.Fatalf("a2a send error = %v, requests = %d, want the request sent", err, hits.Load())
		}
	})
}

func TestAuthLoginScope(t *testing.T) {
	t.Parallel()
	e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL})
	e.mustRun(t, "auth", "login", e.agent.URL, e.clients(), "--scope", "a2a.invoke")
	e.mustRun(t, "send", "-a", e.agent.URL, e.clients(), "--oauth", "cached", oauthTestMessage)
}

func TestAuthLogoutOneAgent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store := &oauthstore.FileStore{Dir: dir}
	// The gateway serves no agent card, so logout falls back to the URL.
	gw := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(gw.Close)
	ep := oauth.Endpoints{Issuer: "https://as.example.com", Authorization: "https://as.example.com/authorize", Token: "https://as.example.com/token"}
	for _, r := range []string{gw.URL + "/agents/a", gw.URL + "/agents/b", gw.URL} {
		g := oauth.Grant{Issuer: ep.Issuer, ClientID: "c", Resource: r, AccessToken: "at", Endpoints: ep, ExpiresAt: time.Now().Add(time.Hour)}
		if err := store.Put(t.Context(), g); err != nil {
			t.Fatal(err)
		}
	}
	d := deps{poller: polling.Stream, cfgLoader: clicfg.LoadEmpty, oauthDir: dir}
	if _, _, err := runCMDCapturingStderrWithConfig(t, d, "auth", "logout", gw.URL+"/agents/a"); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{gw.URL, gw.URL + "/agents/b"}, storedResources(t, dir)); diff != "" {
		t.Fatalf("grants after logging out of /agents/a (-want +got) diff = %s", diff)
	}
}

func TestOAuthInsecureWarning(t *testing.T) {
	t.Parallel()
	e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL})
	e.mustRun(t, "auth", "login", e.agent.URL, e.clients())
	_, stderr := e.mustRun(t, "send", "-a", e.agent.URL, e.clients(), "--insecure", oauthTestMessage)
	if got := strings.Count(stderr, insecureCredentialWarning); got != 1 {
		t.Fatalf("send --insecure stderr = %q, want the credential warning once", stderr)
	}
}

// unixSocketProxy serves HTTP on a Unix socket and forwards each request to
// the TCP server its Host names, so --unix-socket reaches the test servers.
func unixSocketProxy(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "a2a")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "a2a.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	proxy := &httputil.ReverseProxy{Rewrite: func(r *httputil.ProxyRequest) {
		r.Out.URL.Scheme, r.Out.URL.Host = "http", r.In.Host
		r.Out.Host = r.In.Host
	}}
	server := &httptest.Server{Listener: ln, Config: &http.Server{Handler: proxy}}
	server.Start()
	t.Cleanup(server.Close)
	return sock
}

func TestOAuthExplicitCredentialsWin(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		extra func(t *testing.T) []string
		// oauthReacts is set when OAuth still handles the agent's 401; a
		// credential that sets Authorization keeps OAuth out entirely.
		oauthReacts bool
	}{
		{name: "--svc-param", extra: func(*testing.T) []string { return []string{"--svc-param", "X-API-Key=k"} }, oauthReacts: true},
		{name: "--unix-socket", extra: func(t *testing.T) []string { return []string{"--unix-socket", unixSocketProxy(t)} }, oauthReacts: true},
		{name: "credential helper for the agent host", extra: func(t *testing.T) []string {
			return []string{"--credential-helper", "127.0.0.1=" + credentialHelper(t)}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL})
			e.deps.authInteractive = func() bool { return false }
			args := append([]string{"send", "-a", e.agent.URL, e.clients(), oauthTestMessage}, tc.extra(t)...)
			_, stderr, err := e.run(t, args...)
			if got := len(e.agent.AuthorizationHeaders()); got == 0 {
				t.Fatalf("a2a %s sent no request to the agent (error %v, stderr %q), want the request sent", strings.Join(args, " "), err, stderr)
			}
			want := clierr.CodeProtocol
			if tc.oauthReacts {
				want = clierr.CodeCredentialsMissing
			}
			if got := errCode(err); got != want {
				t.Fatalf("a2a %s error = %v (code %s), want %s after the agent's 401", strings.Join(args, " "), err, got, want)
			}
		})
	}

	t.Run("no client registration for the card", func(t *testing.T) {
		t.Parallel()
		e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{Card: oauthtest.CardAuthCode})
		e.deps.authInteractive = func() bool { return false }
		_, stderr, err := e.run(t, "send", "-a", e.agent.URL, "--svc-param", "X-API-Key=k", oauthTestMessage)
		if got := len(e.agent.AuthorizationHeaders()); got == 0 {
			t.Fatalf("a2a send --svc-param sent no request to the agent (error %v, stderr %q), want the request sent", err, stderr)
		}
	})

	t.Run("card metadata unreachable", func(t *testing.T) {
		t.Parallel()
		var hits atomic.Int32
		mux := http.NewServeMux()
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)
		card := &a2a.AgentCard{
			Name: "oauth-only", Version: "1",
			SupportedInterfaces: []*a2a.AgentInterface{a2a.NewAgentInterface(srv.URL, a2a.TransportProtocolHTTPJSON)},
			DefaultInputModes:   []string{"text"}, DefaultOutputModes: []string{"text"}, Skills: []a2a.AgentSkill{},
			SecuritySchemes: a2a.NamedSecuritySchemes{
				"oauth": a2a.OAuth2SecurityScheme{
					Oauth2MetadataURL: "http://127.0.0.1:1/.well-known/oauth-authorization-server",
					Flows:             a2a.AuthorizationCodeOAuthFlow{AuthorizationURL: "http://127.0.0.1:1/authorize", TokenURL: "http://127.0.0.1:1/token"},
				},
			},
			SecurityRequirements: a2a.SecurityRequirementsOptions{{"oauth": {}}},
		}
		mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(card))
		mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
			hits.Add(1)
			http.Error(w, "reached", http.StatusTeapot)
		})
		d := deps{poller: polling.Stream, cfgLoader: clicfg.LoadEmpty, authInteractive: func() bool { return false }, oauthDir: t.TempDir()}
		for _, extra := range [][]string{{"--svc-param", "X-API-Key=k"}, {"--unix-socket", unixSocketProxy(t)}} {
			before := hits.Load()
			args := append([]string{"send", "-a", srv.URL, "--oauth", "auto", oauthTestMessage}, extra...)
			_, stderr, err := runCMDCapturingStderrWithConfig(t, d, args...)
			if hits.Load() == before {
				t.Fatalf("a2a send %s sent no request to the agent (error %v, stderr %q), want the request sent", extra[0], err, stderr)
			}
		}
	})

	t.Run("a stored grant is still attached", func(t *testing.T) {
		t.Parallel()
		e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL})
		e.mustRun(t, "auth", "login", e.agent.URL, e.clients())
		e.deps.authInteractive = func() bool { return false }
		e.mustRun(t, "send", "-a", e.agent.URL, e.clients(), "--svc-param", "X-API-Key=k", oauthTestMessage)
	})
}

// TestOAuthStepUp checks that when the agent answers insufficient_scope for
// more scopes than the card names, signing in again for them replaces the
// narrower grant, and later calls use the wider one without signing in.
func TestOAuthStepUp(t *testing.T) {
	t.Parallel()
	as := oauthtest.NewAuthServer(t, oauthtest.ServerOptions{})
	var full atomic.Int32
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	prm := srv.URL + "/.well-known/oauth-protected-resource"
	card := &a2a.AgentCard{
		Name: "stepup", Version: "1",
		SupportedInterfaces: []*a2a.AgentInterface{a2a.NewAgentInterface(srv.URL, a2a.TransportProtocolHTTPJSON)},
		DefaultInputModes:   []string{"text"}, DefaultOutputModes: []string{"text"}, Skills: []a2a.AgentSkill{},
		SecuritySchemes: a2a.NamedSecuritySchemes{
			"oauth": a2a.OAuth2SecurityScheme{
				Oauth2MetadataURL: as.MetadataURL(),
				Flows:             a2a.AuthorizationCodeOAuthFlow{AuthorizationURL: as.AuthorizationURL(), TokenURL: as.URL + "/token", Scopes: map[string]string{"a": "a"}},
			},
		},
		SecurityRequirements: a2a.SecurityRequirementsOptions{{"oauth": a2a.SecuritySchemeScopes([]string{"a"})}},
	}
	mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(card))
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"resource":%q,"authorization_servers":[%q]}`, srv.URL, as.URL)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		tok, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		switch {
		case as.Valid(tok, srv.URL, []string{"a", "b"}):
			full.Add(1)
			http.Error(w, "reached with a and b", http.StatusTeapot)
		case as.Valid(tok, srv.URL, []string{"a"}):
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer error="insufficient_scope", scope="a b", resource_metadata=%q`, prm))
			http.Error(w, "need b", http.StatusUnauthorized)
		default:
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer scope="a", resource_metadata=%q`, prm))
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		}
	})
	d := deps{poller: polling.Stream, cfgLoader: clicfg.LoadEmpty, authInteractive: func() bool { return true }, openURL: oauthtest.Browser(t), oauthDir: t.TempDir()}
	clients := fmt.Sprintf(`--oauth-clients={"authority":%q,"client_id":"cli-test"}`, as.URL)
	send := func() (signIns int) {
		t.Helper()
		_, stderr, _ := runCMDCapturingStderrWithConfig(t, d, "send", "-a", srv.URL, "--oauth", "auto", clients, oauthTestMessage)
		return strings.Count(stderr, "Authorized.")
	}
	send() // signs in for a, which the agent then refuses
	if got := send(); got != 1 || full.Load() != 1 {
		t.Fatalf("second a2a send: sign-ins = %d, requests with a and b = %d, want 1 and 1", got, full.Load())
	}
	if got := send(); got != 0 || full.Load() != 2 {
		t.Fatalf("third a2a send: sign-ins = %d, requests with a and b = %d, want 0 and 2", got, full.Load())
	}
	if got := storedResources(t, d.oauthDir); len(got) != 1 {
		t.Fatalf("stored grants = %v, want only the wider one", got)
	}
}

func TestFileIsTerminal(t *testing.T) {
	t.Parallel()
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = devNull.Close() })
	if fileIsTerminal(devNull) {
		t.Errorf("fileIsTerminal(%s) = true, want false", os.DevNull)
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close(); _ = w.Close() })
	if fileIsTerminal(w) {
		t.Errorf("fileIsTerminal(pipe) = true, want false")
	}
}

func TestServeProxyWithoutOAuth(t *testing.T) {
	t.Parallel()
	e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{ProtectCard: true})
	e.mustRun(t, "auth", "login", e.agent.URL, e.clients())
	sent := len(e.agent.AuthorizationHeaders())

	done := make(chan error, 1)
	go func() {
		_, _, err := e.run(t, "server", "--proxy", "-a", e.agent.URL, e.clients(), "--port", "0", "--quiet")
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "401") {
			t.Fatalf("a2a server --proxy error = %v, want the upstream card's 401", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("a2a server --proxy started with a stored OAuth grant; want it to call the upstream agent without OAuth")
	}
	for _, h := range e.agent.AuthorizationHeaders()[sent:] {
		if h != "" {
			t.Fatalf("the upstream agent received Authorization %q from the proxy, want none", h)
		}
	}
}

func TestAuthLoginProtectedCard(t *testing.T) {
	t.Parallel()
	e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{ProtectCard: true})
	e.mustRun(t, "auth", "login", e.agent.URL, e.clients())
	e.deps.authInteractive = func() bool { return false }
	e.mustRun(t, "card", "get", e.agent.URL, e.clients(), "--oauth", "cached")
}

func TestAuthLoginWithoutChallenge(t *testing.T) {
	t.Parallel()
	as := oauthtest.NewAuthServer(t, oauthtest.ServerOptions{})
	// The endpoint publishes resource metadata but takes requests without a token.
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"resource":%q,"authorization_servers":[%q]}`, srv.URL, as.URL)
	})
	mux.HandleFunc("/", http.NotFound)
	d := deps{poller: polling.Stream, cfgLoader: clicfg.LoadEmpty, authInteractive: func() bool { return true }, openURL: oauthtest.Browser(t), oauthDir: t.TempDir()}
	clients := fmt.Sprintf(`--oauth-clients={"authority":%q,"client_id":"cli-test"}`, as.URL)
	_, _, err := runCMDCapturingStderrWithConfig(t, d, "auth", "login", "-e", srv.URL, "--transport", "rest", clients)
	if got := errCode(err); got != clierr.CodeUsage || !strings.Contains(err.Error(), "does not advertise OAuth") {
		t.Fatalf("a2a auth login of an endpoint that doesn't answer 401 error = %v (code %s), want a usage error saying it doesn't use OAuth", err, got)
	}
	if as.Hits("/authorize") != 0 {
		t.Fatal("a sign-in started for an endpoint that doesn't ask for a token")
	}
}

func TestOAuthOmitResource(t *testing.T) {
	t.Parallel()
	e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL})
	clients := fmt.Sprintf(`--oauth-clients={"authority":%q,"client_id":"cli-test","omit_resource":true}`, e.as.URL)
	e.mustRun(t, "auth", "login", e.agent.URL, clients)
	if got := e.as.LastAuthorizeQuery().Get("resource"); got != "" {
		t.Fatalf("resource sent to the authorization endpoint = %q, want none with omit_resource", got)
	}
	if got := e.as.LastForm("/token").Get("resource"); got != "" {
		t.Fatalf("resource sent to the token endpoint = %q, want none with omit_resource", got)
	}
}

// putGrant stores a grant for resource with the endpoints of as.
func putGrant(t *testing.T, dir string, as *oauthtest.AuthServer, resource string) {
	t.Helper()
	ep := oauth.Endpoints{Issuer: as.URL, Authorization: as.URL + "/authorize", Token: as.URL + "/token", Revocation: as.URL + "/revoke"}
	g := oauth.Grant{Issuer: ep.Issuer, ClientID: "cli-test", Resource: oauth.NormalizeResource(resource), AccessToken: "at-" + resource, Endpoints: ep, ExpiresAt: time.Now().Add(time.Hour)}
	if err := (&oauthstore.FileStore{Dir: dir}).Put(t.Context(), g); err != nil {
		t.Fatal(err)
	}
}

func storedResources(t *testing.T, dir string) []string {
	t.Helper()
	grants, err := (&oauthstore.FileStore{Dir: dir}).List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, g := range grants {
		out = append(out, g.Resource)
	}
	slices.Sort(out)
	return out
}

// TestAuthAgentUnderPath covers an agent whose card is at the origin and whose
// interface is at /a2a/v1, next to a sibling agent on the same host and a grant
// for the whole host that both share.
func TestAuthAgentUnderPath(t *testing.T) {
	t.Parallel()
	e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL, BasePath: "/a2a/v1"})
	origin := strings.TrimSuffix(e.agent.URL, "/a2a/v1")
	cardURL := origin + "/.well-known/agent-card.json"
	sibling := origin + "/sibling/v1"
	e.mustRun(t, "auth", "login", origin, e.clients())
	putGrant(t, e.deps.oauthDir, e.as, sibling)
	putGrant(t, e.deps.oauthDir, e.as, origin)

	out, _ := e.mustRun(t, "auth", "get", cardURL, "-o", "json")
	var got struct {
		OAuth       []grantView `json:"oauth"`
		AlsoApplies []grantView `json:"alsoApplies"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json.Unmarshal(auth get output %q) error = %v", out, err)
	}
	if len(got.OAuth) != 1 || got.OAuth[0].Resource != e.agent.URL {
		t.Fatalf("auth get %s grants = %+v, want the one for its interface %s", cardURL, got.OAuth, e.agent.URL)
	}
	if len(got.AlsoApplies) != 1 || got.AlsoApplies[0].Resource != origin {
		t.Fatalf("auth get %s also-applies grants = %+v, want the one for %s", cardURL, got.AlsoApplies, origin)
	}

	out, stderr := e.mustRun(t, "auth", "logout", cardURL)
	if !strings.Contains(out, "Logged out of "+e.agent.URL) {
		t.Fatalf("auth logout %s output = %q, want the interface grant removed", cardURL, out)
	}
	if want := "a2a auth logout " + origin; !strings.Contains(stderr, "kept the grant for "+origin) || !strings.Contains(stderr, want) {
		t.Fatalf("auth logout %s stderr = %q, want a note naming the kept grant and %q", cardURL, stderr, want)
	}
	if diff := cmp.Diff([]string{origin, sibling}, storedResources(t, e.deps.oauthDir)); diff != "" {
		t.Fatalf("grants after auth logout %s (-want +got) diff = %s", cardURL, diff)
	}

	// Naming the host's grant exactly removes it, and only it.
	e.mustRun(t, "auth", "logout", origin)
	if diff := cmp.Diff([]string{sibling}, storedResources(t, e.deps.oauthDir)); diff != "" {
		t.Fatalf("grants after auth logout %s (-want +got) diff = %s", origin, diff)
	}
}

// TestAuthLogoutCardUnavailable checks that a host whose card can't be read
// only loses the grant for exactly that URL.
func TestAuthLogoutCardUnavailable(t *testing.T) {
	t.Parallel()
	as := oauthtest.NewAuthServer(t, oauthtest.ServerOptions{})
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "busy", http.StatusServiceUnavailable)
	}))
	t.Cleanup(down.Close)
	dir := t.TempDir()
	others := []string{down.URL + "/a2a/v1", down.URL + "/agents/b"}
	for _, r := range append([]string{down.URL}, others...) {
		putGrant(t, dir, as, r)
	}
	d := deps{poller: polling.Stream, cfgLoader: clicfg.LoadEmpty, oauthDir: dir}
	out, stderr, err := runCMDCapturingStderrWithConfig(t, d, "auth", "logout", down.URL, "-o", "json")
	if err != nil {
		t.Fatalf("a2a auth logout %s error = %v", down.URL, err)
	}
	var got struct {
		Removed []logoutView `json:"removed"`
		Kept    []grantView  `json:"kept"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", out, err)
	}
	if len(got.Removed) != 1 || got.Removed[0].Resource != down.URL {
		t.Fatalf("auth logout %s removed %+v, want only the grant for %s", down.URL, got.Removed, down.URL)
	}
	if len(got.Kept) != len(others) {
		t.Fatalf("auth logout %s kept %+v, want %v reported", down.URL, got.Kept, others)
	}
	if diff := cmp.Diff(others, storedResources(t, dir)); diff != "" {
		t.Fatalf("grants after auth logout %s (-want +got) diff = %s", down.URL, diff)
	}
	for _, r := range others {
		if !strings.Contains(stderr, "a2a auth logout "+r) {
			t.Errorf("auth logout %s stderr = %q, want how to remove %s", down.URL, stderr, r)
		}
	}
	if !strings.Contains(stderr, "--all") {
		t.Errorf("auth logout %s stderr = %q, want --all mentioned", down.URL, stderr)
	}
}

// originPRMAgent serves a card at the origin whose interface is /a2a/v1 and
// declares no OAuth; /a2a/v1 answers 401 and the protected resource metadata
// names the whole origin, as with one metadata document per host.
func originPRMAgent(t *testing.T, as *oauthtest.AuthServer) string {
	t.Helper()
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/agent-card.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"name":"x","description":"x","version":"1","capabilities":{},"defaultInputModes":["text"],"defaultOutputModes":["text"],"skills":[],"supportedInterfaces":[{"url":%q,"protocolBinding":"HTTP+JSON","protocolVersion":"1.0"}]}`, srv.URL+"/a2a/v1")
	})
	mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"resource":%q,"authorization_servers":[%q]}`, srv.URL, as.URL)
	})
	deny := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata=%q`, srv.URL+"/.well-known/oauth-protected-resource"))
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}
	mux.HandleFunc("/a2a/v1/", deny)
	mux.HandleFunc("/a2a/v1", deny)
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// TestAuthGetGrantsBelowRef checks that, for an agent URL with a path whose card
// is found, grants below that URL are the agent's too.
func TestAuthGetGrantsBelowRef(t *testing.T) {
	t.Parallel()
	as := oauthtest.NewAuthServer(t, oauthtest.ServerOptions{})
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/team", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"name":"x","description":"x","version":"1","capabilities":{},"defaultInputModes":["text"],"defaultOutputModes":["text"],"skills":[],"supportedInterfaces":[{"url":%q,"protocolBinding":"HTTP+JSON","protocolVersion":"1.0"}]}`, srv.URL+"/team/v1")
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	putGrant(t, dir, as, srv.URL+"/team/v1")
	putGrant(t, dir, as, srv.URL+"/team/v2")
	putGrant(t, dir, as, srv.URL+"/other")
	d := deps{poller: polling.Stream, cfgLoader: clicfg.LoadEmpty, oauthDir: dir}
	out, _, err := runCMDCapturingStderrWithConfig(t, d, "auth", "get", srv.URL+"/team", "-o", "json")
	if err != nil {
		t.Fatalf("a2a auth get error = %v", err)
	}
	var got struct {
		OAuth []grantView `json:"oauth"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", out, err)
	}
	var resources []string
	for _, g := range got.OAuth {
		resources = append(resources, g.Resource)
	}
	slices.Sort(resources)
	if diff := cmp.Diff([]string{srv.URL + "/team/v1", srv.URL + "/team/v2"}, resources); diff != "" {
		t.Fatalf("auth get %s grants (-want +got) diff = %s", srv.URL+"/team", diff)
	}
}

func TestAuthLoginThenLogoutSameRef(t *testing.T) {
	t.Parallel()
	e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{})
	origin := originPRMAgent(t, e.as)
	e.mustRun(t, "auth", "login", origin, e.clients())
	if diff := cmp.Diff([]string{origin}, storedResources(t, e.deps.oauthDir)); diff != "" {
		t.Fatalf("grants after auth login %s (-want +got) diff = %s", origin, diff)
	}
	if out, _ := e.mustRun(t, "auth", "get", origin, "-o", "json"); !strings.Contains(out, `"oauth": [`+"\n") {
		t.Fatalf("auth get %s = %s, want the grant listed as the agent's", origin, out)
	}
	e.mustRun(t, "auth", "logout", origin)
	if got := storedResources(t, e.deps.oauthDir); len(got) != 0 {
		t.Fatalf("grants after auth logout %s = %v, want the one auth login stored removed", origin, got)
	}
}

func TestAuthLoginAgentRejectsGET(t *testing.T) {
	t.Parallel()
	e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{})
	// The agent routes before it authenticates: a GET of the interface is a
	// 405, and only an A2A call gets the Bearer challenge.
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"resource":%q,"authorization_servers":[%q]}`, srv.URL, e.as.URL)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok && e.as.Valid(token, srv.URL, nil) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"error":{"code":-32001,"message":"task not found"}}`)
			return
		}
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata=%q`, srv.URL+"/.well-known/oauth-protected-resource"))
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	for _, attempt := range []string{"first sign-in", "sign-in with a grant stored"} {
		_, stderr := e.mustRun(t, "auth", "login", "-e", srv.URL, "--transport", "jsonrpc", e.clients())
		if !strings.Contains(stderr, "Authorized. Grant saved for "+srv.URL) {
			t.Fatalf("auth login (%s) stderr = %q, want a grant saved for %s", attempt, stderr, srv.URL)
		}
	}
}

func TestOAuthFlagsIgnoredFromDiscoveredEnv(t *testing.T) {
	t.Parallel()
	e := newOAuthEnv(t, oauthtest.ServerOptions{CIMD: true}, oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL})
	// A fake keychain, so the test can't reach the real one if the .env were read.
	kr := &memKeyring{}
	e.deps.keyring = kr
	e.mustRun(t, "auth", "login", e.agent.URL)
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	env := fmt.Sprintf("A2ACLI_OAUTH=auto\nA2ACLI_OAUTH_CLIENTS={\"authority\":%q,\"client_id\":\"from-env\"}\nA2ACLI_OAUTH_DEFAULT_CLIENT_ID=https://evil.example/c.json\nA2ACLI_OAUTH_STORE=keychain\n", e.as.URL)
	if err := os.WriteFile(envPath, []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	e.deps.cfgLoader = func(opts clicfg.LoadOpts) (*clicfg.Store, error) {
		opts.WorkingDir = dir
		opts.GlobalPath = filepath.Join(dir, "none.env")
		opts.UserPath = filepath.Join(dir, "none.yaml")
		opts.LookupEnv = func(string) (string, bool) { return "", false }
		return clicfg.Load(opts)
	}

	// A2ACLI_OAUTH=auto from the .env doesn't turn OAuth on: the stored grant is not sent.
	sent := len(e.agent.AuthorizationHeaders())
	_, stderr, err := e.runDefault(t, "send", "-a", e.agent.URL, oauthTestMessage)
	if err == nil {
		t.Fatal("a2a send with A2ACLI_OAUTH=auto in a discovered .env error = nil, want the agent's 401 with OAuth off")
	}
	for _, h := range e.agent.AuthorizationHeaders()[sent:] {
		if h != "" {
			t.Fatalf("the agent received Authorization %q, want no grant sent", h)
		}
	}
	for _, flag := range []string{"oauth", "oauth-clients", "oauth-default-client-id", "oauth-store"} {
		if want := "warning: ignoring " + flag + " from " + envPath; !strings.Contains(stderr, want) {
			t.Errorf("a2a send stderr = %q, want %q", stderr, want)
		}
	}

	// Neither the client registration nor the default client ID from the .env is used.
	e.mustRun(t, "auth", "logout", "--all")
	e.mustRun(t, "auth", "login", e.agent.URL)
	if got := e.as.LastAuthorizeQuery().Get("client_id"); got != oauthstore.DefaultClientID {
		t.Fatalf("client_id = %q, want %q: the .env must not choose the client", got, oauthstore.DefaultClientID)
	}
	// Nor is the store: the grant went to grants.json, not the keychain.
	if n := len(kr.secrets); n != 0 {
		t.Fatalf("keychain entries = %d, want none: the .env must not choose the store", n)
	}
	if _, err := os.Stat(filepath.Join(e.deps.oauthDir, "grants.json")); err != nil {
		t.Fatalf("grants.json after auth login: %v, want the file store used", err)
	}
}

func TestOAuthSignInInterrupted(t *testing.T) {
	t.Parallel()
	e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL})
	e.deps.openURL = func(string) {}
	e.deps.notifyInterrupt = func(ctx context.Context) (context.Context, context.CancelFunc) {
		ctx, cancel := context.WithCancel(ctx)
		cancel() // Ctrl-C right away
		return ctx, cancel
	}
	_, _, err := e.run(t, "auth", "login", e.agent.URL, e.clients())
	if got := errCode(err); got != clierr.CodeAuthFailed || !strings.Contains(err.Error(), "interrupted") {
		t.Fatalf("a2a auth login interrupted error = %v (code %s), want %s saying the sign-in was interrupted", err, got, clierr.CodeAuthFailed)
	}
}

func TestOAuthNoClientForInlineFlow(t *testing.T) {
	t.Parallel()
	e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{Card: oauthtest.CardAuthCode})
	_, _, err := e.run(t, "send", "-a", e.agent.URL, oauthTestMessage)
	if got := errCode(err); got != clierr.CodeUsage {
		t.Fatalf("a2a send error = %v (code %s), want %s", err, got, clierr.CodeUsage)
	}
	msg := err.Error()
	if !strings.Contains(msg, "token endpoint is "+e.as.URL+"/token") || strings.Contains(msg, `"authority":"`+e.as.URL+`"`) {
		t.Fatalf("a2a send error = %q, want it to name the token endpoint and not suggest its origin as the authority", msg)
	}
}

func TestAuthGetNothingStored(t *testing.T) {
	t.Parallel()
	e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL})
	_, _, err := e.run(t, "auth", "get", e.agent.URL)
	if got := errCode(err); got != clierr.CodeCredentialsMissing {
		t.Fatalf("a2a auth get with nothing stored error = %v (code %s), want %s", err, got, clierr.CodeCredentialsMissing)
	}

	e.mustRun(t, "auth", "login", e.agent.URL, e.clients())
	out, _ := e.mustRun(t, "auth", "get", e.agent.URL, "-o", "json")
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"url", "helper", "headers", "expires", "oauth", "alsoApplies"} {
		if _, ok := m[key]; !ok {
			t.Errorf("auth get JSON = %s, want key %q present", out, key)
		}
	}
	if string(m["alsoApplies"]) != "[]" {
		t.Errorf("auth get alsoApplies = %s, want []", m["alsoApplies"])
	}
}

func TestOAuthLeavesExplicitCardAuthorization(t *testing.T) {
	t.Parallel()
	e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{ProtectCard: true})
	_, stderr, err := e.run(t, "card", "get", e.agent.URL, e.clients(), "--auth", "Bearer user-supplied")
	if got := errCode(err); got != clierr.CodeCardInvalid {
		t.Fatalf("a2a card get --auth error = %v (code %s), want the card's 401 (%s)", err, got, clierr.CodeCardInvalid)
	}
	if strings.Contains(stderr, "To authorize") || e.as.Hits("/authorize") != 0 {
		t.Fatalf("a sign-in started for a card request that carried --auth; stderr = %q", stderr)
	}
}

// TestAuthLogoutIgnoresOtherOrigins checks that logout follows only the card
// interfaces on the origin of the URL passed: a card can name any URL, and
// following it would remove another agent's grant.
func TestAuthLogoutIgnoresOtherOrigins(t *testing.T) {
	t.Parallel()
	as := oauthtest.NewAuthServer(t, oauthtest.ServerOptions{})
	other := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(other.Close)
	mux := http.NewServeMux()
	cardServer := httptest.NewServer(mux)
	t.Cleanup(cardServer.Close)
	card := newAgentCard(cardServer.URL, a2a.AgentCapabilities{})
	card.SupportedInterfaces = append(card.SupportedInterfaces, a2a.NewAgentInterface(other.URL, a2a.TransportProtocolHTTPJSON))
	mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(card))
	dir := t.TempDir()
	putGrant(t, dir, as, cardServer.URL)
	putGrant(t, dir, as, other.URL)
	d := deps{poller: polling.Stream, cfgLoader: clicfg.LoadEmpty, oauthDir: dir}
	if _, stderr, err := runCMDCapturingStderrWithConfig(t, d, "auth", "logout", cardServer.URL); err != nil {
		t.Fatalf("a2a auth logout error = %v\nstderr:\n%s", err, stderr)
	}
	if diff := cmp.Diff([]string{other.URL}, storedResources(t, dir)); diff != "" {
		t.Fatalf("grants left after logging out of %s (-want +got) diff = %s", cardServer.URL, diff)
	}
}

// TestOAuthPlainUnauthenticated checks that a 401 without a Bearer challenge to
// a call that carried a stored grant is returned as the agent's error, without
// a refresh or a claim that the grant was rejected.
func TestOAuthPlainUnauthenticated(t *testing.T) {
	t.Parallel()
	as := oauthtest.NewAuthServer(t, oauthtest.ServerOptions{})
	var agentURL string
	mux := http.NewServeMux()
	agent := httptest.NewServer(mux)
	t.Cleanup(agent.Close)
	agentURL = agent.URL
	mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(newAgentCard(agentURL, a2a.AgentCapabilities{})))
	var auth []string
	var mu sync.Mutex
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		auth = append(auth, r.Header.Get("Authorization"))
		mu.Unlock()
		http.Error(w, "UNAUTHENTICATED", http.StatusUnauthorized)
	})
	dir := t.TempDir()
	putGrant(t, dir, as, agentURL)
	d := deps{poller: polling.Stream, cfgLoader: clicfg.LoadEmpty, oauthDir: dir, authInteractive: func() bool { return false }}
	_, stderr, err := runCMDCapturingStderrWithConfig(t, d, "--oauth", "cached", "task", "list", "-a", agentURL)
	if err == nil {
		t.Fatal("a2a task list error = nil, want the agent's 401")
	}
	if code := errCode(err); code == clierr.CodeAuthFailed || code == clierr.CodeCredentialsMissing || strings.Contains(err.Error(), "rejected") {
		t.Fatalf("a2a task list error = %v (code %s), want the agent's 401 as it is\nstderr:\n%s", err, code, stderr)
	}
	if hits := as.Hits("/token"); hits != 0 {
		t.Fatalf("token endpoint hits = %d, want no refresh", hits)
	}
	mu.Lock()
	defer mu.Unlock()
	if diff := cmp.Diff([]string{"Bearer at-" + agentURL}, auth); diff != "" {
		t.Fatalf("Authorization the agent received (-want +got) diff = %s", diff)
	}
}

// TestOAuthNotOverInsecureGRPCFromEnv checks that a discovered .env that turns
// on --insecure can't make a2a send a stored token over plaintext gRPC, while
// --insecure on the command line still can.
func TestOAuthNotOverInsecureGRPCFromEnv(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var auth []string
	record := func(ctx context.Context) {
		md, _ := metadata.FromIncomingContext(ctx)
		mu.Lock()
		auth = append(auth, strings.Join(md.Get("authorization"), ","))
		mu.Unlock()
	}
	srv := grpc.NewServer(
		grpc.UnaryInterceptor(func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
			record(ctx)
			return h(ctx, req)
		}),
		grpc.StreamInterceptor(func(s any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, h grpc.StreamHandler) error {
			record(ss.Context())
			return h(s, ss)
		}),
	)
	t.Cleanup(srv.Stop)
	a2agrpcv1.NewHandler(a2asrv.NewHandler(localsrv.NewEchoExecutor())).RegisterWith(srv)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(lis) }()
	addr := lis.Addr().String()

	as := oauthtest.NewAuthServer(t, oauthtest.ServerOptions{})
	oauthDir := t.TempDir()
	putGrant(t, oauthDir, as, addr)
	work := t.TempDir()
	if err := os.WriteFile(filepath.Join(work, ".env"), []byte("A2ACLI_INSECURE=true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	loader := cfgLoaderFunc(func(opts clicfg.LoadOpts) (*clicfg.Store, error) {
		opts.WorkingDir = work
		opts.GlobalPath = filepath.Join(work, "none.env")
		opts.UserPath = filepath.Join(work, "none.yaml")
		opts.LookupEnv = func(string) (string, bool) { return "", false }
		return clicfg.Load(opts)
	})
	d := deps{poller: polling.Stream, cfgLoader: loader, oauthDir: oauthDir}
	run := func(extra ...string) string {
		t.Helper()
		args := append([]string{"--oauth", "cached", "send", "-e", addr, "--transport", "grpc", "hello"}, extra...)
		_, stderr, err := runCMDCapturingStderrWithConfig(t, d, args...)
		if err != nil {
			t.Fatalf("a2a %s error = %v\nstderr:\n%s", strings.Join(args, " "), err, stderr)
		}
		return stderr
	}
	stderr := run()
	if !strings.Contains(stderr, "not sending OAuth tokens over plaintext gRPC") {
		t.Fatalf("stderr = %q, want a warning that the token was not sent", stderr)
	}
	run("--insecure")
	mu.Lock()
	defer mu.Unlock()
	if diff := cmp.Diff([]string{"", "Bearer at-" + addr}, auth); diff != "" {
		t.Fatalf("Authorization the agent received (-want +got) diff = %s", diff)
	}
}

// TestOAuthCredentialHelperOnlyAuthorization checks that with a credential
// helper for the agent's host and a stored grant, the agent receives only the
// helper's Authorization: the helper runs before OAuth, which leaves a call
// that carries one alone.
func TestOAuthCredentialHelperOnlyAuthorization(t *testing.T) {
	t.Parallel()
	helper := credentialHelper(t)
	var mu sync.Mutex
	var auth [][]string
	capabilities := a2a.AgentCapabilities{}
	mux := http.NewServeMux()
	mux.Handle("/", a2asrv.NewRESTHandler(a2asrv.NewHandler(localsrv.NewEchoExecutor(), a2asrv.WithCapabilityChecks(&capabilities))))
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != a2asrv.WellKnownAgentCardPath {
			mu.Lock()
			auth = append(auth, r.Header.Values("Authorization"))
			mu.Unlock()
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(agent.Close)
	mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(newAgentCard(agent.URL, capabilities)))
	as := oauthtest.NewAuthServer(t, oauthtest.ServerOptions{})
	dir := t.TempDir()
	putGrant(t, dir, as, agent.URL)
	d := deps{poller: polling.Stream, cfgLoader: clicfg.LoadEmpty, oauthDir: dir}
	args := []string{"--oauth", "cached", "send", "-a", agent.URL, "--credential-helper", "127.0.0.1=" + helper, oauthTestMessage}
	if _, stderr, err := runCMDCapturingStderrWithConfig(t, d, args...); err != nil {
		t.Fatalf("a2a %s error = %v\nstderr:\n%s", strings.Join(args, " "), err, stderr)
	}
	mu.Lock()
	defer mu.Unlock()
	if diff := cmp.Diff([][]string{{"Bearer " + agent.URL}}, auth); diff != "" {
		t.Fatalf("Authorization values the agent received (-want +got) diff = %s", diff)
	}
}

// TestOAuthRefreshRetry checks, through the CLI, that a token the agent rejects
// is refreshed and the call repeated: for a streaming call, whose rejection
// comes before the first event, and over gRPC, whose errors carry the
// response metadata.
func TestOAuthRefreshRetry(t *testing.T) {
	t.Parallel()
	t.Run("stream", func(t *testing.T) {
		t.Parallel()
		e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL})
		e.mustRun(t, "auth", "login", e.agent.URL, e.clients())
		e.as.Invalidate()
		tokens := e.as.Hits("/token")
		out, _ := e.mustRun(t, "send", "--stream", "-a", e.agent.URL, e.clients(), "--oauth", "cached", oauthTestMessage)
		if !strings.Contains(out, oauthTestMessage) || e.as.Hits("/token") != tokens+1 {
			t.Fatalf("a2a send --stream after the token was invalidated = %q, token requests %d, want the reply after one refresh", out, e.as.Hits("/token")-tokens)
		}
	})
	t.Run("gRPC", func(t *testing.T) {
		t.Parallel()
		as := oauthtest.NewAuthServer(t, oauthtest.ServerOptions{})
		var addr string
		var mu sync.Mutex
		var auth []string
		check := func(ctx context.Context) error {
			md, _ := metadata.FromIncomingContext(ctx)
			token, _ := strings.CutPrefix(strings.Join(md.Get("authorization"), ""), "Bearer ")
			mu.Lock()
			auth = append(auth, token)
			mu.Unlock()
			if as.Valid(token, oauth.NormalizeResource(addr), nil) {
				return nil
			}
			_ = grpc.SetHeader(ctx, metadata.Pairs("www-authenticate", `Bearer error="invalid_token"`))
			return status.Error(codes.Unauthenticated, "invalid token")
		}
		srv := grpc.NewServer(
			grpc.UnaryInterceptor(func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
				if err := check(ctx); err != nil {
					return nil, err
				}
				return h(ctx, req)
			}),
			grpc.StreamInterceptor(func(s any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, h grpc.StreamHandler) error {
				if err := check(ss.Context()); err != nil {
					return err
				}
				return h(s, ss)
			}),
		)
		t.Cleanup(srv.Stop)
		a2agrpcv1.NewHandler(a2asrv.NewHandler(localsrv.NewEchoExecutor())).RegisterWith(srv)
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		go func() { _ = srv.Serve(lis) }()
		addr = lis.Addr().String()

		d := deps{poller: polling.Stream, cfgLoader: clicfg.LoadEmpty, oauthDir: t.TempDir(), authInteractive: func() bool { return true }, openURL: oauthtest.Browser(t)}
		clients := fmt.Sprintf(`--oauth-clients={"authority":%q,"client_id":"cli-test"}`, as.URL)
		// Sign in for the gRPC address directly: discovery after a 401 needs an
		// https or loopback URL, which a gRPC address is not.
		r := &oauth.Requirement{Endpoints: oauth.Endpoints{Issuer: as.URL, Authorization: as.AuthorizationURL(), Token: as.URL + "/token"}, Resource: oauth.NormalizeResource(addr)}
		m := &oauth.Manager{Clients: []oauth.Registration{{Authority: as.URL, ClientID: "cli-test"}}, Store: &oauthstore.FileStore{Dir: d.oauthDir}}
		if _, err := m.SignIn(t.Context(), r, oauth.SignInOptions{ShowURL: oauthtest.Browser(t)}); err != nil {
			t.Fatalf("signing in for %s error = %v", addr, err)
		}
		as.Invalidate()
		tokens := as.Hits("/token")
		args := []string{"--oauth", "cached", "--insecure", "send", "-e", addr, "--transport", "grpc", clients, oauthTestMessage}
		out, stderr, err := runCMDCapturingStderrWithConfig(t, d, args...)
		if err != nil || !strings.Contains(out, oauthTestMessage) {
			t.Fatalf("a2a %s = %q, %v\nstderr:\n%s", strings.Join(args, " "), out, err, stderr)
		}
		mu.Lock()
		defer mu.Unlock()
		if as.Hits("/token") != tokens+1 || len(auth) != 2 || auth[0] == auth[1] {
			t.Fatalf("token requests = %d, tokens the agent received = %q, want one refresh and the call repeated with the new token", as.Hits("/token")-tokens, auth)
		}
	})
}

// TestAuthLogoutSameOriginSibling checks that logout doesn't remove the grant
// of another agent on the same host when the agent's card lists that agent's
// URL: on a host that routes agents by path, a card can name any of them. The
// sibling's grant is kept and listed.
func TestAuthLogoutSameOriginSibling(t *testing.T) {
	t.Parallel()
	as := oauthtest.NewAuthServer(t, oauthtest.ServerOptions{})
	mux := http.NewServeMux()
	host := httptest.NewServer(mux)
	t.Cleanup(host.Close)
	a, b := host.URL+"/agents/a", host.URL+"/agents/b"
	card := newAgentCard(a, a2a.AgentCapabilities{})
	card.SupportedInterfaces = append(card.SupportedInterfaces, a2a.NewAgentInterface(b, a2a.TransportProtocolHTTPJSON))
	// A reference with a path is fetched as it is.
	mux.Handle("/agents/a", a2asrv.NewStaticAgentCardHandler(card))
	dir := t.TempDir()
	putGrant(t, dir, as, a)
	putGrant(t, dir, as, b)
	d := deps{poller: polling.Stream, cfgLoader: clicfg.LoadEmpty, oauthDir: dir}
	_, stderr, err := runCMDCapturingStderrWithConfig(t, d, "auth", "logout", a)
	if err != nil {
		t.Fatalf("a2a auth logout error = %v\nstderr:\n%s", err, stderr)
	}
	if diff := cmp.Diff([]string{b}, storedResources(t, dir)); diff != "" {
		t.Fatalf("grants left after logging out of %s (-want +got) diff = %s", a, diff)
	}
	if !strings.Contains(stderr, "kept the grant for "+b) {
		t.Fatalf("stderr = %q, want a note that the grant for %s was kept", stderr, b)
	}
}

// TestOAuthCLIErrors checks the CLI's handling of a few OAuth inputs: duplicate
// client registrations are a usage error naming them, auth logout refuses an
// argument that isn't an http(s) URL, and a credential helper scoped to the host
// of a gRPC address counts as an explicit credential.
func TestOAuthCLIErrors(t *testing.T) {
	t.Parallel()
	t.Run("ambiguous client registrations", func(t *testing.T) {
		t.Parallel()
		e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL})
		dup := fmt.Sprintf(`--oauth-clients=[{"authority":%q,"client_id":"one"},{"authority":%q,"client_id":"two"}]`, e.as.URL, e.as.URL)
		_, _, err := e.run(t, "send", "-a", e.agent.URL, dup, oauthTestMessage)
		if code := errCode(err); code != clierr.CodeUsage || !strings.Contains(err.Error(), "one, two") {
			t.Fatalf("a2a send with duplicate registrations error = %v (code %s), want a usage error naming both", err, code)
		}
	})
	t.Run("logout of something that isn't a URL", func(t *testing.T) {
		t.Parallel()
		d := deps{poller: polling.Stream, cfgLoader: clicfg.LoadEmpty, oauthDir: t.TempDir()}
		_, _, err := runCMDCapturingStderrWithConfig(t, d, "auth", "logout", "http://[::1")
		if code := errCode(err); code != clierr.CodeUsage {
			t.Fatalf("a2a auth logout http://[::1 error = %v (code %s), want a usage error", err, code)
		}
	})
	t.Run("credential helper for a gRPC address", func(t *testing.T) {
		t.Parallel()
		helper := credentialHelper(t)
		cfg := &globalConfig{svcParams: &flagparse.ServiceParams{}}
		if err := cfg.credentialHelpers.Set("127.0.0.1=" + helper); err != nil {
			t.Fatal(err)
		}
		cfg.initCredentials()
		if !cfg.explicitCredentials("127.0.0.1:50051") {
			t.Fatal("explicitCredentials(127.0.0.1:50051) = false, want true for a helper scoped to 127.0.0.1")
		}
	})
}
