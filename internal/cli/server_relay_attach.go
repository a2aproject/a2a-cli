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
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-cli/internal/relay"
)

func newServerRelayAttachCmd(cfg *globalConfig) *cobra.Command {
	var (
		rf      relayClientFlags
		name    string
		desc    string
		headers []string
		public  bool
	)
	cmd := &cobra.Command{
		Use:   "attach [webhook-url]",
		Short: "Attach a node; prints its A2A URL and a pairing code",
		Long: `Attach an agent woken by a webhook to the relay. Prints the agent's A2A URL
and a one-time pairing code to give to the agent. With --node, issues a new
pairing code for an existing node (and updates its webhook if one is given),
which rotates the node token once the agent pairs again.`,
		Example: `  a2a server relay attach https://agent.example/webhook \
    --webhook-header "Authorization: Bearer env:ROUTINE_SECRET" --name my-agent \
    --relay https://relay.example --admin-token env:A2A_RELAY_ADMIN_TOKEN`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := relayContext(cmd, cfg)
			defer cancel()
			c, err := rf.adminClient()
			if err != nil {
				return err
			}
			hdrs, err := parseHeaders(headers)
			if err != nil {
				return err
			}
			var res *relay.PairingResponse
			if rf.opts.Node != "" {
				if len(args) > 0 || len(hdrs) > 0 {
					upd := relay.UpdateNodeRequest{WebhookHeaders: hdrs}
					if len(args) > 0 {
						upd.Webhook = &args[0]
					}
					if _, err := c.UpdateNode(ctx, upd); err != nil {
						return err
					}
				}
				res, err = c.NewPairing(ctx, rf.opts.Node)
			} else {
				if len(args) != 1 {
					return clierr.Usage("attach needs the webhook URL of the agent, or --node to re-pair an existing node")
				}
				res, err = c.CreateNode(ctx, relay.CreateNodeRequest{
					Name: name, Description: desc, Webhook: args[0], WebhookHeaders: hdrs, Public: public,
				})
			}
			if err != nil {
				return err
			}
			if _, err := relay.UpdateClientConfig(func(cc *relay.ClientConfig) {
				cc.Relay, cc.AdminToken = c.BaseURL, c.Token
			}); err != nil {
				slog.Warn("could not save relay config", "error", err)
			}
			if cfg.IsJSON() {
				return cfg.PrintJSON(res)
			}
			n := res.Node
			access := "callers need a grant (" + relay.CLI + " grant <name>)"
			if n.Public {
				access = "anyone can call"
			}
			_, err = fmt.Fprintf(cfg.Out, `Attached %s (node %s), %s.

  A2A URL:      %s
  Agent card:   %s/.well-known/agent-card.json
  Pairing code: %s (expires %s)

Tell the agent:

  Install the skill at %s/skill.md and pair with the relay using code %s:
    curl -fsSL -o a2a %s/download/a2a && chmod +x a2a
    ./a2a server relay pair %s --relay %s
`, n.Name, n.ID, access, n.URL, n.URL, res.PairingCode, res.ExpiresAt.Local().Format("15:04"),
				c.BaseURL, res.PairingCode, c.BaseURL, res.PairingCode, c.BaseURL)
			return err
		},
	}
	rf.register(cmd.Flags())
	f := cmd.Flags()
	f.StringVar(&name, "name", "", "Agent name in the card")
	f.StringVar(&desc, "description", "", "Agent description in the card")
	f.StringArrayVar(&headers, "webhook-header", nil, `Header sent with every wake, "Name: value"; the value may be env:VAR (repeatable)`)
	f.BoolVar(&public, "public", false, "Let anyone call the agent; by default callers need a grant")
	return cmd
}
