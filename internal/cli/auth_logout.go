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
	"github.com/a2aproject/a2a-cli/internal/oauth"
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
		Example: `  a2a auth logout https://agent.example.com
  a2a auth logout --all`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.oauth == nil {
				return clierr.Usage("OAuth is disabled (--oauth off)")
			}
			if all == (len(args) == 1) {
				return clierr.Usage("pass either an agent URL or --all")
			}
			grants, err := cfg.oauth.List()
			if err != nil {
				return err
			}
			if !all {
				var ref flagparse.URLOrPath
				if err := ref.Set(args[0]); err != nil {
					return err
				}
				if grants, err = cfg.grantsForAgent(ref.URL()); err != nil {
					return err
				}
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
				v := logoutView{Resource: g.Key.Resource, Issuer: g.Key.Issuer}
				switch err := cfg.oauth.Revoke(ctx, &g); {
				case err == nil:
					v.Revoked = true
				case errors.Is(err, oauth.ErrRevocationUnsupported):
					v.Error = err.Error()
				default:
					v.Error = err.Error()
					cfg.logf("revoking the grant for %s failed: %v", g.Key.Resource, err)
				}
				if err := cfg.oauth.Delete(ctx, g.Key); err != nil {
					return err
				}
				views = append(views, v)
			}

			if cfg.IsJSON() {
				return cfg.PrintJSON(map[string]any{"removed": views})
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
