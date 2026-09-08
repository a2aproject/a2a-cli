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
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/polling"
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
)

func newTaskGetCmd(cfg *globalConfig) *cobra.Command {
	var history int
	var wait bool
	var pollInterval time.Duration

	cmd := &cobra.Command{
		Use:   "get <task-id>",
		Short: "Get task details",
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

			req := &a2a.GetTaskRequest{
				ID:     a2a.TaskID(args[0]),
				Tenant: cfg.tenant,
			}
			if cmd.Flags().Changed("history") {
				req.HistoryLength = &history
			}

			task, err := getTask(ctx, cfg, client, req, wait, pollInterval)
			if err != nil {
				return err
			}

			if err := cfg.PrintTask(task); err != nil {
				return fmt.Errorf("failed to print task: %w", err)
			}
			return nil
		},
	}

	f := cmd.Flags()
	f.IntVar(&history, "history", 0, "Include up to n history messages")
	f.BoolVar(&wait, "wait", false, "Poll until the task reaches a terminal or interrupted (input/auth-required) state")
	f.DurationVar(&pollInterval, "poll-interval", 2*time.Second, "Duration between polls while waiting; only used with --wait. Overall wait budget is --timeout.")
	return cmd
}

func getTask(ctx context.Context, cfg *globalConfig, client *a2aclient.Client, req *a2a.GetTaskRequest, wait bool, interval time.Duration) (*a2a.Task, error) {
	if !wait {
		task, err := client.GetTask(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("failed to get task %s: %w", req.ID, err)
		}
		return task, nil
	}

	cfg.logf("waiting for task %s (poll interval %v, timeout %v)", req.ID, interval, cfg.timeout)

	task, err := polling.WaitForTask(ctx, client, req, interval)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("timed out after %s waiting for task %s: %w", cfg.timeout, req.ID, err)
		}
		return nil, fmt.Errorf("failed to wait for task %s: %w", req.ID, err)
	}
	return task, nil
}
