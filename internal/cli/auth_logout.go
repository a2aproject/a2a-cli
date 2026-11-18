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
	"io"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-cli/internal/flagparse"
	"github.com/a2aproject/a2a-go/v2/a2aclient/oauth"
)

type logoutView struct {
	Resource string `json:"resource"`
	Issuer   string `json:"issuer"`
	Revoked  bool   `json:"revoked"`
	Error    string `json:"revokeError,omitempty"`
}

func newAuthLogoutCmd(cfg *globalConfig) *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "logout [agent-url]",
		Short: "Revoke and delete stored OAuth grants for an agent, or all of them",
		Long: `Revoke and delete stored OAuth grants for an agent, or all of them.

The agent's grants are the one for exactly the URL you pass, those for the
interface URLs in its agent card, and, when the URL has a path, those below
it. When the card can't be read, only the grant for exactly the URL is
removed. Other grants that may apply, such as one for the whole host that
other agents may share, are kept; a note names each with the command that
removes it, and --all removes every grant.`,
		Example: `  a2a auth logout https://agent.example.com
  a2a auth logout --all`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if all == (len(args) == 1) {
				return clierr.Usage("pass either an agent URL or --all")
			}
			if err := cfg.requireOAuth(); err != nil {
				return err
			}
			var grants, kept []oauth.Grant
			cardFound := true
			if all {
				var err error
				if grants, err = cfg.oauth.Store.List(cmd.Context()); err != nil {
					return err
				}
			} else {
				var ref flagparse.URLOrPath
				if err := ref.Set(args[0]); err != nil {
					return err
				}
				if !isHTTPURL(ref.URL()) {
					return clierr.Usage(fmt.Sprintf("auth logout needs an http(s) agent URL, got %q", args[0]))
				}
				found, err := cfg.grantsForAgent(cmd.Context(), ref.URL())
				if err != nil {
					return err
				}
				grants, kept, cardFound = found.selected, found.alsoApply, found.cardFound
			}

			if len(grants) > 0 {
				if err := cfg.initTLS(); err != nil {
					return err
				}
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), cfg.timeout)
			defer cancel()
			views := make([]logoutView, 0, len(grants))
			for _, g := range grants {
				v := logoutView{Resource: g.Resource, Issuer: g.Issuer}
				// Revoke removes the grant even when the server doesn't revoke
				// it; a FailureError means the store or its lock failed.
				var fe *oauth.FailureError
				switch err := cfg.oauth.Revoke(ctx, g); {
				case err == nil:
					v.Revoked = true
				case errors.As(err, &fe):
					return err
				case errors.Is(err, oauth.ErrNoRevocationEndpoint):
					v.Error = err.Error()
				default:
					v.Error = err.Error()
					cfg.logf("revoking the grant for %s failed: %v", g.Resource, err)
				}
				views = append(views, v)
			}

			if err := printKeptGrants(cfg, args, kept, cardFound); err != nil {
				return err
			}
			if cfg.IsJSON() {
				out := map[string]any{"removed": views}
				if len(kept) > 0 {
					keptViews := make([]grantView, 0, len(kept))
					for _, g := range kept {
						keptViews = append(keptViews, newGrantView(g, false))
					}
					out["kept"] = keptViews
				}
				return cfg.PrintJSON(out)
			}
			if len(views) == 0 {
				_, err := io.WriteString(cfg.Out, "No stored OAuth grants to remove.\n")
				return err
			}
			for _, v := range views {
				status := "revoked"
				if !v.Revoked {
					status = "deleted locally; not revoked: " + v.Error
				}
				if _, err := fmt.Fprintf(cfg.Out, "Logged out of %s (%s)\n", v.Resource, status); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "Remove every stored grant")
	return cmd
}

// printKeptGrants explains on stderr which grants that may apply to the agent
// were kept, and how to remove them.
func printKeptGrants(cfg *globalConfig, args []string, kept []oauth.Grant, cardFound bool) error {
	if len(kept) == 0 {
		return nil
	}
	w := cfg.stderr()
	if !cardFound {
		if _, err := fmt.Fprintf(w, "note: the agent card of %s could not be read, so only a grant for exactly that URL was removed\n", args[0]); err != nil {
			return err
		}
	}
	for _, g := range kept {
		if _, err := fmt.Fprintf(w, "note: kept the grant for %s, which may also apply to this agent and to others; remove it with `a2a auth logout %s`\n", g.Resource, g.Resource); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w, "note: `a2a auth logout --all` removes every grant")
	return err
}
