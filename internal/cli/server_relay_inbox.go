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
)

func newServerRelayInboxCmd(cfg *globalConfig) *cobra.Command {
	var (
		rf  relayClientFlags
		all bool
	)
	cmd := &cobra.Command{
		Use:   "inbox",
		Short: "List wakes not delivered yet or still waiting for a reply",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := relayContext(cmd, cfg)
			defer cancel()
			c, err := rf.nodeClient()
			if err != nil {
				return err
			}
			inbox, err := c.Inbox(ctx, all)
			if err != nil {
				return err
			}
			if cfg.IsJSON() {
				return cfg.PrintJSON(inbox)
			}
			return printRelayInbox(cfg.Out, inbox)
		},
	}
	rf.register(cmd.Flags())
	cmd.Flags().BoolVar(&all, "all", false, "Show all recent wakes, not only pending ones")
	return cmd
}

func printRelayInbox(w io.Writer, inbox *relay.Inbox) error {
	if len(inbox.Entries) == 0 {
		_, err := fmt.Fprintln(w, "Nothing pending.")
		return err
	}
	var b strings.Builder
	for _, e := range inbox.Entries {
		env := e.Envelope
		var flags []string
		if e.Open {
			flags = append(flags, "waiting for reply")
		}
		if !e.Delivered {
			flags = append(flags, "not delivered: "+e.LastError)
		}
		who := env.Sender.String()
		if env.Kind == relay.KindResult {
			who = env.Peer
		}
		fmt.Fprintf(&b, "%s  %-8s %s  %s  %s", env.CreatedAt.Local().Format("Jan 02 15:04"), env.Kind, env.TaskID, who, env.State)
		if len(flags) > 0 {
			fmt.Fprintf(&b, "  [%s]", strings.Join(flags, "; "))
		}
		b.WriteString("\n")
		if env.Text != "" {
			fmt.Fprintf(&b, "    %s\n", oneLine(env.Text, 160))
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}
