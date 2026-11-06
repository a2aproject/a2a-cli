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
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-cli/internal/oauth"
	"github.com/a2aproject/a2a-cli/internal/output"
)

// grantView is the JSON/text view of a stored OAuth grant.
type grantView struct {
	Resource     string   `json:"resource"`
	Issuer       string   `json:"issuer"`
	ClientID     string   `json:"clientId"`
	ClientSource string   `json:"clientSource,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
	ExpiresAt    string   `json:"expiresAt,omitempty"`
	Refreshable  bool     `json:"refreshable"`
	AccessToken  string   `json:"accessToken,omitempty"`
}

func newGrantView(g oauth.Grant, reveal bool) grantView {
	v := grantView{
		Resource:     g.Key.Resource,
		Issuer:       g.Key.Issuer,
		ClientID:     g.Key.ClientID,
		ClientSource: g.ClientSource,
		Scopes:       g.Key.Scopes,
		Refreshable:  g.RefreshToken != "",
	}
	if !g.ExpiresAt.IsZero() {
		v.ExpiresAt = g.ExpiresAt.UTC().Format(time.RFC3339)
	}
	if reveal {
		v.AccessToken = g.AccessToken
	}
	return v
}

func newAuthListCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List stored OAuth grants",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.oauth == nil {
				return clierr.Usage("OAuth is disabled (--oauth off)")
			}
			grants, err := cfg.oauth.List()
			if err != nil {
				return err
			}
			views := make([]grantView, 0, len(grants))
			for _, g := range grants {
				views = append(views, newGrantView(g, false))
			}
			if cfg.IsJSON() {
				return cfg.PrintJSON(views)
			}
			if len(views) == 0 {
				_, err := io.WriteString(cfg.Out, "No stored OAuth grants.\n")
				return err
			}
			return printGrantTable(cfg.Printer, views)
		},
	}
}

func printGrantTable(p *output.Printer, views []grantView) error {
	rows := make([][]string, 0, len(views))
	for _, v := range views {
		expires := v.ExpiresAt
		switch {
		case expires == "":
			expires = "unknown"
		case v.Refreshable:
			expires += " (refreshable)"
		}
		rows = append(rows, []string{v.Resource, v.Issuer, v.ClientID, dashIfEmpty(strings.Join(v.Scopes, " ")), expires})
	}
	return p.PrintTable([]string{"RESOURCE", "ISSUER", "CLIENT", "SCOPES", "EXPIRES"}, rows)
}

// grantsForAgent returns the stored grants that apply to target: those whose
// resource is target or a parent of it, not every grant on the same host.
func (g *globalConfig) grantsForAgent(target string) ([]oauth.Grant, error) {
	if g.oauth == nil {
		return nil, nil
	}
	grants, err := g.oauth.List()
	if err != nil {
		return nil, err
	}
	var out []oauth.Grant
	for _, gr := range grants {
		if oauth.ResourceCovers(gr.Key.Resource, target) {
			out = append(out, gr)
		}
	}
	return out, nil
}
