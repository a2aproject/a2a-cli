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
	"log/slog"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/harness/a2aopencode"
	"github.com/a2aproject/a2a-cli/internal/localsrv"
	"github.com/a2aproject/a2a-go/v2/a2a"
)

func newServerOpencodeCmd(cfg *globalConfig) *cobra.Command {
	var (
		hf     harnessFlags
		attach string
		model  string
		agent  string
		ask    []string
	)
	cmd := &cobra.Command{
		Use:   "opencode",
		Short: "Serve an opencode agent (prototype)",
		Long: `Serve opencode, or a fork of it, as an A2A agent. One opencode server runs
per process and every A2A context maps to an opencode session. Permission
requests and questions pause the task in input-required until the caller
answers.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signalContext(cmd.Context())
			defer stop()
			setup, err := hf.setup(cfg)
			if err != nil {
				return err
			}
			hf.cardDefaults("opencode", "An opencode coding agent working in "+filepath.Base(hf.workspace))
			sc, err := hf.listen(cfg)
			if err != nil {
				return err
			}
			sc.Skills = []a2a.AgentSkill{codingSkill("opencode")}

			var rules []a2aopencode.PermissionRule
			for _, p := range ask {
				rules = append(rules, a2aopencode.PermissionRule{Permission: p, Pattern: "*", Action: "ask"})
			}
			executor, err := a2aopencode.NewExecutor(ctx, a2aopencode.Config{
				Binary: hf.bin, Args: hf.harnessArgs, Dir: hf.workspace, URL: attach, Log: setup.log,
				Sessions: setup.sessions, Questions: setup.questions,
				Model: model, Agent: agent, Permission: rules,
			})
			if err != nil {
				return err
			}
			defer executor.Close()
			slog.Info("opencode is up", "url", executor.URL(), "workspace", hf.workspace, "state", hf.stateDir)

			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			go func() {
				select {
				case <-executor.Done():
					slog.Error("opencode exited", "error", executor.Err())
					cancel()
				case <-ctx.Done():
				}
			}()

			return localsrv.ServeAgent(ctx, sc, executor)
		},
	}
	hf.register(cmd.Flags(), "opencode")
	f := cmd.Flags()
	f.StringVar(&attach, "attach", "", "URL of a running opencode server to use instead of starting one")
	f.StringVar(&model, "model", "", "Model as provider/model")
	f.StringVar(&agent, "agent", "", "opencode agent, e.g. build or plan")
	f.StringSliceVar(&ask, "ask", []string{"bash", "edit", "webfetch", "external_directory"}, "opencode permissions that require approval in A2A sessions")
	return cmd
}

func codingSkill(id string) a2a.AgentSkill {
	return a2a.AgentSkill{
		ID: id, Name: "Coding agent", Tags: []string{"coding", "shell", "files"},
		Description: "Reads and edits files and runs commands in its workspace. Tool calls that need approval pause the task in input-required.",
	}
}
