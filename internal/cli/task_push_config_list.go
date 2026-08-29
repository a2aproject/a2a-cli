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

func newPushConfigListCmd(cfg *globalConfig) *cobra.Command {
	var (
		limit     int
		pageToken string
	)

	cmd := &cobra.Command{
		Use:   "list <task-id>",
		Short: "List a task's push-notification configurations",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), cfg.timeout)
			defer cancel()
			ctx = withServiceParams(ctx, cfg)

			client, err := newAgentClient(ctx, cfg)
			if err != nil {
				return fmt.Errorf("failed to create a client: %w", err)
			}
			defer destroyClient(cfg, client)

			configs, err := client.ListTaskPushConfigs(ctx, &a2a.ListTaskPushConfigRequest{
				Tenant:    cfg.tenant,
				TaskID:    a2a.TaskID(args[0]),
				PageSize:  limit,
				PageToken: pageToken,
			})
			if err != nil {
				return fmt.Errorf("failed to list push configs: %w", err)
			}
			if err := cfg.PrintPushConfigList(configs); err != nil {
				return fmt.Errorf("failed to print push configs: %w", err)
			}
			return nil
		},
	}

	f := cmd.Flags()
	f.IntVar(&limit, "limit", 0, "Page size")
	f.StringVar(&pageToken, "page-token", "", "Pagination token")
	return cmd
}
