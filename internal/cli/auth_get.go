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
returns for it, and the OAuth grants stored for its host.

A credential helper follows the Bazel credential helper protocol:
https://github.com/EngFlow/credential-helper-spec. --credential-helper takes
"[scope=]path", where scope is a host such as example.com, or *.example.com
for a domain and its subdomains. Config files list {scope, command} entries
under credential-helper. The most specific helper for the agent's host runs.

OAuth grants are obtained with a2a auth login or when an agent requires
them; see a2a auth login --help. --reveal prints header values and access tokens.`,
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
			grants, err := cfg.grantsForAgent(target)
			if err != nil {
				return err
			}
			if helper == nil && len(grants) == 0 {
				return fmt.Errorf("no credential helper or OAuth grant for %s", target)
			}
			views := make([]grantView, 0, len(grants))
			for _, g := range grants {
				views = append(views, newGrantView(g, reveal))
			}

			if cfg.IsJSON() {
				out := map[string]any{"url": target}
				if helper != nil {
					headers := helper.headers
					if !reveal {
						headers = redactHeaders(headers)
					}
					out["helper"], out["headers"], out["expires"] = helper.spec, headers, helper.expires
				}
				if len(views) > 0 {
					out["oauth"] = views
				}
				return cfg.PrintJSON(out)
			}
			if helper != nil {
				if err := printHelperHeaders(cfg, helper, reveal); err != nil {
					return err
				}
			}
			if len(views) == 0 {
				return nil
			}
			if _, err := fmt.Fprintln(cfg.Out, "oauth grants:"); err != nil {
				return err
			}
			if err := printGrantTable(cfg.Printer, views); err != nil {
				return err
			}
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
