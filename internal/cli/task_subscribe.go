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

	"github.com/a2aproject/a2a-cli/internal/utils"
	"github.com/a2aproject/a2a-go/v2/a2a"
)

func newTaskSubscribeCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "subscribe <task-id>",
		Short: "Subscribe to task events",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := withServiceParams(cmd.Context(), cfg)
			ctx, debounceTimeout := utils.WithInactivityTimeout(ctx, cfg.timeout)

			client, err := newAgentClient(ctx, cfg)
			if err != nil {
				return fmt.Errorf("failed to create a client: %w", err)
			}
			defer destroyClient(cfg, client)

			cfg.logf("subscribing to task %s", args[0])

			for event, err := range client.SubscribeToTask(ctx, &a2a.SubscribeToTaskRequest{
				ID:     a2a.TaskID(args[0]),
				Tenant: cfg.tenant,
			}) {
				debounceTimeout()
				if err != nil {
					return utils.UnpackCause(ctx, err)
				}
				if err := cfg.PrintEvent(event); err != nil {
					return fmt.Errorf("failed to print event: %w", err)
				}
			}
			return nil
		},
	}
}
