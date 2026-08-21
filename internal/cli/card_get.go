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
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
)

func newCardGetCmd(cfg *globalConfig) *cobra.Command {
	var extended bool

	cmd := &cobra.Command{
		Use:   "get [url]",
		Short: "Fetch and display an agent card",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			ctx, cancel := context.WithTimeout(ctx, cfg.timeout)
			defer cancel()

			var card *a2a.AgentCard
			var err error
			if extended {
				card, err = getExtendedAgentCard(ctx, cfg)
			} else {
				card, err = getPublicAgentCard(ctx, cfg, args)
			}
			if err != nil {
				return err
			}

			if err := cfg.PrintCard(card); err != nil {
				return fmt.Errorf("failed to print agent card: %w", err)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&extended, "extended", false, "Fetch the extended agent card")
	return cmd
}

func getExtendedAgentCard(ctx context.Context, cfg *globalConfig) (*a2a.AgentCard, error) {
	ctx = withServiceParams(ctx, cfg)
	client, err := newAgentClient(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create a client: %w", err)
	}
	defer func() {
		if err := client.Destroy(); err != nil {
			cfg.logf("failed to destroy client: %v", err)
		}
	}()

	card, err := client.GetExtendedAgentCard(ctx, &a2a.GetExtendedAgentCardRequest{Tenant: cfg.tenant})
	if err != nil {
		return nil, fmt.Errorf("failed to get extended agent card: %w", err)
	}
	return card, nil
}

func getPublicAgentCard(ctx context.Context, cfg *globalConfig, args []string) (*a2a.AgentCard, error) {
	ref := cfg.agentCard
	if len(args) == 1 {
		ref = args[0]
	}
	if ref == "" {
		return nil, fmt.Errorf("specify the agent card URL as an argument or with --agent-card")
	}

	var resolveOpts []agentcard.ResolveOption
	if auth := cfg.svcParams.Auth(); auth != "" {
		resolveOpts = append(resolveOpts, agentcard.WithRequestHeader("Authorization", auth))
	}
	cfg.logf("fetching agent card from %s", ref)

	var err error
	card, err := compatCardResolver.Resolve(ctx, ref, resolveOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve agent card: %w", err)
	}
	return card, nil
}
