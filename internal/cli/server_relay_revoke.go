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

func newServerRelayRevokeCmd(cfg *globalConfig) *cobra.Command {
	var rf relayClientFlags
	cmd := &cobra.Command{
		Use:   "revoke <name>",
		Short: "Revoke a grant",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := relayContext(cmd, cfg)
			defer cancel()
			c, err := rf.nodeClient()
			if err != nil {
				return err
			}
			if err := c.Revoke(ctx, args[0]); err != nil {
				return err
			}
			if cfg.IsJSON() {
				return cfg.PrintJSON(map[string]string{"revoked": args[0]})
			}
			_, err = fmt.Fprintf(cfg.Out, "Revoked %s.\n", args[0])
			return err
		},
	}
	rf.register(cmd.Flags())
	return cmd
}
