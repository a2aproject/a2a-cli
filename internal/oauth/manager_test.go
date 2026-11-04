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

package oauth_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/a2aproject/a2a-cli/internal/oauth"
	"github.com/a2aproject/a2a-cli/internal/oauth/oauthtest"
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
)

func newManager(t *testing.T, as *oauthtest.AuthServer) *oauth.Manager {
	t.Helper()
	return &oauth.Manager{
		Clients:         []oauth.Registration{{Authority: as.URL, ClientID: "test-client"}},
		DefaultClientID: oauth.DefaultClientID,
		Store:           &oauth.FileStore{Dir: t.TempDir()},
		HTTPClient:      http.DefaultClient,
	}
}

func fetchCard(t *testing.T, agentURL string) *a2a.AgentCard {
	t.Helper()
	card, err := agentcard.DefaultResolver.Resolve(t.Context(), agentURL)
	if err != nil {
		t.Fatalf("Resolve(%q) error = %v", agentURL, err)
	}
	return card
}

func noSleep(context.Context, time.Duration) error { return nil }

func TestRequirementForCard(t *testing.T) {
	t.Parallel()
	as := oauthtest.NewAuthServer(t, oauthtest.ServerOptions{})
	full := oauth.Endpoints{
		Issuer:              as.URL,
		Authorization:       as.URL + "/authorize",
		Token:               as.URL + "/token",
		DeviceAuthorization: as.URL + "/device",
		Revocation:          as.URL + "/revoke",
	}
	tests := []struct {
		name string
		card oauthtest.CardSecurity
		want *oauth.Requirement
	}{
		{name: "no security", card: oauthtest.CardNone},
		{name: "metadata url", card: oauthtest.CardMetadataURL, want: &oauth.Requirement{Endpoints: full, Scopes: []string{"a2a.invoke"}}},
		{name: "openid connect", card: oauthtest.CardOIDC, want: &oauth.Requirement{Endpoints: full, Scopes: []string{"a2a.invoke"}}},
		{
			name: "inline authorization code",
			card: oauthtest.CardAuthCode,
			want: &oauth.Requirement{Endpoints: oauth.Endpoints{Issuer: as.URL, Authorization: as.URL + "/authorize", Token: as.URL + "/token"}, Scopes: []string{"a2a.invoke"}},
		},
		{
			name: "inline device code",
			card: oauthtest.CardDeviceCode,
			want: &oauth.Requirement{Endpoints: oauth.Endpoints{Issuer: as.URL, DeviceAuthorization: as.URL + "/device", Token: as.URL + "/token"}, Scopes: []string{"a2a.invoke"}, PreferDevice: true},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			agent := oauthtest.NewProtectedAgent(t, as, oauthtest.AgentOptions{Card: tc.card, Scopes: []string{"a2a.invoke"}})
			m := newManager(t, as)
			got, err := m.RequirementForCard(t.Context(), fetchCard(t, agent.URL), agent.URL+"/")
			if err != nil {
				t.Fatalf("Manager.RequirementForCard() error = %v", err)
			}
			if tc.want != nil {
				tc.want.Resource = agent.URL
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("Manager.RequirementForCard() wrong result (-want +got) diff = %s", diff)
			}
		})
	}
}

func TestRequirementForChallenge(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		agent     oauthtest.AgentOptions
		clients   func(as *oauthtest.AuthServer) []oauth.Registration
		server    oauthtest.ServerOptions
		noDefault bool
		wantErr   bool
		noClient  bool
	}{
		{name: "resource_metadata in the challenge", agent: oauthtest.AgentOptions{Scopes: []string{"a2a.invoke"}}},
		{name: "well-known probe", agent: oauthtest.AgentOptions{OmitResourceMetadata: true}},
		{name: "OIDC discovery only", server: oauthtest.ServerOptions{OIDCOnly: true}},
		{
			name:     "no client",
			clients:  func(*oauthtest.AuthServer) []oauth.Registration { return nil },
			wantErr:  true,
			noClient: true,
		},
		{
			name:    "CIMD fallback",
			clients: func(*oauthtest.AuthServer) []oauth.Registration { return nil },
			server:  oauthtest.ServerOptions{CIMD: true},
		},
		{
			name:      "CIMD fallback disabled",
			clients:   func(*oauthtest.AuthServer) []oauth.Registration { return nil },
			server:    oauthtest.ServerOptions{CIMD: true},
			noDefault: true,
			wantErr:   true,
			noClient:  true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			as := oauthtest.NewAuthServer(t, tc.server)
			agent := oauthtest.NewProtectedAgent(t, as, tc.agent)
			m := newManager(t, as)
			if tc.clients != nil {
				m.Clients = tc.clients(as)
			}
			if tc.noDefault {
				m.DefaultClientID = ""
			}
			resp, err := http.Post(agent.URL+"/message:send", "application/json", strings.NewReader("{}"))
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			target, _ := url.Parse(agent.URL + "/message:send")
			got, err := m.RequirementForChallenge(t.Context(), target, oauth.ParseChallenges(resp.Header.Values("WWW-Authenticate")))
			if (err != nil) != tc.wantErr {
				t.Fatalf("Manager.RequirementForChallenge() error = %v, want error %v", err, tc.wantErr)
			}
			var nc *oauth.NoClientError
			if errors.As(err, &nc) != tc.noClient {
				t.Fatalf("Manager.RequirementForChallenge() error = %v, want NoClientError %v", err, tc.noClient)
			}
			if tc.wantErr {
				return
			}
			if got.Resource != agent.URL || got.Endpoints.Issuer != as.URL || got.Endpoints.Token != as.URL+"/token" {
				t.Fatalf("Manager.RequirementForChallenge() = %+v, want resource %s at %s", got, agent.URL, as.URL)
			}
			if diff := cmp.Diff(tc.agent.Scopes, got.Scopes); diff != "" {
				t.Fatalf("Manager.RequirementForChallenge() scopes wrong result (-want +got) diff = %s", diff)
			}
		})
	}
}

func TestRequirementForChallengeRejectsBadMetadata(t *testing.T) {
	t.Parallel()
	as := oauthtest.NewAuthServer(t, oauthtest.ServerOptions{})
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"resource":"http://elsewhere.example","authorization_servers":["` + as.URL + `"]}`))
	}))
	t.Cleanup(other.Close)
	tests := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{
			name: "cross-origin redirect",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, other.URL+"/prm", http.StatusFound)
			},
		},
		{
			name: "metadata for another resource",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"resource":"http://elsewhere.example","authorization_servers":["` + as.URL + `"]}`))
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(tc.handler)
			t.Cleanup(srv.Close)
			target, _ := url.Parse(srv.URL + "/a2a")
			chs := oauth.ParseChallenges([]string{`Bearer resource_metadata="` + srv.URL + `/prm"`})
			_, err := newManager(t, as).RequirementForChallenge(t.Context(), target, chs)
			var fe *oauth.FailureError
			if !errors.As(err, &fe) {
				t.Fatalf("Manager.RequirementForChallenge() error = %v, want FailureError", err)
			}
		})
	}
}

func TestAcquire(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		server oauthtest.ServerOptions
		device bool
		path   string
	}{
		{name: "authorization code", path: "/token"},
		{name: "device flow requested", device: true, path: "/device"},
		{name: "device flow without authorization endpoint", server: oauthtest.ServerOptions{NoAuthorizationEndpoint: true, PendingPolls: 2}, path: "/device"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			as := oauthtest.NewAuthServer(t, tc.server)
			agent := oauthtest.NewProtectedAgent(t, as, oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL, Scopes: []string{"a2a.invoke"}})
			m := newManager(t, as)
			req, err := m.RequirementForCard(t.Context(), fetchCard(t, agent.URL), agent.URL)
			if err != nil {
				t.Fatal(err)
			}
			var prompt bytes.Buffer
			g, err := m.Acquire(t.Context(), req, oauth.AcquireOptions{
				Device:   tc.device,
				Prompter: oauth.Prompter{Out: &prompt, Open: oauthtest.Browser(t)},
				Sleep:    noSleep,
			})
			if err != nil {
				t.Fatalf("Manager.Acquire() error = %v", err)
			}
			if !as.Valid(g.AccessToken, agent.URL, []string{"a2a.invoke"}) {
				t.Fatalf("Manager.Acquire() returned a token the agent does not accept")
			}
			if !strings.Contains(prompt.String(), "To authorize, open:") {
				t.Fatalf("Manager.Acquire() prompt = %q, want authorization instructions", prompt.String())
			}
			if got := as.LastForm(tc.path).Get("resource"); got != agent.URL {
				t.Fatalf("resource sent to %s = %q, want %q", tc.path, got, agent.URL)
			}
			stored, err := m.Grant(t.Context(), req)
			if err != nil || stored == nil || stored.AccessToken != g.AccessToken {
				t.Fatalf("Manager.Grant() after Acquire = %v, %v, want the acquired grant", stored, err)
			}
		})
	}
}

func TestAcquireDenied(t *testing.T) {
	t.Parallel()
	for _, device := range []bool{false, true} {
		as := oauthtest.NewAuthServer(t, oauthtest.ServerOptions{DenyAuthorization: true})
		agent := oauthtest.NewProtectedAgent(t, as, oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL})
		m := newManager(t, as)
		req, err := m.RequirementForCard(t.Context(), fetchCard(t, agent.URL), agent.URL)
		if err != nil {
			t.Fatal(err)
		}
		_, err = m.Acquire(t.Context(), req, oauth.AcquireOptions{Device: device, Prompter: oauth.Prompter{Open: oauthtest.Browser(t)}, Sleep: noSleep})
		var oe *oauth.Error
		if !errors.As(err, &oe) || oe.Code != "access_denied" {
			t.Fatalf("Manager.Acquire(device=%v) error = %v, want access_denied", device, err)
		}
	}
}

func acquire(t *testing.T, m *oauth.Manager, agentURL string) (*oauth.Requirement, *oauth.Grant) {
	t.Helper()
	req, err := m.RequirementForCard(t.Context(), fetchCard(t, agentURL), agentURL)
	if err != nil {
		t.Fatal(err)
	}
	g, err := m.Acquire(t.Context(), req, oauth.AcquireOptions{Prompter: oauth.Prompter{Open: oauthtest.Browser(t)}})
	if err != nil {
		t.Fatalf("Manager.Acquire() error = %v", err)
	}
	return req, g
}

func TestGrantRefreshesNearExpiry(t *testing.T) {
	t.Parallel()
	for _, rotate := range []bool{false, true} {
		as := oauthtest.NewAuthServer(t, oauthtest.ServerOptions{RotateRefreshTokens: rotate, TokenLifetime: 30 * time.Second})
		agent := oauthtest.NewProtectedAgent(t, as, oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL})
		m := newManager(t, as)
		req, first := acquire(t, m, agent.URL)
		for i := range 2 {
			g, err := m.Grant(t.Context(), req)
			if err != nil {
				t.Fatalf("Manager.Grant() #%d error = %v", i, err)
			}
			if g.AccessToken == first.AccessToken {
				t.Fatalf("Manager.Grant() #%d did not refresh a token that expires in 30s", i)
			}
			if !as.Valid(g.AccessToken, agent.URL, nil) {
				t.Fatalf("Manager.Grant() #%d returned an invalid token (rotate=%v)", i, rotate)
			}
		}
		if rotate && as.Hits("/token") != 3 {
			t.Fatalf("token endpoint hits = %d, want 3 (one exchange, two refreshes with rotated tokens)", as.Hits("/token"))
		}
	}
}

func TestGrantDropsRevokedRefreshToken(t *testing.T) {
	t.Parallel()
	as := oauthtest.NewAuthServer(t, oauthtest.ServerOptions{TokenLifetime: 30 * time.Second})
	agent := oauthtest.NewProtectedAgent(t, as, oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL})
	m := newManager(t, as)
	req, _ := acquire(t, m, agent.URL)
	as.RevokeRefreshTokens()
	g, err := m.Grant(t.Context(), req)
	if err != nil || g != nil {
		t.Fatalf("Manager.Grant() with a revoked refresh token = %v, %v, want nil, nil", g, err)
	}
	if grants, _ := m.List(); len(grants) != 0 {
		t.Fatalf("Manager.List() = %d grants, want the rejected grant deleted", len(grants))
	}
}

func TestRevoke(t *testing.T) {
	t.Parallel()
	as := oauthtest.NewAuthServer(t, oauthtest.ServerOptions{})
	agent := oauthtest.NewProtectedAgent(t, as, oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL})
	m := newManager(t, as)
	_, g := acquire(t, m, agent.URL)
	if err := m.Revoke(t.Context(), g); err != nil {
		t.Fatalf("Manager.Revoke() error = %v", err)
	}
	if diff := cmp.Diff([]string{g.RefreshToken}, as.Revoked()); diff != "" {
		t.Fatalf("revoked tokens wrong result (-want +got) diff = %s", diff)
	}
	if as.Valid(g.AccessToken, agent.URL, nil) {
		t.Fatalf("access token still valid after revoking its refresh token")
	}

	noRevoke := oauthtest.NewAuthServer(t, oauthtest.ServerOptions{NoRevocation: true})
	agent2 := oauthtest.NewProtectedAgent(t, noRevoke, oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL})
	m2 := newManager(t, noRevoke)
	_, g2 := acquire(t, m2, agent2.URL)
	if err := m2.Revoke(t.Context(), g2); !errors.Is(err, oauth.ErrRevocationUnsupported) {
		t.Fatalf("Manager.Revoke() without a revocation endpoint error = %v, want ErrRevocationUnsupported", err)
	}
}

func TestTransport(t *testing.T) {
	t.Parallel()
	as := oauthtest.NewAuthServer(t, oauthtest.ServerOptions{})
	agent := oauthtest.NewProtectedAgent(t, as, oauthtest.AgentOptions{})
	m := newManager(t, as)
	client := &http.Client{Transport: m.Transport(nil)}
	body := `{"message":{"messageId":"1","role":"ROLE_USER","parts":[{"text":"hi"}]}}`

	_, err := client.Post(agent.URL+"/message:send", "application/json", strings.NewReader(body))
	var ir *oauth.InteractionRequiredError
	if !errors.As(err, &ir) {
		t.Fatalf("POST without a grant error = %v, want InteractionRequiredError", err)
	}
	if _, err := m.Acquire(t.Context(), ir.Requirement, oauth.AcquireOptions{Prompter: oauth.Prompter{Open: oauthtest.Browser(t)}}); err != nil {
		t.Fatal(err)
	}

	post := func() int {
		t.Helper()
		resp, err := client.Post(agent.URL+"/message:send", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatalf("POST error = %v", err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	if got := post(); got != http.StatusOK {
		t.Fatalf("POST with a stored grant status = %d, want 200", got)
	}
	as.Invalidate()
	if got := post(); got != http.StatusOK {
		t.Fatalf("POST after the token was invalidated status = %d, want 200 after a refresh", got)
	}
	if as.SawAuthorizationHeader() {
		t.Fatalf("the authorization server received an Authorization header")
	}

	explicit, err := http.NewRequest(http.MethodPost, agent.URL+"/message:send", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	explicit.Header.Set("Authorization", "Bearer user-supplied")
	resp, err := client.Do(explicit)
	if err != nil {
		t.Fatalf("POST with an explicit credential error = %v, want the 401 passed through", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("POST with an explicit credential status = %d, want 401", resp.StatusCode)
	}
}
