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

func newRelayTaskCmd(cfg *globalConfig) *cobra.Command {
	var rf relayClientFlags
	cmd := &cobra.Command{
		Use:   "task <task-id>",
		Short: "Show a task addressed to the node, with history and artifacts",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := relayContext(cmd, cfg)
			defer cancel()
			c, err := rf.nodeClient(cfg)
			if err != nil {
				return err
			}
			task, err := c.Task(ctx, args[0])
			if err != nil {
				return err
			}
			if cfg.IsJSON() {
				return cfg.PrintJSON(task)
			}
			if err := cfg.PrintEvent(task); err != nil {
				return err
			}
			return cfg.Flush()
		},
	}
	rf.register(cmd.Flags())
	return cmd
}
