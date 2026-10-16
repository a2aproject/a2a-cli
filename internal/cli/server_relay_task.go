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
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/relay"
	"github.com/a2aproject/a2a-go/v2/a2a"
)

func newServerRelayTaskCmd(cfg *globalConfig) *cobra.Command {
	var rf relayClientFlags
	cmd := &cobra.Command{
		Use:   "task <task-id>",
		Short: "Show a task addressed to this node, with history and artifacts",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := relayContext(cmd, cfg)
			defer cancel()
			c, err := rf.nodeClient()
			if err != nil {
				return err
			}
			t, err := c.Task(ctx, args[0])
			if err != nil {
				return err
			}
			if cfg.IsJSON() {
				return cfg.PrintJSON(t)
			}
			return printRelayTask(cfg.Out, t)
		},
	}
	rf.register(cmd.Flags())
	return cmd
}

func printRelayTask(w io.Writer, t *a2a.Task) error {
	var b strings.Builder
	fmt.Fprintf(&b, "Task %s (context %s): %s\n", t.ID, t.ContextID, relay.StateName(t.Status.State))
	if len(t.History) > 0 {
		b.WriteString("History:\n")
		for _, m := range t.History {
			role := "caller"
			if m.Role == a2a.MessageRoleAgent {
				role = "agent"
			}
			fmt.Fprintf(&b, "  %s: %s\n", role, oneLine(relay.TextOf(m), 200))
		}
	}
	if msg := relay.TextOf(t.Status.Message); msg != "" {
		fmt.Fprintf(&b, "Status message: %s\n", oneLine(msg, 200))
	}
	for _, a := range t.Artifacts {
		var desc []string
		for _, p := range a.Parts {
			switch {
			case p.Text() != "":
				desc = append(desc, oneLine(p.Text(), 200))
			case p.Raw() != nil:
				desc = append(desc, fmt.Sprintf("[%s, %d bytes]", p.MediaType, len(p.Raw())))
			default:
				desc = append(desc, "[data]")
			}
		}
		fmt.Fprintf(&b, "Artifact %s: %s\n", a.Name, strings.Join(desc, " "))
	}
	_, err := io.WriteString(w, b.String())
	return err
}
