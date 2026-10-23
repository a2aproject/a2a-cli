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
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-cli/internal/controlapi"
	"github.com/a2aproject/a2a-cli/internal/relayapi"
)

func newServerPairCmd(cfg *globalConfig, sf *serverFlags) *cobra.Command {
	var (
		req     controlapi.PairCodeRequest
		headers []string
	)
	cmd := &cobra.Command{
		Use:   "pair",
		Short: "Mint a one-time code that lets someone call or message the agent, or host a webhook node",
		Long: `Mint an 8-character code, valid once for 15 minutes. The caller exchanges it
for a token with ` + "`a2a pair add <agent-url> <code>`" + `, or links a messenger
account by sending "link <code>" to the agent's bot.

With --webhook, <name> is a webhook node: an agent which only runs when woken,
such as a Grok Bot routine. The agent hosts it at <agent-url>/n/<name>, calls
it as <name> with the a2a CLI, and wakes it with a signed POST to the
webhook. The node pairs with the code the same way and answers with
` + "`a2a relay reply`" + `. Running pair again for a node issues a new code, for
example after it lost its token; with --webhook it also updates the node, and
its previous token stops working. Header values may be env:VAR.`,
		Example: `  a2a server pair --name grok
  a2a server pair --name anna --role owner
  a2a server pair --name grok --webhook https://example.com/hooks/123 \
    --webhook-header "Authorization: Bearer env:GROK_WEBHOOK_TOKEN"`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("role") {
				req.Role = ""
			}
			if req.Webhook == "" && (len(headers) > 0 || req.Description != "") {
				return clierr.Usage("--webhook-header and --description go with --webhook")
			}
			hdrs, err := parseWebhookHeaders(headers)
			if err != nil {
				return err
			}
			req.Headers = hdrs
			c, err := sf.control()
			if err != nil {
				return err
			}
			resp, err := c.Pair(cmd.Context(), req)
			if err != nil {
				return err
			}
			if cfg.IsJSON() {
				return cfg.PrintJSON(resp)
			}
			return printPairCode(cfg, resp)
		},
	}
	f := cmd.Flags()
	f.StringVar(&req.Name, "name", "", "Who the code is for; becomes the principal pair:<name>, or the node's name (required)")
	f.StringVar(&req.Role, "role", "member", "Role: member (can talk) or owner (can also approve)")
	f.StringVar(&req.Webhook, "webhook", "", "Host a webhook node: the URL the agent POSTs wakes to")
	f.StringArrayVar(&headers, "webhook-header", nil, "Header sent with every wake, \"Name: value\" (repeatable)")
	f.StringVar(&req.Description, "description", "", "Description for the node's card")
	if err := cmd.MarkFlagRequired("name"); err != nil {
		panic(err)
	}
	cmd.AddCommand(newServerPairListCmd(cfg, sf), newServerPairRmCmd(cfg, sf))
	return cmd
}

func parseWebhookHeaders(headers []string) (map[string]string, error) {
	out := map[string]string{}
	for _, h := range headers {
		k, v, ok := strings.Cut(h, ":")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, clierr.Usage(fmt.Sprintf("--webhook-header must be \"Name: value\", got %q", h))
		}
		v = strings.TrimSpace(v)
		if i := strings.Index(v, "env:"); i >= 0 {
			secret, err := relayapi.Secret(v[i:])
			if err != nil {
				return nil, clierr.Usage(fmt.Sprintf("--webhook-header %s: %v", strings.TrimSpace(k), err))
			}
			v = v[:i] + secret
		}
		out[strings.TrimSpace(k)] = v
	}
	return out, nil
}

func printPairCode(cfg *globalConfig, resp *controlapi.PairCodeResponse) error {
	mins := int(time.Until(resp.Expires).Round(time.Minute).Minutes())
	var sb strings.Builder
	if resp.Node != nil {
		fmt.Fprintf(&sb, "→ node code %s (%d min, single use); %s is reachable for the agent as %q\n  node: %s\n  node URL: %s\n",
			resp.Code, mins, resp.Node.Name, resp.Node.ID, resp.PairCommand, resp.Node.URL)
	} else {
		fmt.Fprintf(&sb, "→ code %s (%d min, single use)\n  caller: %s\n", resp.Code, mins, resp.PairCommand)
	}
	for _, h := range resp.Hints {
		sb.WriteString("  " + h + "\n")
	}
	_, err := fmt.Fprint(cfg.Out, sb.String())
	return err
}
