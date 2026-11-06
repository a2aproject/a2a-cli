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
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-cli/internal/oauthstore"
	"github.com/a2aproject/a2a-cli/internal/signin"
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2aclient/oauth"
)

func newAuthLoginCmd(cfg *globalConfig, d deps) *cobra.Command {
	var device bool
	var scopes []string
	cmd := &cobra.Command{
		Use:   "login [agent-ref]",
		Short: "Sign in to an agent and store the OAuth grant, without sending a message",
		Long: `Sign in to an agent and store the OAuth grant, without sending a message.

a2a learns where to sign in from the agent card's oauth2 or openIdConnect
security scheme, or from the Bearer challenge of a 401 response and the
agent's protected resource metadata. It prints a URL to open and waits for the
redirect to 127.0.0.1; with --device it prints a URL and a code to enter on any
device. The grant is stored in ~/.config/a2a-cli/oauth/grants.json (or the OS
keychain with --oauth-store keychain) and refreshed when it expires.

Other commands leave OAuth off unless you turn it on (--oauth off, the
default), so agents that take other credentials work as before. With --oauth
auto, or oauth: auto in ~/.config/a2a-cli/config.yaml, they use stored grants
and sign in the same way when an agent requires it and a terminal is attached.
--oauth cached only uses stored grants, so scripts and sandboxed agents can run
after an a2a auth login. No OAuth runs when --auth, --svc-param or a credential
helper supplies the Authorization header. Other credentials you pass (any
--svc-param, a credential helper for the agent's host, --unix-socket) are sent
first: OAuth then signs in only when the agent answers 401. a2a server --proxy
never uses OAuth for the upstream agent.

--oauth-clients configures one client per authorization server: on the command
line or in A2ACLI_OAUTH_CLIENTS as JSON, {"authority":"<issuer>","client_id":"<id
or CIMD URL>","scopes":[...]}, and in config files as a list of such objects.
"omit_resource":true leaves the RFC 8707 resource parameter out, for servers
that reject it.
When none matches a server that supports Client ID Metadata Documents, a2a uses
` + oauthstore.DefaultClientID + `.`,
		Example: `  a2a auth login https://agent.example.com
  a2a auth login https://agent.example.com --device
  a2a auth login https://agent.example.com --scope a2a.invoke
  a2a auth login -e https://agent.example.com/a2a --transport jsonrpc`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cfg.requireOAuth(); err != nil {
				return err
			}
			if cfg.oauthMode == oauthOff {
				// Signing in is the point of this command, so it uses OAuth whatever the default.
				cfg.oauthMode = oauthAuto
			}
			if len(args) == 1 {
				if cfg.url != "" {
					return clierr.Usage("pass the agent either as an argument or with --endpoint, not both")
				}
				if err := cfg.agentCard.Set(args[0]); err != nil {
					return err
				}
			}
			req, err := cfg.loginRequirement(cmd.Context())
			if err != nil {
				return cfg.oauthError(err)
			}
			grant, err := cfg.signIn(cmd.Context(), d, req, signin.Options{Device: device, Scopes: scopes})
			if err != nil {
				return err
			}
			if cfg.IsJSON() {
				return cfg.PrintJSON(newGrantView(*grant, false))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&device, "device", false, "Use the device authorization flow: print a URL and a code to enter on any device")
	cmd.Flags().StringArrayVar(&scopes, "scope", nil, "Scope to request instead of the ones the agent names (repeatable)")
	return cmd
}

// loginProbe records the call the client makes, and so the interface it chose.
type loginProbe struct {
	a2aclient.PassthroughInterceptor
	req *a2aclient.Request
}

func (p *loginProbe) Before(ctx context.Context, req *a2aclient.Request) (context.Context, any, error) {
	r := *req
	p.req = &r
	return ctx, nil, nil
}

// loginRequirement finds out where to sign in for the configured agent. It
// makes a real A2A call, a GetTask, the way other commands reach the agent:
// a protected agent card, a card that requires OAuth or the agent's Bearer 401
// yield the requirement as they would for them. When the call needs no sign-in,
// for example because a grant is already stored, it reads the requirement from
// the card for the interface the client chose, or else asks that interface
// without credentials with [oauth.Manager.RequirementForURL]. It probes with no
// stored grants, so a grant stored before can't make the call succeed and hide
// the agent's 401 when signing in again.
func (g *globalConfig) loginRequirement(ctx context.Context) (*oauth.Requirement, error) {
	ctx, cancel := context.WithTimeout(withServiceParams(ctx, g), g.timeout)
	defer cancel()

	stored := g.oauth
	g.oauth = &oauth.Manager{
		Clients:         stored.Clients,
		DefaultClientID: stored.DefaultClientID,
		HTTPClient:      stored.HTTPClient,
		Store:           oauth.NewMemoryStore(),
	}
	defer func() { g.oauth = stored }()

	probe := &loginProbe{}
	client, err := newAgentClient(ctx, g, a2aclient.WithCallInterceptors(probe))
	ir, err := g.interactionRequired(ctx, err)
	if ir != nil {
		return ir.Requirement, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create a client: %w", err)
	}
	defer destroyClient(g, client)

	_, err = client.GetTask(ctx, &a2a.GetTaskRequest{ID: "a2a-auth-login", Tenant: g.tenant})
	var nc *oauth.NoClientError
	var ac *oauth.AmbiguousClientError
	var fe *oauth.FailureError
	switch {
	case errors.As(err, &ir):
		return ir.Requirement, nil
	case errors.As(err, &nc), errors.As(err, &ac), errors.As(err, &fe):
		return nil, err
	case probe.req == nil:
		return nil, fmt.Errorf("probing the agent: %w", err)
	}
	if err != nil {
		g.logf("the login probe got %v; looking for OAuth in the card and the interface", err)
	}
	if probe.req.Card != nil {
		r, err := g.oauth.RequirementForCard(ctx, probe.req.Card, probe.req.BaseURL)
		if err != nil || r != nil {
			return r, err
		}
	}
	raw := probe.req.BaseURL
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	target, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid interface URL %q: %w", probe.req.BaseURL, err)
	}
	r, err := g.oauth.RequirementForURL(ctx, target)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, clierr.Usage(fmt.Sprintf("%s does not advertise OAuth: its card declares no oauth2 or openIdConnect scheme and it does not answer 401 with a Bearer challenge and protected resource metadata", probe.req.BaseURL))
	}
	return r, nil
}
