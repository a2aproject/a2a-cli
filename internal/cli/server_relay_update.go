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

	"github.com/a2aproject/a2a-cli/internal/relay"
)

func newServerRelayUpdateCmd(cfg *globalConfig) *cobra.Command {
	var (
		rf      relayClientFlags
		webhook string
		headers []string
		name    string
		desc    string
		public  bool
	)
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Change the webhook, name, description or access of this node",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var req relay.UpdateNodeRequest
			f := cmd.Flags()
			if f.Changed("webhook") {
				req.Webhook = &webhook
			}
			if f.Changed("name") {
				req.Name = &name
			}
			if f.Changed("description") {
				req.Description = &desc
			}
			if f.Changed("public") {
				req.Public = &public
			}
			hdrs, err := parseHeaders(headers)
			if err != nil {
				return err
			}
			req.WebhookHeaders = hdrs

			ctx, cancel := relayContext(cmd, cfg)
			defer cancel()
			c, err := rf.nodeClient()
			if err != nil {
				return err
			}
			n, err := c.UpdateNode(ctx, req)
			if err != nil {
				return err
			}
			return printRelayNode(cfg, n)
		},
	}
	rf.register(cmd.Flags())
	f := cmd.Flags()
	f.StringVar(&webhook, "webhook", "", "New webhook URL")
	f.StringArrayVar(&headers, "webhook-header", nil, `Header sent with every wake, "Name: value" (repeatable; replaces all headers)`)
	f.StringVar(&name, "name", "", "Agent name")
	f.StringVar(&desc, "description", "", "Agent description")
	f.BoolVar(&public, "public", false, "Let anyone call the agent (--public=false to require grants)")
	return cmd
}
