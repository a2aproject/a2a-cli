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

package oauth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/a2aproject/a2a-cli/internal/oauth/oauthtest"
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
)

func testFetcher() fetcher { return fetcher{client: &http.Client{}} }

// jsonServer serves body as JSON at every path.
func jsonServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRequirementFromCard(t *testing.T) {
	t.Parallel()
	as := oauthtest.NewAuthServer(t, oauthtest.ServerOptions{})
	scopes := []string{"a2a.invoke"}
	all := Endpoints{
		Issuer:              as.URL,
		Authorization:       as.URL + "/authorize",
		Token:               as.URL + "/token",
		DeviceAuthorization: as.URL + "/device",
		Revocation:          as.URL + "/revoke",
	}
	tests := []struct {
		name string
		card oauthtest.CardSecurity
		want *Requirement
	}{
		{
			name: "oauth2 metadata URL",
			card: oauthtest.CardMetadataURL,
			want: &Requirement{Endpoints: all, Scopes: scopes},
		},
		{
			name: "inline authorization code flow",
			card: oauthtest.CardAuthCode,
			want: &Requirement{Endpoints: Endpoints{Issuer: as.URL, Authorization: as.URL + "/authorize", Token: as.URL + "/token"}, Scopes: scopes},
		},
		{
			name: "inline device code flow",
			card: oauthtest.CardDeviceCode,
			want: &Requirement{Endpoints: Endpoints{Issuer: as.URL, DeviceAuthorization: as.URL + "/device", Token: as.URL + "/token"}, Scopes: scopes, PreferDevice: true},
		},
		{
			name: "openIdConnect",
			card: oauthtest.CardOIDC,
			want: &Requirement{Endpoints: all, Scopes: scopes},
		},
		{
			name: "no security",
			card: oauthtest.CardNone,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			agent := oauthtest.NewProtectedAgent(t, as, oauthtest.AgentOptions{Card: tc.card, Scopes: scopes})
			card, err := agentcard.DefaultResolver.Resolve(t.Context(), agent.URL)
			if err != nil {
				t.Fatalf("Resolve(%q) error = %v", agent.URL, err)
			}
			if tc.want != nil {
				tc.want.Resource = NormalizeResource(agent.URL)
			}
			got, err := testFetcher().requirementFromCard(t.Context(), card, agent.URL)
			if err != nil {
				t.Fatalf("requirementFromCard() error = %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("requirementFromCard() wrong result (-want +got) diff = %s", diff)
			}
		})
	}
}

func TestRequirementFromCardSkipsUnusableOptions(t *testing.T) {
	t.Parallel()
	card := &a2a.AgentCard{
		SecuritySchemes: a2a.NamedSecuritySchemes{
			"key":   a2a.APIKeySecurityScheme{Name: "X-Key", Location: "header"},
			"oauth": a2a.OAuth2SecurityScheme{Flows: a2a.AuthorizationCodeOAuthFlow{AuthorizationURL: "https://as.example.com/authorize", TokenURL: "https://as.example.com/token"}},
		},
		SecurityRequirements: a2a.SecurityRequirementsOptions{
			{"key": nil},
			{"key": nil, "oauth": nil},
			{"missing": nil},
			{"oauth": {"read"}},
		},
	}
	got, err := testFetcher().requirementFromCard(t.Context(), card, "https://agent.example.com/")
	if err != nil {
		t.Fatalf("requirementFromCard() error = %v", err)
	}
	// The API key option makes the OAuth requirement optional.
	want := &Requirement{
		Endpoints: Endpoints{Issuer: "https://as.example.com", Authorization: "https://as.example.com/authorize", Token: "https://as.example.com/token"},
		Resource:  "https://agent.example.com",
		Scopes:    []string{"read"},
		Optional:  true,
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("requirementFromCard() wrong result (-want +got) diff = %s", diff)
	}

	// An empty option means the agent can be called without credentials.
	card.SecurityRequirements = append(a2a.SecurityRequirementsOptions{{}}, card.SecurityRequirements...)
	if got, err := testFetcher().requirementFromCard(t.Context(), card, "https://agent.example.com"); got != nil || err != nil {
		t.Fatalf("requirementFromCard() with an anonymous option = %+v, %v, want nil, nil", got, err)
	}
}

func TestRequirementFromCardRejectsBadMetadata(t *testing.T) {
	t.Parallel()
	srv := jsonServer(t, `{"issuer":"https://elsewhere.example.com","token_endpoint":"https://elsewhere.example.com/token"}`)
	card := &a2a.AgentCard{
		SecuritySchemes:      a2a.NamedSecuritySchemes{"oauth": a2a.OAuth2SecurityScheme{Oauth2MetadataURL: srv.URL + "/.well-known/oauth-authorization-server"}},
		SecurityRequirements: a2a.SecurityRequirementsOptions{{"oauth": nil}},
	}
	_, err := testFetcher().requirementFromCard(t.Context(), card, "https://agent.example.com")
	if err == nil || !strings.Contains(err.Error(), "does not publish metadata there") {
		t.Fatalf("requirementFromCard() error = %v, want the issuer mismatch", err)
	}
}

func TestDiscoverFromChallenge(t *testing.T) {
	t.Parallel()
	as := oauthtest.NewAuthServer(t, oauthtest.ServerOptions{})
	for _, omit := range []bool{false, true} {
		agent := oauthtest.NewProtectedAgent(t, as, oauthtest.AgentOptions{Scopes: []string{"a2a.invoke"}, OmitResourceMetadata: omit})
		resp, err := http.Get(agent.URL + "/tasks")
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("GET %s status = %d, want 401", agent.URL, resp.StatusCode)
		}
		target, _ := url.Parse(agent.URL + "/tasks")
		got, err := testFetcher().discoverFromChallenge(t.Context(), target, ParseChallenges(resp.Header.Values("WWW-Authenticate")))
		if err != nil {
			t.Fatalf("discoverFromChallenge(omit resource_metadata=%v) error = %v", omit, err)
		}
		want := &challengeTarget{
			resource: &ResourceMetadata{Resource: agent.URL, AuthorizationServers: []string{as.URL}, ScopesSupported: []string{"a2a.invoke"}},
			scopes:   []string{"a2a.invoke"},
		}
		if diff := cmp.Diff(want, got, cmp.AllowUnexported(challengeTarget{})); diff != "" {
			t.Fatalf("discoverFromChallenge(omit resource_metadata=%v) wrong result (-want +got) diff = %s", omit, diff)
		}
	}
}

func TestResourceMetadataErrors(t *testing.T) {
	t.Parallel()
	notFound := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(notFound.Close)
	tests := []struct {
		name     string
		body     string
		explicit bool
		wantErr  string
	}{
		{name: "other resource", body: `{"resource":"https://other.example.com","authorization_servers":["https://as.example.com"]}`, wantErr: "describes"},
		{name: "no resource", body: `{"authorization_servers":["https://as.example.com"]}`, wantErr: "has no resource"},
		{name: "no authorization servers", body: `{"resource":"%s"}`, wantErr: "lists no authorization servers"},
		{name: "explicit URL not found", explicit: true, wantErr: "HTTP 404"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var srv *httptest.Server
			srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.body == "" {
					http.NotFound(w, r)
					return
				}
				_, _ = w.Write([]byte(strings.ReplaceAll(tc.body, "%s", srv.URL)))
			}))
			t.Cleanup(srv.Close)
			target, _ := url.Parse(srv.URL)
			metadataURL := ""
			if tc.explicit {
				metadataURL = srv.URL + "/.well-known/oauth-protected-resource"
			}
			_, err := testFetcher().resourceMetadata(t.Context(), target, metadataURL)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("resourceMetadata() error = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}

	t.Run("none published", func(t *testing.T) {
		t.Parallel()
		target, _ := url.Parse(notFound.URL + "/a2a")
		if _, err := testFetcher().resourceMetadata(t.Context(), target, ""); !errors.Is(err, errNoResourceMetadata) {
			t.Fatalf("resourceMetadata() error = %v, want errNoResourceMetadata", err)
		}
	})

	t.Run("cross-origin redirect", func(t *testing.T) {
		t.Parallel()
		other := jsonServer(t, `{"resource":"https://agent.example.com","authorization_servers":["https://as.example.com"]}`)
		redirect := httptest.NewServer(http.RedirectHandler(other.URL+"/prm", http.StatusFound))
		t.Cleanup(redirect.Close)
		target, _ := url.Parse(redirect.URL)
		_, err := testFetcher().resourceMetadata(t.Context(), target, redirect.URL+"/prm")
		if err == nil || !strings.Contains(err.Error(), "another origin") {
			t.Fatalf("resourceMetadata() error = %v, want the cross-origin redirect refused", err)
		}
	})
}

func TestServerMetadata(t *testing.T) {
	t.Parallel()
	for _, oidcOnly := range []bool{false, true} {
		as := oauthtest.NewAuthServer(t, oauthtest.ServerOptions{OIDCOnly: oidcOnly, CIMD: true})
		m, err := testFetcher().serverMetadata(t.Context(), as.URL)
		if err != nil {
			t.Fatalf("serverMetadata(OIDCOnly=%v) error = %v", oidcOnly, err)
		}
		want := Endpoints{
			Issuer:              as.URL,
			Authorization:       as.URL + "/authorize",
			Token:               as.URL + "/token",
			DeviceAuthorization: as.URL + "/device",
			Revocation:          as.URL + "/revoke",
			CIMDSupported:       true,
		}
		if diff := cmp.Diff(want, m.endpoints()); diff != "" {
			t.Fatalf("serverMetadata(OIDCOnly=%v).endpoints() wrong result (-want +got) diff = %s", oidcOnly, diff)
		}
	}

	srv := jsonServer(t, `{"issuer":"https://elsewhere.example.com"}`)
	if _, err := testFetcher().serverMetadata(t.Context(), srv.URL); err == nil || !strings.Contains(err.Error(), "names issuer") {
		t.Fatalf("serverMetadata() error = %v, want the issuer mismatch", err)
	}
}

func TestEndpointsValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		e       Endpoints
		wantErr string
	}{
		{name: "authorization code", e: Endpoints{Issuer: "https://as.example.com", Authorization: "https://as.example.com/authorize", Token: "https://as.example.com/token"}},
		{name: "device only", e: Endpoints{Issuer: "https://as.example.com", DeviceAuthorization: "https://as.example.com/device", Token: "https://as.example.com/token"}},
		{name: "no issuer", e: Endpoints{Token: "https://as.example.com/token"}, wantErr: "no issuer"},
		{name: "no token endpoint", e: Endpoints{Issuer: "https://as.example.com", Authorization: "https://as.example.com/authorize"}, wantErr: "no token endpoint"},
		{name: "no user flow", e: Endpoints{Issuer: "https://as.example.com", Token: "https://as.example.com/token"}, wantErr: "neither"},
		{name: "plain http", e: Endpoints{Issuer: "http://as.example.com", Authorization: "http://as.example.com/authorize", Token: "http://as.example.com/token"}, wantErr: "https"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.e.validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validate() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("validate() error = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestAcceptsOtherCredentials(t *testing.T) {
	t.Parallel()
	oauthScheme := a2a.OAuth2SecurityScheme{Flows: a2a.AuthorizationCodeOAuthFlow{AuthorizationURL: "https://as.example.com/authorize", TokenURL: "https://as.example.com/token"}}
	schemes := a2a.NamedSecuritySchemes{
		"oauth": oauthScheme,
		"oidc":  a2a.OpenIDConnectSecurityScheme{OpenIDConnectURL: "https://as.example.com/.well-known/openid-configuration"},
		"key":   a2a.APIKeySecurityScheme{Name: "X-Key", Location: "header"},
	}
	tests := []struct {
		name    string
		options a2a.SecurityRequirementsOptions
		want    bool
	}{
		{name: "oauth only", options: a2a.SecurityRequirementsOptions{{"oauth": nil}}},
		{name: "oauth or openIdConnect", options: a2a.SecurityRequirementsOptions{{"oauth": nil}, {"oidc": nil}}},
		{name: "oauth or api key", options: a2a.SecurityRequirementsOptions{{"oauth": nil}, {"key": nil}}, want: true},
		{name: "oauth or no credentials", options: a2a.SecurityRequirementsOptions{{"oauth": nil}, {}}, want: true},
		{name: "oauth and api key together", options: a2a.SecurityRequirementsOptions{{"oauth": nil, "key": nil}}},
		{name: "undefined scheme", options: a2a.SecurityRequirementsOptions{{"oauth": nil}, {"missing": nil}}},
	}
	for _, tc := range tests {
		card := &a2a.AgentCard{SecuritySchemes: schemes, SecurityRequirements: tc.options}
		if got := acceptsOtherCredentials(card); got != tc.want {
			t.Errorf("%s: acceptsOtherCredentials() = %v, want %v", tc.name, got, tc.want)
		}
	}
}
