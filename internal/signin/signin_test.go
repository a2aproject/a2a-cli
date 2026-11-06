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

package signin

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
	"github.com/a2aproject/a2a-go/v2/a2aclient/oauth"
	"github.com/a2aproject/a2a-go/v2/a2aclient/oauth/oauthtest"
)

func newManager(as *oauthtest.AuthServer) *oauth.Manager {
	return &oauth.Manager{
		Clients: []oauth.Registration{{Authority: as.URL, ClientID: "test-client"}},
		Store:   oauth.NewMemoryStore(),
	}
}

func cardRequirement(t *testing.T, m *oauth.Manager, agentURL string) *oauth.Requirement {
	t.Helper()
	card, err := agentcard.DefaultResolver.Resolve(t.Context(), agentURL)
	if err != nil {
		t.Fatalf("Resolve(%q) error = %v", agentURL, err)
	}
	req, err := m.RequirementForCard(t.Context(), card, agentURL)
	if err != nil || req == nil {
		t.Fatalf("Manager.RequirementForCard() = %v, %v, want a requirement", req, err)
	}
	return req
}

func TestAcquire(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		server oauthtest.ServerOptions
		card   oauthtest.CardSecurity
		device bool
		path   string
		prompt string
	}{
		{name: "authorization code", card: oauthtest.CardMetadataURL, path: "/token", prompt: "To authorize, open:\n"},
		{name: "device flow requested", card: oauthtest.CardMetadataURL, device: true, path: "/device", prompt: "Enter code: "},
		{name: "device flow without authorization endpoint", server: oauthtest.ServerOptions{NoAuthorizationEndpoint: true, PendingPolls: 2}, card: oauthtest.CardMetadataURL, path: "/device", prompt: "Enter code: "},
		{name: "device flow is all the card offers", card: oauthtest.CardDeviceCode, path: "/device", prompt: "Enter code: "},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			as := oauthtest.NewAuthServer(t, tc.server)
			agent := oauthtest.NewProtectedAgent(t, as, oauthtest.AgentOptions{Card: tc.card, Scopes: []string{"a2a.invoke"}})
			m := newManager(as)
			req := cardRequirement(t, m, agent.URL)
			var prompt bytes.Buffer
			g, err := Acquire(t.Context(), m, req, Options{
				Device:       tc.device,
				Prompter:     Prompter{Out: &prompt, Open: oauthtest.Browser(t)},
				PollInterval: time.Millisecond,
			})
			if err != nil {
				t.Fatalf("Acquire() error = %v", err)
			}
			if !as.Valid(g.AccessToken, agent.URL, []string{"a2a.invoke"}) {
				t.Fatalf("Acquire() returned a token the agent does not accept")
			}
			if !strings.Contains(prompt.String(), tc.prompt) {
				t.Fatalf("Acquire() prompt = %q, want it to contain %q", prompt.String(), tc.prompt)
			}
			if got := as.LastForm(tc.path).Get("resource"); got != agent.URL {
				t.Fatalf("resource sent to %s = %q, want %q", tc.path, got, agent.URL)
			}
			if tc.path == "/token" {
				redirect, _ := url.Parse(as.LastAuthorizeQuery().Get("redirect_uri"))
				if redirect.Hostname() != "127.0.0.1" || redirect.Path != CallbackPath {
					t.Fatalf("redirect_uri = %s, want http://127.0.0.1:<port>%s", redirect, CallbackPath)
				}
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
		m := newManager(as)
		_, err := Acquire(t.Context(), m, cardRequirement(t, m, agent.URL), Options{Device: device, Prompter: Prompter{Open: oauthtest.Browser(t)}, PollInterval: time.Millisecond})
		var oe *oauth.Error
		if !errors.As(err, &oe) || oe.Code != "access_denied" {
			t.Fatalf("Acquire(device=%v) error = %v, want access_denied", device, err)
		}
	}
}

func TestAcquireIgnoresStrayCallbacks(t *testing.T) {
	t.Parallel()
	as := oauthtest.NewAuthServer(t, oauthtest.ServerOptions{})
	agent := oauthtest.NewProtectedAgent(t, as, oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL})
	m := newManager(as)
	browser := oauthtest.Browser(t)
	open := func(authURL string) {
		u, _ := url.Parse(authURL)
		callback := u.Query().Get("redirect_uri")
		for _, q := range []string{"state=forged&code=stolen", "state=forged&error=access_denied", ""} {
			resp, err := http.Get(callback + "?" + q)
			if err != nil {
				t.Errorf("GET %s?%s error = %v", callback, q, err)
				continue
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("GET %s?%s status = %d, want 400", callback, q, resp.StatusCode)
			}
		}
		browser(authURL)
	}
	g, err := Acquire(t.Context(), m, cardRequirement(t, m, agent.URL), Options{Prompter: Prompter{Open: open}})
	if err != nil || !as.Valid(g.AccessToken, agent.URL, nil) {
		t.Fatalf("Acquire() after stray callbacks = %v, %v, want a valid grant", g, err)
	}
}

func TestAcquireCanceled(t *testing.T) {
	t.Parallel()
	as := oauthtest.NewAuthServer(t, oauthtest.ServerOptions{})
	m := newManager(as)
	req := &oauth.Requirement{
		Endpoints: oauth.Endpoints{Issuer: as.URL, Authorization: as.URL + "/authorize", Token: as.URL + "/token"},
		Resource:  "https://agent.example.com",
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Acquire(ctx, m, req, Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Acquire() with a canceled context error = %v, want context.Canceled", err)
	}
}
