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
	"net/http"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/credstore"
	"github.com/a2aproject/a2a-cli/internal/relayapi"
)

func newPairAddCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "add <agent-url> <code>",
		Short: "Exchange a pairing code for a token and save it",
		Long: `Exchange a pairing code from an agent's owner (a2a server pair invite) for a token.
The token is saved to ~/.config/a2a-cli/credentials.json, and later commands
for the same agent, such as a2a send, attach it.

A code for a webhook node (a2a server pair invite --webhook) returns a node token
instead, which the a2a relay commands use.`,
		Example: "  a2a pair add https://assistant.tail1234.ts.net 3MZ8-LQ0D",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), cfg.timeout)
			defer cancel()
			c, err := credstore.Pair(ctx, &http.Client{Transport: cfg.httpTransport}, args[0], args[1])
			if err != nil {
				return err
			}
			store, err := cfg.credentialStore()
			if err != nil {
				return err
			}
			if err := store.Put(c); err != nil {
				return err
			}
			if c.Kind == credstore.KindNode {
				return printNodePaired(cfg, c, store.Path)
			}
			if cfg.IsJSON() {
				return cfg.PrintJSON(map[string]string{"url": c.URL, "agent": c.Agent, "principal": c.Principal, "saved": store.Path})
			}
			_, err = fmt.Fprintf(cfg.Out, "✓ paired with %s as %s; token saved to %s\n", c.Agent, c.Principal, store.Path)
			return err
		},
	}
}

func printNodePaired(cfg *globalConfig, c credstore.Credential, saved string) error {
	if cfg.IsJSON() {
		return cfg.PrintJSON(map[string]string{"url": c.URL, "node": c.Agent, "principal": c.Principal, "token": c.Token, "saved": saved})
	}
	agent, _ := relayapi.AgentURL(c.Token)
	_, err := fmt.Fprintf(cfg.Out, "✓ paired as node %s at %s; token saved to %s\n\n  Node token: %s\n\nKeep the token as the secret %s if the node runs elsewhere; it includes the agent's URL.\nThe node calls the agent with: a2a send -a %s \"…\"\n",
		c.Agent, c.URL, saved, c.Token, relayapi.EnvNodeToken, agent)
	return err
}
