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

func newAuthGetCmd(cfg *globalConfig) *cobra.Command {
	var reveal bool
	cmd := &cobra.Command{
		Use:   "get <agent-url>",
		Short: "Print the headers the credential helper returns for an agent",
		Long: `Print the headers the credential helper returns for an agent.

A credential helper follows the Bazel credential helper protocol:
https://github.com/EngFlow/credential-helper-spec. --credential-helper takes
"[scope=]path", where scope is a host such as example.com, or *.example.com
for a domain and its subdomains. Config files list {scope, command} entries
under credential-helper. The most specific helper for the agent's host runs.`,
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
			if cfg.credentials == nil {
				return fmt.Errorf("no credential helper configured")
			}
			h, ok := credhelper.Select(cfg.credentials.Helpers, target)
			if !ok {
				return fmt.Errorf("no credential helper for %s", target)
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), cfg.timeout)
			defer cancel()
			resp, err := h.Get(ctx, target)
			if err != nil {
				return err
			}
			headers := resp.Headers
			if !reveal {
				headers = redactHeaders(headers)
			}
			if cfg.IsJSON() {
				return cfg.PrintJSON(map[string]any{"url": target, "helper": h.Spec, "headers": headers, "expires": resp.Expires})
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
			if _, err := fmt.Fprintf(cfg.Out, "helper: %s\n", h.Spec); err != nil {
				return err
			}
			if resp.Expires != "" {
				if _, err := fmt.Fprintf(cfg.Out, "expires: %s\n", resp.Expires); err != nil {
					return err
				}
			}
			if len(rows) == 0 {
				_, err := fmt.Fprintln(cfg.Out, "no headers")
				return err
			}
			return cfg.PrintTable([]string{"HEADER", "VALUE"}, rows)
		},
	}
	cmd.Flags().BoolVar(&reveal, "reveal", false, "Print header values instead of redacting them")
	return cmd
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
