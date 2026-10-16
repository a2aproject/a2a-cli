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

	"github.com/spf13/cobra"
)

func newServerRelayPingCmd(cfg *globalConfig) *cobra.Command {
	var rf relayClientFlags
	cmd := &cobra.Command{
		Use:   "ping",
		Short: "Send this node a test wake",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := relayContext(cmd, cfg)
			defer cancel()
			c, err := rf.nodeClient()
			if err != nil {
				return err
			}
			res, err := c.Ping(ctx)
			if err != nil {
				return err
			}
			if cfg.IsJSON() {
				if err := cfg.PrintJSON(res); err != nil {
					return err
				}
			} else if res.Delivered {
				_, err := fmt.Fprintf(cfg.Out, "Webhook accepted test wake %s.\n", res.Envelope.ID)
				return err
			}
			if !res.Delivered {
				return fmt.Errorf("webhook did not accept test wake %s: %s", res.Envelope.ID, res.LastError)
			}
			return nil
		},
	}
	rf.register(cmd.Flags())
	return cmd
}
