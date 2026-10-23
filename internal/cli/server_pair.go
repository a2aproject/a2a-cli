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
	"github.com/spf13/cobra"
)

func newServerPairCmd(cfg *globalConfig, sf *serverFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pair",
		Short: "Invite callers and webhook nodes, and manage who is paired with the agent",
		Long: `Manage who may call or message the agent. "invite" creates a one-time code,
which the caller redeems on their side with ` + "`a2a pair add <agent-url> <code>`" + `,
or by sending "link <code>" to the agent's bot.`,
		Example: `  a2a server pair invite anna
  a2a server pair list
  a2a server pair rm anna`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newServerPairInviteCmd(cfg, sf), newServerPairListCmd(cfg, sf), newServerPairRmCmd(cfg, sf))
	return cmd
}
