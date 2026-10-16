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
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/harness/a2aclaude"
	"github.com/a2aproject/a2a-cli/internal/localsrv"
	"github.com/a2aproject/a2a-go/v2/a2a"
)

func newServerClaudeCmd(cfg *globalConfig) *cobra.Command {
	var (
		hf       harnessFlags
		model    string
		permMode string
		allowed  []string
	)
	cmd := &cobra.Command{
		Use:   "claude",
		Short: "Serve a Claude Code agent (prototype)",
		Long: `Serve Claude Code as an A2A agent. Every turn runs claude -p with stream-json
output and resumes the session of the A2A context. Tools outside
--allowed-tools pause the task in input-required; an approval resumes the
session with the exact calls allowed.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signalContext(cmd.Context())
			defer stop()
			setup, err := hf.setup(cfg)
			if err != nil {
				return err
			}
			executor := a2aclaude.NewExecutor(a2aclaude.Config{
				Binary: hf.bin, Args: hf.harnessArgs, Dir: hf.workspace, Log: setup.log,
				Sessions: setup.sessions, Questions: setup.questions,
				Model: model, PermissionMode: permMode, AllowedTools: allowed,
			})
			hf.cardDefaults("claude", "A Claude Code agent working in "+filepath.Base(hf.workspace))
			sc, err := hf.listen(cfg)
			if err != nil {
				return err
			}
			sc.Skills = []a2a.AgentSkill{codingSkill("claude-code")}
			return localsrv.ServeAgent(ctx, sc, executor)
		},
	}
	hf.register(cmd.Flags(), "claude")
	f := cmd.Flags()
	f.StringVar(&model, "model", "", "Claude model")
	f.StringVar(&permMode, "permission-mode", "", "Claude Code --permission-mode, e.g. acceptEdits")
	f.StringSliceVar(&allowed, "allowed-tools", []string{"Read", "Glob", "Grep", "LS"}, "Claude Code tools allowed without asking")
	return cmd
}
