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
	"time"

	"github.com/spf13/cobra"
)

func newPairListCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List saved agent tokens",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := cfg.credentialStore()
			if err != nil {
				return err
			}
			creds, err := store.List()
			if err != nil {
				return err
			}
			rows := make([][]string, 0, len(creds))
			out := make([]map[string]string, 0, len(creds))
			for _, c := range creds {
				rows = append(rows, []string{c.URL, c.Agent, c.Principal, c.Created.Format(time.DateTime)})
				out = append(out, map[string]string{"url": c.URL, "agent": c.Agent, "principal": c.Principal})
			}
			if cfg.IsJSON() {
				return cfg.PrintJSON(out)
			}
			return cfg.PrintTable([]string{"URL", "AGENT", "PRINCIPAL", "CREATED"}, rows)
		},
	}
}
