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

	"github.com/a2aproject/a2a-cli/internal/agentsrv"
)

func newServerLogsCmd(cfg *globalConfig, sf *serverFlags) *cobra.Command {
	var (
		follow bool
		lines  int
	)
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Print the agent's log",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := sf.agentDir()
			if err != nil {
				return err
			}
			ctx, stop := signalContext(cmd.Context())
			defer stop()
			return agentsrv.FollowLog(ctx, d, lines, follow, cfg.Out)
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Keep printing new lines")
	cmd.Flags().IntVarP(&lines, "lines", "n", 100, "Number of lines to print first; 0 prints all")
	return cmd
}
