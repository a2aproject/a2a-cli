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
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/agentsrv"
)

func newServerPairListCmd(cfg *globalConfig, sf *serverFlags) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List the callers and webhook nodes paired with the agent",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := sf.control()
			if err != nil {
				return err
			}
			toks, err := c.Tokens(cmd.Context())
			if err != nil {
				return err
			}
			nodes, err := c.Nodes(cmd.Context())
			if err != nil {
				return err
			}
			if cfg.IsJSON() {
				return cfg.PrintJSON(struct {
					Callers []agentsrv.TokenInfo `json:"callers"`
					Nodes   []agentsrv.NodeInfo  `json:"nodes"`
				}{toks, nodes})
			}
			rows := make([][]string, 0, len(toks)+len(nodes))
			for _, t := range toks {
				used := "never"
				if !t.LastUsed.IsZero() {
					used = t.LastUsed.Format(time.DateTime)
				}
				rows = append(rows, []string{t.Name, agentsrv.PairKindCaller, t.Principal, strings.Join(t.Roles, ","), t.Created.Format(time.DateTime), used})
			}
			for _, n := range nodes {
				used := "-"
				if !n.Paired {
					used = "not paired"
				}
				rows = append(rows, []string{n.ID, agentsrv.PairKindNode, "node:" + n.ID, "", n.Created.Format(time.DateTime), used})
			}
			return cfg.PrintTable([]string{"NAME", "KIND", "PRINCIPAL", "ROLES", "CREATED", "LAST USED"}, rows)
		},
	}
}
