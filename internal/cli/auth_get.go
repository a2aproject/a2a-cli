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
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-cli/internal/credhelper"
	"github.com/a2aproject/a2a-cli/internal/flagparse"
	"github.com/a2aproject/a2a-go/v2/a2aclient/oauth"
)

type helperResult struct {
	spec    string
	headers map[string][]string
	expires string
}

func newAuthGetCmd(cfg *globalConfig) *cobra.Command {
	var reveal bool
	cmd := &cobra.Command{
		Use:   "get <agent-url>",
		Short: "Print the credentials sent to an agent: credential helper headers and stored OAuth grants",
		Long: `Print the credentials sent to an agent: the headers the credential helper
returns for it, and the OAuth grants stored for it.

A credential helper follows the Bazel credential helper protocol:
https://github.com/EngFlow/credential-helper-spec. --credential-helper takes
"[scope=]path", where scope is a host such as example.com, or *.example.com
for a domain and its subdomains. Config files list {scope, command} entries
under credential-helper. The most specific helper for the agent's host runs.

OAuth grants are obtained with a2a auth login or when an agent requires
them; see a2a auth login --help. The agent's grants are the one for exactly
the URL you pass, those for the interface URLs in its card, and, when the URL
has a path, those below it; when the card can't be read, only the one for
exactly the URL. Other grants that may apply, such as one for the whole host
that other agents may share, are listed apart. --reveal prints header values
and access tokens.`,
		Example: `  a2a auth get https://agent.example.com
  a2a auth get https://agent.example.com --reveal`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var ref flagparse.URLOrPath
			if err := ref.Set(args[0]); err != nil {
				return err
			}
			target := ref.URL()
			if !isHTTPURL(target) {
				return clierr.Usage(fmt.Sprintf("auth get needs an http(s) agent URL, got %q", args[0]))
			}
			helper, err := cfg.helperHeaders(cmd.Context(), target)
			if err != nil {
				return err
			}
			grants, err := cfg.grantsForAgent(cmd.Context(), target)
			if err != nil && cfg.oauthMode == oauthOff && helper != nil {
				// The grant store is optional here; the helper headers still print.
				cfg.logf("not listing OAuth grants: %v", err)
				grants, err = &agentGrants{}, nil
			}
			if err != nil {
				return err
			}
			if helper == nil && len(grants.selected) == 0 && len(grants.alsoApply) == 0 {
				return clierr.CredentialsMissing(
					fmt.Sprintf("no credential helper or OAuth grant for %s", target),
					fmt.Sprintf("sign in with `a2a auth login %s`, or configure --credential-helper", target),
					nil)
			}
			views, err := cfg.grantViews(cmd.Context(), grants.selected, reveal)
			if err != nil {
				return err
			}
			also, err := cfg.grantViews(cmd.Context(), grants.alsoApply, reveal)
			if err != nil {
				return err
			}

			if cfg.IsJSON() {
				// Every key is always present, so scripts need no special cases.
				out := map[string]any{"url": target, "helper": "", "headers": map[string][]string{}, "expires": "", "oauth": views, "alsoApplies": also}
				if helper != nil {
					headers := helper.headers
					if !reveal {
						headers = redactHeaders(headers)
					}
					out["helper"], out["headers"], out["expires"] = helper.spec, headers, helper.expires
				}
				return cfg.PrintJSON(out)
			}
			if helper != nil {
				if err := printHelperHeaders(cfg, helper, reveal); err != nil {
					return err
				}
			}
			if len(views) > 0 {
				if _, err := fmt.Fprintln(cfg.Out, "oauth grants:"); err != nil {
					return err
				}
				if err := printGrantTable(cfg.Printer, views); err != nil {
					return err
				}
			}
			if len(also) > 0 {
				if _, err := fmt.Fprintln(cfg.Out, "oauth grants that may also apply (other agents may share them):"); err != nil {
					return err
				}
				if err := printGrantTable(cfg.Printer, also); err != nil {
					return err
				}
			}
			views = append(views, also...)
			if reveal {
				for _, v := range views {
					if _, err := fmt.Fprintf(cfg.Out, "access token for %s: %s\n", v.Resource, v.AccessToken); err != nil {
						return err
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&reveal, "reveal", false, "Print header values and access tokens instead of redacting them")
	return cmd
}

// grantViews returns the views of grants, with their access tokens when reveal
// is set. A store may list grants without their tokens, so they are read with
// Get.
func (g *globalConfig) grantViews(ctx context.Context, grants []oauth.Grant, reveal bool) ([]grantView, error) {
	views := make([]grantView, 0, len(grants))
	for _, gr := range grants {
		if reveal {
			full, err := g.oauth.Store.Get(ctx, gr.Key)
			if err != nil {
				return nil, fmt.Errorf("reading the grant for %s: %w", gr.Key.Resource, err)
			}
			gr = *full
		}
		views = append(views, newGrantView(gr, reveal))
	}
	return views, nil
}

// helperHeaders runs the credential helper for target, or returns nil when none
// is configured for its host.
func (g *globalConfig) helperHeaders(ctx context.Context, target string) (*helperResult, error) {
	if g.credentials == nil {
		return nil, nil
	}
	h, ok := credhelper.Select(g.credentials.Helpers, target)
	if !ok {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()
	resp, err := h.Get(ctx, target)
	if err != nil {
		return nil, err
	}
	return &helperResult{spec: h.Spec, headers: resp.Headers, expires: resp.Expires}, nil
}

func printHelperHeaders(cfg *globalConfig, helper *helperResult, reveal bool) error {
	headers := helper.headers
	if !reveal {
		headers = redactHeaders(headers)
	}
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	slices.Sort(names)
	rows := make([][]string, 0, len(names))
	for _, name := range names {
		rows = append(rows, []string{name, strings.Join(headers[name], ", ")})
	}
	if _, err := fmt.Fprintf(cfg.Out, "helper: %s\n", helper.spec); err != nil {
		return err
	}
	if helper.expires != "" {
		if _, err := fmt.Fprintf(cfg.Out, "expires: %s\n", helper.expires); err != nil {
			return err
		}
	}
	if len(rows) == 0 {
		_, err := fmt.Fprintln(cfg.Out, "no headers")
		return err
	}
	return cfg.PrintTable([]string{"HEADER", "VALUE"}, rows)
}

func redactHeaders(headers map[string][]string) map[string][]string {
	out := make(map[string][]string, len(headers))
	for name, values := range headers {
		redacted := make([]string, len(values))
		for i := range values {
			redacted[i] = "<redacted>"
		}
		out[name] = redacted
	}
	return out
}
