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
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/a2aproject/a2a-cli/internal/oauth"
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
)

var inlineEndpoints = oauth.Endpoints{
	Issuer:        "https://as.example.com",
	Authorization: "https://as.example.com/authorize",
	Token:         "https://as.example.com/token",
}

// inlineCard declares the inline oauth2 scheme, and an API key alternative when
// withAPIKey is set.
func inlineCard(withAPIKey bool) *a2a.AgentCard {
	card := &a2a.AgentCard{
		Name: "inline",
		SecuritySchemes: a2a.NamedSecuritySchemes{
			"oauth": a2a.OAuth2SecurityScheme{Flows: a2a.AuthorizationCodeOAuthFlow{AuthorizationURL: inlineEndpoints.Authorization, TokenURL: inlineEndpoints.Token}},
			"key":   a2a.APIKeySecurityScheme{Name: "X-API-Key", Location: "header"},
		},
		SecurityRequirements: a2a.SecurityRequirementsOptions{{"oauth": nil}},
	}
	if withAPIKey {
		card.SecurityRequirements = append(card.SecurityRequirements, a2a.SecurityRequirements{"key": nil})
	}
	return card
}

// managerWithGrant returns a manager whose store holds one grant for resource
// and scopes, with the given tokens.
func managerWithGrant(t *testing.T, resource string, scopes []string, access, refresh string) *oauth.Manager {
	t.Helper()
	store := &oauth.FileStore{Dir: t.TempDir()}
	if resource != "" {
		g := oauth.Grant{
			Key:          oauth.NewGrantKey(inlineEndpoints.Issuer, "c", resource, scopes),
			AccessToken:  access,
			RefreshToken: refresh,
			Endpoints:    inlineEndpoints,
		}
		if access != "" {
			g.ExpiresAt = time.Now().Add(time.Hour)
		}
		if err := store.Put(g); err != nil {
			t.Fatal(err)
		}
	}
	return &oauth.Manager{Clients: []oauth.Registration{{Authority: inlineEndpoints.Issuer, ClientID: "c"}}, Store: store}
}

// before runs the interceptor and returns the Authorization values it attached.
func before(t *testing.T, m *oauth.Manager, baseURL string, card *a2a.AgentCard) ([]string, error) {
	t.Helper()
	req := &a2aclient.Request{BaseURL: baseURL, Card: card, ServiceParams: a2aclient.ServiceParams{}}
	_, _, err := m.Interceptor().Before(context.Background(), req)
	return req.ServiceParams.Get("authorization"), err
}

func TestInterceptorOptionalOAuth(t *testing.T) {
	t.Parallel()
	const agent = "https://agent.example.com"

	// A card that also accepts an API key goes out without a grant.
	got, err := before(t, managerWithGrant(t, "", nil, "", ""), agent, inlineCard(true))
	if err != nil || len(got) != 0 {
		t.Fatalf("Before() without a grant = %q, %v, want no token and no error", got, err)
	}
	// It still gets a stored grant.
	got, err = before(t, managerWithGrant(t, agent, nil, "at", ""), agent, inlineCard(true))
	if err != nil || len(got) != 1 || got[0] != "Bearer at" {
		t.Fatalf("Before() with a grant = %q, %v, want the stored token", got, err)
	}
	// A card that only accepts OAuth requires sign-in.
	_, err = before(t, managerWithGrant(t, "", nil, "", ""), agent, inlineCard(false))
	var ir *oauth.InteractionRequiredError
	if !errors.As(err, &ir) {
		t.Fatalf("Before() for an OAuth-only card error = %v, want InteractionRequiredError", err)
	}
	// Without a matching client, only the OAuth-only card fails.
	m := managerWithGrant(t, "", nil, "", "")
	m.Clients = nil
	if got, err := before(t, m, agent, inlineCard(true)); err != nil || len(got) != 0 {
		t.Fatalf("Before() for an optional card without a client = %q, %v, want no token and no error", got, err)
	}
	var nc *oauth.NoClientError
	if _, err := before(t, m, agent, inlineCard(false)); !errors.As(err, &nc) {
		t.Fatalf("Before() for an OAuth-only card without a client error = %v, want NoClientError", err)
	}
}

func TestInterceptorUnreadableStore(t *testing.T) {
	t.Parallel()
	m := managerWithGrant(t, "", nil, "", "")
	dir := m.Store.(*oauth.FileStore).Dir
	if err := os.WriteFile(filepath.Join(dir, "grants.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Agents that don't require OAuth are not affected.
	if got, err := before(t, m, "https://agent.example.com", nil); err != nil || len(got) != 0 {
		t.Fatalf("Before() without a requirement = %q, %v, want no token and no error", got, err)
	}
	if _, err := before(t, m, "https://agent.example.com", inlineCard(false)); err == nil {
		t.Fatal("Before() for an OAuth-only card error = nil, want the store error")
	}
}

func TestInterceptorRefusesCleartext(t *testing.T) {
	t.Parallel()
	const agent = "http://agent.example.com"
	m := managerWithGrant(t, agent, nil, "secret", "")
	got, err := before(t, m, agent, inlineCard(false))
	var fe *oauth.FailureError
	if !errors.As(err, &fe) || len(got) != 0 {
		t.Fatalf("Before() for an http agent = %q, %v, want FailureError and no token", got, err)
	}
	if got, err := before(t, m, agent, nil); err != nil || len(got) != 0 {
		t.Fatalf("Before() for an http agent without a requirement = %q, %v, want no token and no error", got, err)
	}
	if got, err := before(t, managerWithGrant(t, "http://127.0.0.1:8080", nil, "at", ""), "http://127.0.0.1:8080", nil); err != nil || len(got) != 1 {
		t.Fatalf("Before() for a loopback agent = %q, %v, want the token", got, err)
	}
}

func TestInterceptorGrantWithoutTokens(t *testing.T) {
	t.Parallel()
	const agent = "https://agent.example.com"
	// A grant whose tokens are gone, such as a keychain entry that was removed,
	// needs a new sign-in instead of an empty Bearer token.
	got, err := before(t, managerWithGrant(t, agent, nil, "", ""), agent, inlineCard(false))
	var ir *oauth.InteractionRequiredError
	if !errors.As(err, &ir) || len(got) != 0 {
		t.Fatalf("Before() = %q, %v, want InteractionRequiredError and no token", got, err)
	}
	if got, err := before(t, managerWithGrant(t, agent, nil, "", ""), agent, nil); err != nil || len(got) != 0 {
		t.Fatalf("Before() without a requirement = %q, %v, want no token and no error", got, err)
	}
}

func TestInterceptorCoveringGrant(t *testing.T) {
	t.Parallel()
	const iface = "https://agent.example.com/a2a/v1"
	// A grant for the agent's origin, with the scopes auth login --scope asked
	// for, serves calls to an interface below it.
	m := managerWithGrant(t, "https://agent.example.com", []string{"a2a.invoke"}, "at", "")
	for _, card := range []*a2a.AgentCard{inlineCard(false), nil} {
		if got, err := before(t, m, iface, card); err != nil || len(got) != 1 || got[0] != "Bearer at" {
			t.Fatalf("Before(card=%v) = %q, %v, want the origin grant", card != nil, got, err)
		}
	}
	// A grant lacking a required scope is not used.
	card := inlineCard(false)
	card.SecurityRequirements = a2a.SecurityRequirementsOptions{{"oauth": {"a2a.admin"}}}
	var ir *oauth.InteractionRequiredError
	if _, err := before(t, m, iface, card); !errors.As(err, &ir) {
		t.Fatalf("Before() needing another scope error = %v, want InteractionRequiredError", err)
	}
	// Another agent on the same host is not covered.
	if got, err := before(t, managerWithGrant(t, "https://agent.example.com/a", nil, "at", ""), "https://agent.example.com/b", nil); err != nil || len(got) != 0 {
		t.Fatalf("Before() for a sibling path = %q, %v, want no token", got, err)
	}
}
