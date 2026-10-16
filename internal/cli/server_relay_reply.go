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

	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-cli/internal/relay"
)

func newServerRelayReplyCmd(cfg *globalConfig) *cobra.Command {
	var (
		rf            relayClientFlags
		status        string
		text          string
		final         string
		inputRequired string
		fail          string
		artifacts     []string
		key           string
	)
	cmd := &cobra.Command{
		Use:   "reply <task-id>",
		Short: "Post progress, a question or the result of a task",
		Example: `  a2a server relay reply <task> --status working --text "on it"
  a2a server relay reply <task> --input-required "Which branch?"
  a2a server relay reply <task> --final "Done" --artifact report.pdf
  a2a server relay reply <task> --fail "Could not reach the repo"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID := args[0]
			var ev relay.Event
			set := 0
			for _, f := range []struct{ val, state string }{
				{final, "completed"}, {inputRequired, "input-required"}, {fail, "failed"},
			} {
				if f.val != "" {
					set++
					ev.State, ev.Text = f.state, f.val
				}
			}
			if status != "" {
				set++
				ev.State, ev.Text = status, text
			} else if text != "" {
				return clierr.Usage("--text goes with --status")
			}
			if set == 0 && len(artifacts) > 0 {
				ev.State, set = "completed", 1
			}
			if set != 1 {
				return clierr.Usage("set exactly one of --final, --input-required, --fail or --status")
			}
			if _, ok := relay.ParseState(ev.State); !ok {
				return clierr.Usage(fmt.Sprintf("unknown state %q", ev.State))
			}
			for _, path := range artifacts {
				art, err := relay.FileArtifact(path)
				if err != nil {
					return err
				}
				ev.Artifacts = append(ev.Artifacts, art)
			}
			ev.IdempotencyKey = key
			if ev.IdempotencyKey == "" {
				ev.IdempotencyKey = relay.ContentKey(taskID, ev)
			}

			ctx, cancel := relayContext(cmd, cfg)
			defer cancel()
			c, err := rf.nodeClient()
			if err != nil {
				return err
			}
			res, err := c.PostEvent(ctx, taskID, ev)
			if err != nil {
				return err
			}
			if cfg.IsJSON() {
				return cfg.PrintJSON(res)
			}
			note := ""
			if res.Duplicate {
				note = " (already applied, ignored the retry)"
			}
			_, err = fmt.Fprintf(cfg.Out, "task %s: %s%s\n", res.TaskID, res.State, note)
			return err
		},
	}
	rf.register(cmd.Flags())
	f := cmd.Flags()
	f.StringVar(&status, "status", "", "Task state: working, input-required, completed, failed, rejected")
	f.StringVar(&text, "text", "", "Status message, or the result for completed (with --status)")
	f.StringVar(&final, "final", "", "Complete the task with this result")
	f.StringVar(&inputRequired, "input-required", "", "Ask the caller this question")
	f.StringVar(&fail, "fail", "", "Fail the task with this reason")
	f.StringArrayVar(&artifacts, "artifact", nil, "Attach a file as an artifact (repeatable)")
	f.StringVar(&key, "key", "", "Idempotency key (default: derived from the task and the event content)")
	return cmd
}
