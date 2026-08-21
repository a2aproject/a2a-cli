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

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/flagparse"
	"github.com/a2aproject/a2a-go/v2/a2a"
)

func newTaskCancelCmd(cfg *globalConfig) *cobra.Command {
	var meta flagparse.Metadata

	cmd := &cobra.Command{
		Use:   "cancel <task-id>",
		Short: "Cancel a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), cfg.timeout)
			defer cancel()
			ctx = withServiceParams(ctx, cfg)

			client, err := newAgentClient(ctx, cfg)
			if err != nil {
				return fmt.Errorf("failed to create a client: %w", err)
			}
			defer func() {
				if err := client.Destroy(); err != nil {
					cfg.logf("failed to destroy client: %v", err)
				}
			}()

			req := &a2a.CancelTaskRequest{
				ID:     a2a.TaskID(args[0]),
				Tenant: cfg.tenant,
			}
			meta.ApplyTo(req)

			task, err := client.CancelTask(ctx, req)
			if err != nil {
				return fmt.Errorf("failed to cancel task %s: %w", args[0], err)
			}

			if err := cfg.PrintTask(task); err != nil {
				return fmt.Errorf("failed to print task: %w", err)
			}
			return nil
		},
	}

	meta.Attach(cmd.Flags(), "metadata", "Attach request metadata as a JSON object (repeatable)")
	return cmd
}
