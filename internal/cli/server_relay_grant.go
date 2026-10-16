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

func newServerRelayGrantCmd(cfg *globalConfig) *cobra.Command {
	var rf relayClientFlags
	cmd := &cobra.Command{
		Use:   "grant <name | node:<id>>",
		Short: "Issue a caller token, or allow another node on the relay",
		Long: `Issue a caller token for name (shown once; granting the same name again
rotates it), or allow node:<id>, another node on this relay, which needs no
token.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := relayContext(cmd, cfg)
			defer cancel()
			c, err := rf.nodeClient()
			if err != nil {
				return err
			}
			res, err := c.Grant(ctx, args[0])
			if err != nil {
				return err
			}
			if cfg.IsJSON() {
				return cfg.PrintJSON(res)
			}
			if res.Token == "" {
				_, err = fmt.Fprintf(cfg.Out, "Allowed %s.\n", res.Name)
				return err
			}
			_, err = fmt.Fprintf(cfg.Out, "Granted %s. Caller token (shown once):\n\n  %s\n\nThe caller sends it as Authorization: Bearer <token>.\n", res.Name, res.Token)
			return err
		},
	}
	rf.register(cmd.Flags())
	return cmd
}
