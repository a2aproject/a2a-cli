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
)

func newServerContactListCmd(cfg *globalConfig, sf *serverFlags) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List contacts",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := sf.control()
			if err != nil {
				return err
			}
			cs, err := c.Contacts(cmd.Context())
			if err != nil {
				return err
			}
			if cfg.IsJSON() {
				return cfg.PrintJSON(cs)
			}
			rows := make([][]string, 0, len(cs))
			for _, x := range cs {
				auth := "network identity"
				if x.HasToken {
					auth = "token"
				}
				rows = append(rows, []string{x.Name, x.URL, x.Via, auth})
			}
			return cfg.PrintTable([]string{"NAME", "URL", "VIA", "AUTH"}, rows)
		},
	}
}
