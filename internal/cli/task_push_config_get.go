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

	"github.com/a2aproject/a2a-go/v2/a2a"
)

func newPushConfigGetCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "get <task-id> <config-id>",
		Short: "Get a task's push-notification configuration",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), cfg.timeout)
			defer cancel()
			ctx = withServiceParams(ctx, cfg)

			client, err := newAgentClient(ctx, cfg)
			if err != nil {
				return fmt.Errorf("failed to create a client: %w", err)
			}
			defer destroyClient(cfg, client)

			result, err := client.GetTaskPushConfig(ctx, &a2a.GetTaskPushConfigRequest{
				Tenant: cfg.tenant,
				TaskID: a2a.TaskID(args[0]),
				ID:     args[1],
			})
			if err != nil {
				return fmt.Errorf("failed to get push config %s: %w", args[1], err)
			}
			if err := cfg.PrintPushConfig(result); err != nil {
				return fmt.Errorf("failed to print push config: %w", err)
			}
			return nil
		},
	}
}
