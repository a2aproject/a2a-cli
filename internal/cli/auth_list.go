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
	"io"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/output"
	"github.com/a2aproject/a2a-go/v2/a2aclient/oauth"
)

// grantView is the JSON/text view of a stored OAuth grant.
type grantView struct {
	Resource     string             `json:"resource"`
	Issuer       string             `json:"issuer"`
	ClientID     string             `json:"clientId"`
	ClientSource oauth.ClientSource `json:"clientSource,omitempty"`
	Scopes       []string           `json:"scopes,omitempty"`
	ExpiresAt    string             `json:"expiresAt,omitempty"`
	Refreshable  bool               `json:"refreshable"`
	AccessToken  string             `json:"accessToken,omitempty"`
}

func newGrantView(g oauth.Grant, reveal bool) grantView {
	v := grantView{
		Resource:     g.Resource,
		Issuer:       g.Issuer,
		ClientID:     g.ClientID,
		ClientSource: g.ClientSource,
		Scopes:       g.Scopes,
		Refreshable:  g.Refreshable || g.RefreshToken != "",
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
			if err := cfg.requireOAuth(); err != nil {
				return err
			}
			grants, err := cfg.oauth.Store.List(cmd.Context())
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

// agentGrants are the stored grants for an agent reference.
type agentGrants struct {
	// selected are the agent's own grants.
	selected []oauth.Grant
	// alsoApply are other grants that may apply to the agent: grants for a
	// parent URL, such as its host, which other agents may share, and, when
	// the agent card can't be read, grants below the URL.
	alsoApply []oauth.Grant
	// cardFound reports whether the agent card was read.
	cardFound bool
}

// grantsForAgent finds the stored grants for the agent at ref. A grant whose
// resource is ref itself is always the agent's. Grants obtained through the
// card are kept under its interface URLs, so it resolves ref's agent card, best
// effort, and also selects the grants for those of its interfaces at or below
// ref and, when ref has a path, the grants below ref. A card can name any URL:
// grants for its other interfaces on ref's origin, which may belong to other
// agents on a host that routes agents by path, are listed as also applying, and
// interfaces on other origins are left out. Without a card it selects nothing
// else: guessing could revoke the grants of other agents on the host. Grants
// for a parent URL are never selected, since other agents may share them.
func (g *globalConfig) grantsForAgent(ctx context.Context, ref string) (*agentGrants, error) {
	if err := g.requireOAuth(); err != nil {
		return nil, err
	}
	grants, err := g.oauth.Store.List(ctx)
	if err != nil || len(grants) == 0 {
		return &agentGrants{}, err
	}
	own, others, found := g.interfaceURLs(ctx, ref)
	target := oauth.NormalizeResource(ref)
	belowRef := found && hasPath(target)
	covered := func(resource string) bool {
		return covers(resource, target) || slices.ContainsFunc(own, func(i string) bool { return covers(resource, i) })
	}
	out := &agentGrants{cardFound: found}
	for _, gr := range grants {
		resource := oauth.NormalizeResource(gr.Resource)
		switch {
		case resource == target || slices.Contains(own, resource) || belowRef && covers(target, resource):
			out.selected = append(out.selected, gr)
		case slices.Contains(others, resource) || covered(resource) || !found && covers(target, resource):
			out.alsoApply = append(out.alsoApply, gr)
		}
	}
	return out, nil
}

// interfaceURLs returns the normalized URLs of the interfaces of the agent card
// at ref: those at or below ref, then the others on ref's origin, and false
// when the card can't be fetched.
func (g *globalConfig) interfaceURLs(ctx context.Context, ref string) (own, others []string, found bool) {
	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()
	card, _, err := fetchCardDocument(ctx, g, ref, false)
	if err != nil {
		g.logf("selecting grants by URL: reading the agent card of %s failed: %v", ref, err)
		return nil, nil, false
	}
	base := agentBase(ref)
	for _, iface := range card.SupportedInterfaces {
		u := oauth.NormalizeResource(iface.URL)
		switch {
		case covers(base, u):
			own = append(own, u)
		case origin(u) == origin(base):
			g.logf("selecting grants by URL: the interface %s is not at or below %s, so its grant is only listed", u, base)
			others = append(others, u)
		default:
			g.logf("selecting grants by URL: ignoring the interface %s, which is not on the origin of %s", u, ref)
		}
	}
	return own, others, true
}

// agentBase returns ref without the well-known agent card path, normalized.
func agentBase(ref string) string {
	r := oauth.NormalizeResource(ref)
	return strings.TrimSuffix(r, "/.well-known/agent-card.json")
}

// covers reports whether a grant for resource applies to requests to target.
func covers(resource, target string) bool {
	return oauth.Grant{Resource: resource}.Covers(target)
}

// origin returns scheme://host[:port] of a URL, normalized, or "" when it
// doesn't parse.
func origin(raw string) string {
	u, err := url.Parse(oauth.NormalizeResource(raw))
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func hasPath(resource string) bool {
	u, err := url.Parse(resource)
	return err == nil && u.Path != ""
}
