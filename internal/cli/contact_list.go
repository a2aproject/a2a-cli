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
		Long:    "List the contact book. No requests are made.",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			book, err := cfg.loadContacts()
			if err != nil {
				return err
			}
			views := []contactView{}
			for _, c := range book.List() {
				views = append(views, newContactView(c))
			}
			if cfg.IsJSON() {
				return cfg.PrintJSON(views)
			}
			if len(views) == 0 {
				_, err := fmt.Fprintf(cfg.Out, "No contacts in %s.\nAdd one with: a2a contact add <name> <ref>\n", book.Path())
				return err
			}
			rows := make([][]string, 0, len(views))
			for _, v := range views {
				rows = append(rows, []string{v.Name, dashIfEmpty(v.DisplayName), v.Identifier, dashIfEmpty(truncate(v.Note, 60))})
			}
			return cfg.PrintTable([]string{"NAME", "AGENT", "IDENTIFIER", "NOTE"}, rows)
		},
	}
}
