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
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/a2aproject/a2a-cli/internal/channel"
	"github.com/a2aproject/a2a-cli/internal/credstore"
	"github.com/a2aproject/a2a-cli/internal/relayapi"
)

func newRelayCmd(cfg *globalConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "relay",
		Short: "Answer tasks as a webhook node hosted by an agent server",
		Long: `Commands a webhook node runs: an agent which only runs when woken, such as a
Grok Bot routine, hosted by an agent server ("a2a server pair --webhook" on its side).
The node pairs once with "a2a pair add <agent-url> <code>", then answers each
wake with reply.

The node token comes from --token, $` + relayapi.EnvNodeToken + `, or the token
"a2a pair add" saved. It includes the agent's URL.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		newRelayReplyCmd(cfg),
		newRelayTaskCmd(cfg),
		newRelayInboxCmd(cfg),
	)
	return cmd
}

// relayClientFlags select the node token of a node command.
type relayClientFlags struct {
	token string
}

func (r *relayClientFlags) register(f *pflag.FlagSet) {
	f.StringVar(&r.token, "token", "", "Node token, or env:VAR (default: $"+relayapi.EnvNodeToken+" or the token saved by a2a pair add)")
}

func (r *relayClientFlags) nodeClient(cfg *globalConfig) (*relayapi.Client, error) {
	token, err := r.nodeToken(cfg)
	if err != nil {
		return nil, err
	}
	return relayapi.NodeClient(token)
}

func (r *relayClientFlags) nodeToken(cfg *globalConfig) (string, error) {
	if r.token != "" {
		return relayapi.Secret(r.token)
	}
	if v := os.Getenv(relayapi.EnvNodeToken); v != "" {
		return v, nil
	}
	store, err := cfg.credentialStore()
	if err != nil {
		return "", err
	}
	c, err := store.Node()
	if errors.Is(err, credstore.ErrNoNode) {
		return "", fmt.Errorf("no node token: run `a2a pair add <agent-url> <code>`, or set %s", relayapi.EnvNodeToken)
	}
	if err != nil {
		return "", err
	}
	return c.Token, nil
}

func relayContext(cmd *cobra.Command, cfg *globalConfig) (context.Context, context.CancelFunc) {
	return context.WithTimeout(cmd.Context(), cfg.timeout)
}

func oneLine(s string, limit int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > limit {
		s = channel.Truncate(s, limit) + "…"
	}
	return s
}
