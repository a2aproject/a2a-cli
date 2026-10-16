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
	"strings"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/relay"
)

func newServerRelaySendCmd(cfg *globalConfig) *cobra.Command {
	var (
		rf        relayClientFlags
		taskID    string
		contextID string
		peerToken string
	)
	cmd := &cobra.Command{
		Use:   "send <agent-url | node-id> <text>",
		Short: "Message another agent; the result comes back as a wake",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := relayContext(cmd, cfg)
			defer cancel()
			c, err := rf.nodeClient()
			if err != nil {
				return err
			}
			res, err := c.Send(ctx, relay.SendRequest{
				Peer: args[0], Text: strings.Join(args[1:], " "), TaskID: taskID, ContextID: contextID, Token: relay.Secret(peerToken),
			})
			if err != nil {
				return err
			}
			if cfg.IsJSON() {
				return cfg.PrintJSON(res)
			}
			switch {
			case res.Message != nil:
				_, err = fmt.Fprintf(cfg.Out, "%s answered: %s\n", res.Peer, relay.TextOf(res.Message))
			case res.TaskID != "":
				_, err = fmt.Fprintf(cfg.Out, "Sent to %s: task %s (%s, context %s). The result arrives as a result wake.\n", res.Peer, res.TaskID, res.State, res.ContextID)
			default:
				_, err = fmt.Fprintf(cfg.Out, "Sent to %s. The result arrives as a result wake.\n", res.Peer)
			}
			return err
		},
	}
	rf.register(cmd.Flags())
	f := cmd.Flags()
	f.StringVar(&taskID, "task", "", "Continue this peer task, e.g. to answer input-required")
	f.StringVar(&contextID, "context", "", "Context ID for the conversation")
	f.StringVar(&peerToken, "peer-token", "", "Bearer token for the peer, such as a grant it issued you; may be env:VAR")
	return cmd
}
