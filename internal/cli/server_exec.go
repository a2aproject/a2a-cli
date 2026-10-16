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

func newServerExecCmd(cfg *globalConfig) *cobra.Command {
	var (
		sf    serverFlags
		chunk string
	)
	cmd := &cobra.Command{
		Use:   "exec <command>",
		Short: "Serve a shell command as an agent",
		Long: `Serve a shell command as an agent. Each incoming message is piped to the
command's stdin and its stdout becomes the response.`,
		Example: `  a2a server exec "python3 -u agent.py"
  a2a server exec "for i in 1 2 3; do echo $i; sleep 0.5; done" --chunk=$'\n'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signalContext(cmd.Context())
			defer stop()
			sc, err := sf.listen(cfg)
			if err != nil {
				return err
			}
			return localsrv.ServeExec(ctx, sc, args[0], chunk)
		},
	}
	sf.register(cmd.Flags())
	cmd.Flags().StringVar(&chunk, "chunk", "", "Delimiter for streaming the command output")
	return cmd
}
