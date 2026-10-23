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
	"strconv"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/relayapi"
)

func newRelayInboxCmd(cfg *globalConfig) *cobra.Command {
	var (
		rf  relayClientFlags
		all bool
	)
	cmd := &cobra.Command{
		Use:   "inbox",
		Short: "Show the node, and wakes not delivered yet or tasks still waiting for a reply",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := relayContext(cmd, cfg)
			defer cancel()
			c, err := rf.nodeClient(cfg)
			if err != nil {
				return err
			}
			node, err := c.Node(ctx)
			if err != nil {
				return err
			}
			inbox, err := c.Inbox(ctx, all)
			if err != nil {
				return err
			}
			if cfg.IsJSON() {
				return cfg.PrintJSON(struct {
					Node    *relayapi.NodeInfo    `json:"node"`
					Entries []relayapi.InboxEntry `json:"entries"`
				}{node, inbox.Entries})
			}
			if _, err := fmt.Fprintf(cfg.Out, "%s (node %s)\n  A2A URL: %s\n  Webhook: %s\n\n", node.Name, node.ID, node.URL, node.Webhook); err != nil {
				return err
			}
			rows := make([][]string, 0, len(inbox.Entries))
			for _, e := range inbox.Entries {
				rows = append(rows, []string{e.Envelope.CreatedAt.Format("15:04:05"), e.Envelope.Kind, e.Envelope.TaskID,
					e.Envelope.Sender.String(), strconv.FormatBool(e.Delivered), strconv.FormatBool(e.Open), oneLine(e.Envelope.Text, 60)})
			}
			return cfg.PrintTable([]string{"TIME", "KIND", "TASK", "SENDER", "DELIVERED", "OPEN", "TEXT"}, rows)
		},
	}
	rf.register(cmd.Flags())
	cmd.Flags().BoolVar(&all, "all", false, "List the last 200 wakes")
	return cmd
}
