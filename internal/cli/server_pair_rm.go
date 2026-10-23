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

	"github.com/a2aproject/a2a-cli/internal/agentsrv"
)

func newServerPairRmCmd(cfg *globalConfig, sf *serverFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "rm <name>",
		Short: "Revoke the tokens and messenger links issued to a name, or remove a webhook node",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := sf.control()
			if err != nil {
				return err
			}
			resp, err := c.Revoke(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if resp.Kind == agentsrv.PairKindNode {
				_, err = fmt.Fprintf(cfg.Out, "✓ removed node %s; the agent can no longer wake it\n", args[0])
				return err
			}
			_, err = fmt.Fprintf(cfg.Out, "✓ %s can no longer call the agent\n", args[0])
			return err
		},
	}
}
