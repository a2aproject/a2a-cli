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

type pushConfigCreateFlags struct {
	taskID          string
	tenant          string
	url             string
	id              string
	token           string
	authScheme      string
	authCredentials string
}

func newPushConfigCreateCmd(cfg *globalConfig) *cobra.Command {
	var f pushConfigCreateFlags

	cmd := &cobra.Command{
		Use:   "create <task-id>",
		Short: "Create a push-notification configuration for a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			f.taskID = args[0]
			f.tenant = cfg.tenant
			pc, err := buildPushConfig(f)
			if err != nil {
				return err
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), cfg.timeout)
			defer cancel()
			ctx = withServiceParams(ctx, cfg)

			client, err := newAgentClient(ctx, cfg)
			if err != nil {
				return fmt.Errorf("failed to create a client: %w", err)
			}
			defer destroyClient(cfg, client)

			result, err := client.CreateTaskPushConfig(ctx, pc)
			if err != nil {
				return fmt.Errorf("failed to create push config: %w", err)
			}
			if err := cfg.PrintPushConfig(result); err != nil {
				return fmt.Errorf("failed to print push config: %w", err)
			}
			return nil
		},
	}

	fl := cmd.Flags()
	fl.StringVar(&f.url, "url", "", "Webhook callback URL the agent posts updates to (required)")
	fl.StringVar(&f.id, "id", "", "Optional client-set configuration ID (allows multiple callbacks)")
	fl.StringVar(&f.token, "token", "", "Optional token the agent echoes back so the receiver can validate calls")
	fl.StringVar(&f.authScheme, "auth-scheme", "", "Optional auth scheme the agent uses when calling the webhook (e.g. Bearer)")
	fl.StringVar(&f.authCredentials, "auth-credentials", "", "Optional credentials the agent presents to the webhook")
	return cmd
}

func buildPushConfig(f pushConfigCreateFlags) (*a2a.PushConfig, error) {
	if f.url == "" {
		return nil, fmt.Errorf("--url is required")
	}
	pc := &a2a.PushConfig{
		Tenant: f.tenant,
		TaskID: a2a.TaskID(f.taskID),
		ID:     f.id,
		Token:  f.token,
		URL:    f.url,
	}
	if f.authScheme != "" || f.authCredentials != "" {
		if f.authScheme == "" {
			return nil, fmt.Errorf("--auth-scheme is required when --auth-credentials is set")
		}
		pc.Auth = &a2a.PushAuthInfo{Scheme: f.authScheme, Credentials: f.authCredentials}
	}
	return pc, nil
}
