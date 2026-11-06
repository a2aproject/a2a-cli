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
	"net/http"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-cli/internal/oauth"
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
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

Other commands sign in the same way when an agent requires it (--oauth auto,
the default) if a terminal is attached. --oauth cached only uses stored grants,
so scripts and sandboxed agents can run after an a2a auth login; --oauth off
disables OAuth. No OAuth runs when --auth, --svc-param or a credential helper
supplies the Authorization header.

--oauth-clients configures one client per authorization server: on the command
line or in A2ACLI_OAUTH_CLIENTS as JSON, {"authority":"<issuer>","client_id":"<id
or CIMD URL>","scopes":[...]}, and in config files as a list of such objects.
When none matches a server that supports Client ID Metadata Documents, a2a uses
` + oauth.DefaultClientID + `.`,
		Example: `  a2a auth login https://agent.example.com
  a2a auth login https://agent.example.com --device
  a2a auth login https://agent.example.com --scope a2a.invoke
  a2a auth login -e https://agent.example.com/a2a --transport jsonrpc`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.oauth == nil {
				return clierr.Usage("OAuth is disabled (--oauth off)")
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
			grant, err := cfg.signIn(cmd.Context(), d, req, oauth.AcquireOptions{Device: device, Scopes: scopes})
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

var errLoginProbe = errors.New("login probe")

// loginProbe stops the first call and records the interface it was made to.
type loginProbe struct {
	a2aclient.PassthroughInterceptor
	req *a2aclient.Request
}

func (p *loginProbe) Before(ctx context.Context, req *a2aclient.Request) (context.Context, any, error) {
	r := *req
	p.req = &r
	return ctx, nil, errLoginProbe
}

// loginRequirement finds out where to sign in for the configured agent: from
// its card's security schemes for the interface the client would use, or from
// the Bearer challenge and Protected Resource Metadata of that interface.
func (g *globalConfig) loginRequirement(ctx context.Context) (*oauth.Requirement, error) {
	ctx, cancel := context.WithTimeout(withServiceParams(ctx, g), g.timeout)
	defer cancel()

	probe := &loginProbe{}
	client, err := newAgentClient(ctx, g, a2aclient.WithCallInterceptors(probe))
	var ir *oauth.InteractionRequiredError
	if errors.As(err, &ir) {
		return ir.Requirement, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create a client: %w", err)
	}
	defer destroyClient(g, client)

	_, err = client.GetTask(ctx, &a2a.GetTaskRequest{ID: "a2a-auth-login", Tenant: g.tenant})
	if errors.As(err, &ir) {
		return ir.Requirement, nil
	}
	if !errors.Is(err, errLoginProbe) {
		return nil, fmt.Errorf("probing the agent: %w", err)
	}
	if probe.req.Card != nil {
		r, err := g.oauth.RequirementForCard(ctx, probe.req.Card, probe.req.BaseURL)
		if err != nil || r != nil {
			return r, err
		}
	}
	return g.challengeRequirement(ctx, probe.req.BaseURL)
}

// challengeRequirement asks an interface without credentials for its Bearer
// challenge and follows it, or the well-known locations, to the authorization server.
func (g *globalConfig) challengeRequirement(ctx context.Context, baseURL string) (*oauth.Requirement, error) {
	raw := baseURL
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	target, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid interface URL %q: %w", baseURL, err)
	}
	var chs []oauth.Challenge
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}
	if resp, err := g.oauth.HTTPClient.Do(req); err == nil {
		if resp.StatusCode == http.StatusUnauthorized {
			chs = oauth.ParseChallenges(resp.Header.Values("WWW-Authenticate"))
		}
		_ = resp.Body.Close()
	} else {
		g.logf("requesting %s without credentials failed: %v", target, err)
	}
	r, err := g.oauth.RequirementForChallenge(ctx, target, chs)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, clierr.Usage(fmt.Sprintf("%s does not advertise OAuth: its card declares no oauth2 or openIdConnect scheme and it publishes no protected resource metadata", baseURL))
	}
	return r, nil
}
