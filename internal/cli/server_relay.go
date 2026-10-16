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
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/a2aproject/a2a-cli/internal/relay"
)

func newServerRelayCmd(cfg *globalConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "relay",
		Short: "Run or use a relay for agents without a public address (prototype)",
		Long: `A relay gives agents that can't accept connections or stay running, such as
cloud agents woken by a webhook, a public A2A URL. Callers talk standard A2A
to the relay; the relay wakes the agent (the node) through its webhook, and
the node answers with "reply".

Operator:  serve
Owner:     attach
Node:      pair, reply, task, inbox, send, grant, revoke, grants, ping, status, update, skill

Client commands find the relay and token in --relay/--token, A2A_RELAY_URL /
A2A_RELAY_TOKEN, or ~/.config/a2a-cli/relay.json, which pair and attach write.
The owner can run node commands for any node with --admin-token and --node.`,
	}
	cmd.AddCommand(
		newServerRelayServeCmd(cfg),
		newServerRelayAttachCmd(cfg),
		newServerRelayPairCmd(cfg),
		newServerRelayReplyCmd(cfg),
		newServerRelayTaskCmd(cfg),
		newServerRelayInboxCmd(cfg),
		newServerRelaySendCmd(cfg),
		newServerRelayGrantCmd(cfg),
		newServerRelayRevokeCmd(cfg),
		newServerRelayGrantsCmd(cfg),
		newServerRelayPingCmd(cfg),
		newServerRelayStatusCmd(cfg),
		newServerRelayUpdateCmd(cfg),
		newServerRelaySkillCmd(cfg),
	)
	return cmd
}

// relayClientFlags select the relay and credentials of a relay client command.
type relayClientFlags struct {
	opts relay.ClientOptions
}

func (r *relayClientFlags) register(f *pflag.FlagSet) {
	f.StringVar(&r.opts.Relay, "relay", "", "Relay URL (default: $"+relay.EnvRelayURL+", the node token or the config file)")
	f.StringVar(&r.opts.Token, "token", "", "Node token, or env:VAR (default: $"+relay.EnvNodeToken+" or the config file)")
	f.StringVar(&r.opts.AdminToken, "admin-token", "", "Relay admin token, or env:VAR (default: $"+relay.EnvAdminToken+" or the config file)")
	f.StringVar(&r.opts.Node, "node", "", "Node ID, when the owner acts on a node with the admin token")
}

func (r *relayClientFlags) nodeClient() (*relay.Client, error) {
	cc, err := relay.LoadClientConfig()
	if err != nil {
		return nil, err
	}
	return r.opts.NodeClient(cc)
}

func (r *relayClientFlags) adminClient() (*relay.Client, error) {
	cc, err := relay.LoadClientConfig()
	if err != nil {
		return nil, err
	}
	return r.opts.AdminClient(cc)
}

func relayContext(cmd *cobra.Command, cfg *globalConfig) (context.Context, context.CancelFunc) {
	return context.WithTimeout(cmd.Context(), cfg.timeout)
}

// parseHeaders parses "Name: value" flags. Values may be env:VAR references.
func parseHeaders(hs []string) (map[string]string, error) {
	if len(hs) == 0 {
		return nil, nil
	}
	out := map[string]string{}
	for _, h := range hs {
		k, v, ok := strings.Cut(h, ":")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, fmt.Errorf(`webhook header must look like "Name: value", got %q`, h)
		}
		out[strings.TrimSpace(k)] = relay.Secret(strings.TrimSpace(v))
	}
	return out, nil
}

func printRelayNode(cfg *globalConfig, n *relay.NodeInfo) error {
	if cfg.IsJSON() {
		return cfg.PrintJSON(n)
	}
	_, err := fmt.Fprintf(cfg.Out, "%s (node %s)\n  A2A URL: %s\n  Webhook: %s\n  Public:  %v\n  Paired:  %v\n  Grants:  %s\n",
		n.Name, n.ID, n.URL, n.Webhook, n.Public, n.Paired, strings.Join(n.Grants, ", "))
	return err
}

func oneLine(s string, limit int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > limit {
		s = s[:limit] + "…"
	}
	return s
}
