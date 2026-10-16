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

	"github.com/a2aproject/a2a-cli/internal/localsrv"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
)

func newServerProxyCmd(cfg *globalConfig) *cobra.Command {
	var sf serverFlags
	cmd := &cobra.Command{
		Use:   "proxy",
		Short: "Forward requests to the agent selected with --agent-card or --endpoint",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signalContext(cmd.Context())
			defer stop()
			sc, err := sf.listen(cfg)
			if err != nil {
				return err
			}
			var opts []a2aclient.FactoryOption
			if !sf.quiet {
				opts = append(opts, localsrv.ProxyLogInterceptorOption())
			}
			client, err := newAgentClient(ctx, cfg, opts...)
			if err != nil {
				return fmt.Errorf("creating upstream client: %w", err)
			}
			return localsrv.ServeProxy(ctx, sc, cfg.svcParams, client, cfg.tenant)
		},
	}
	sf.register(cmd.Flags())
	return cmd
}
