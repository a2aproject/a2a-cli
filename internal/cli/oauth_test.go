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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/a2aproject/a2a-cli/internal/clicfg"
	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-cli/internal/oauth"
	"github.com/a2aproject/a2a-cli/internal/oauth/oauthtest"
	"github.com/a2aproject/a2a-cli/internal/polling"
	"github.com/a2aproject/a2a-cli/internal/testutil"
	"github.com/a2aproject/a2a-go/v2/a2a"
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
			authSleep:       func(context.Context, time.Duration) error { return nil },
		},
	}
}

func (e *oauthEnv) clients() string {
	return fmt.Sprintf(`--oauth-clients={"authority":%q,"client_id":"cli-test"}`, e.as.URL)
}

func (e *oauthEnv) run(t *testing.T, args ...string) (stdout, stderr string, err error) {
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
		if got := e.as.LastAuthorizeQuery().Get("client_id"); got != oauth.DefaultClientID {
			t.Fatalf("client_id = %q, want %q", got, oauth.DefaultClientID)
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
		{"auth", "list", "--oauth", "off"},
		{"send", "-a", e.agent.URL, "--oauth", "sometimes", "x"},
		{"send", "-a", e.agent.URL, "--oauth-clients", "not json", "x"},
	}
	for _, args := range tests {
		if _, _, err := e.run(t, args...); errCode(err) != clierr.CodeUsage {
			t.Errorf("a2a %s error = %v, want a usage error", strings.Join(args, " "), err)
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
			args := append([]string{"send", "-a", agent, "--svc-param", "X-API-Key=k1", "hi"}, extra...)
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
		_, _, err := runCMDCapturingStderrWithConfig(t, d, "send", "-e", srv.URL, "--transport", "rest", "hi")
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
	store := &oauth.FileStore{Dir: dir}
	ep := oauth.Endpoints{Issuer: "https://as.example.com", Authorization: "https://as.example.com/authorize", Token: "https://as.example.com/token"}
	for _, r := range []string{"https://gw.example.com/agents/a", "https://gw.example.com/agents/b"} {
		g := oauth.Grant{Key: oauth.NewGrantKey(ep.Issuer, "c", r, nil), AccessToken: "at", Endpoints: ep, ExpiresAt: time.Now().Add(time.Hour)}
		if err := store.Put(g); err != nil {
			t.Fatal(err)
		}
	}
	d := deps{poller: polling.Stream, cfgLoader: clicfg.LoadEmpty, oauthDir: dir}
	if _, _, err := runCMDCapturingStderrWithConfig(t, d, "auth", "logout", "https://gw.example.com/agents/a"); err != nil {
		t.Fatal(err)
	}
	left, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 || left[0].Key.Resource != "https://gw.example.com/agents/b" {
		t.Fatalf("grants after logging out of /agents/a = %+v, want only /agents/b", left)
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
