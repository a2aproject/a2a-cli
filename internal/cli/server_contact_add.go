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

	"github.com/a2aproject/a2a-cli/internal/agentsrv"
	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-cli/internal/relayapi"
)

func newServerContactAddCmd(cfg *globalConfig, sf *serverFlags) *cobra.Command {
	var req agentsrv.ContactRequest
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Let the agent call another agent",
		Long: `Let the agent call another agent with the a2a CLI. Without --url the
agent is looked up by name on the private network, where it recognizes this
agent by its network identity. With --url, pair with a code from the other
agent's owner, or pass a token.`,
		Example: "  a2a server contact add coder\n  a2a server contact add grok --url https://grok.example.com --code K7Q2-9XF4",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := sf.control()
			if err != nil {
				return err
			}
			req.Name = args[0]
			if req.Token, err = relayapi.Secret(req.Token); err != nil {
				return clierr.Usage("--token: " + err.Error())
			}
			info, err := c.AddContact(cmd.Context(), req)
			if err != nil {
				return err
			}
			if cfg.IsJSON() {
				return cfg.PrintJSON(info)
			}
			status, err := c.Status(cmd.Context())
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cfg.Out, "✓ %s can call %s (%s, via %s)\n", status.Name, info.Name, info.URL, info.Via)
			return err
		},
	}
	cmd.Flags().StringVar(&req.URL, "url", "", "The agent's URL")
	cmd.Flags().StringVar(&req.Code, "code", "", "A pairing code from the other agent's owner")
	cmd.Flags().StringVar(&req.Token, "token", "", "A token for the other agent: env:VAR or the token")
	return cmd
}
