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

	"github.com/a2aproject/a2a-cli/internal/relay"
)

func newServerRelayPairCmd(cfg *globalConfig) *cobra.Command {
	var (
		rf     relayClientFlags
		noSave bool
	)
	cmd := &cobra.Command{
		Use:   "pair <code>",
		Short: "Exchange a pairing code for a node token",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := relayContext(cmd, cfg)
			defer cancel()
			cc, err := relay.LoadClientConfig()
			if err != nil {
				return err
			}
			base := rf.opts.RelayURL(cc)
			if base == "" {
				return fmt.Errorf("set --relay or %s", relay.EnvRelayURL)
			}
			c := &relay.Client{BaseURL: base}
			res, err := c.Pair(ctx, args[0])
			if err != nil {
				return err
			}
			var saved string
			if !noSave {
				saved, err = relay.UpdateClientConfig(func(cc *relay.ClientConfig) { cc.Relay, cc.Token = base, res.Token })
				if err != nil {
					slog.Warn("could not save relay config", "error", err)
					saved = ""
				}
			}
			if cfg.IsJSON() {
				return cfg.PrintJSON(res)
			}
			if _, err := fmt.Fprintf(cfg.Out, "Paired as %s (node %s).\n\n  A2A URL:    %s\n  Node token: %s\n\n",
				res.Node.Name, res.Node.ID, res.Node.URL, res.Token); err != nil {
				return err
			}
			if saved != "" {
				if _, err := fmt.Fprintf(cfg.Out, "Saved to %s. ", saved); err != nil {
					return err
				}
			}
			_, err = fmt.Fprintf(cfg.Out, "Keep the token as the secret %s; it is shown only once and includes the relay URL.\n", relay.EnvNodeToken)
			return err
		},
	}
	rf.register(cmd.Flags())
	cmd.Flags().BoolVar(&noSave, "no-save", false, "Don't save the token to ~/.config/a2a-cli/relay.json")
	return cmd
}
