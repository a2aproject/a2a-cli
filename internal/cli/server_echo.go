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

	"github.com/a2aproject/a2a-cli/internal/localsrv"
)

func newServerEchoCmd(cfg *globalConfig) *cobra.Command {
	var sf serverFlags
	cmd := &cobra.Command{
		Use:   "echo",
		Short: "Serve an agent that returns the user's message as a response",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signalContext(cmd.Context())
			defer stop()
			sc, err := sf.listen(cfg)
			if err != nil {
				return err
			}
			return localsrv.ServeEcho(ctx, sc)
		},
	}
	sf.register(cmd.Flags())
	return cmd
}
