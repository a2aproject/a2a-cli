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
)

func newContactListCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List the contact book",
		Long:    "List the contact book. Agent names come from the card cache; no requests are made.",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			book, err := cfg.loadContacts()
			if err != nil {
				return err
			}
			cache := cfg.cardCache()
			views := []contactView{}
			for _, c := range book.List() {
				v := contactView{Contact: c}
				if e, ok := cache.Get(c.Card); ok && e.Found() {
					v.Agent = e.Card.Name
				}
				views = append(views, v)
			}

			if cfg.IsJSON() {
				return cfg.PrintJSON(views)
			}
			if len(views) == 0 {
				_, err := fmt.Fprintf(cfg.Out, "No contacts in %s.\nAdd one with: a2a contact add <name> <card-ref>\n", book.Path())
				return err
			}
			rows := make([][]string, 0, len(views))
			for _, v := range views {
				rows = append(rows, []string{v.Name, dashIfEmpty(v.Agent), v.Card, dashIfEmpty(v.Note)})
			}
			return cfg.PrintTable([]string{"NAME", "AGENT", "CARD", "NOTE"}, rows)
		},
	}
}
