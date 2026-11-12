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

func newContactRemoveCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:     "remove <name>...",
		Aliases: []string{"rm"},
		Short:   "Remove agents from the contact book",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			book, err := cfg.loadContacts()
			if err != nil {
				return err
			}
			for _, name := range args {
				if _, err := lookupContact(book, name); err != nil {
					return err
				}
			}
			for _, name := range args {
				if err := book.Remove(name); err != nil {
					return contactError(err)
				}
			}
			if err := saveContacts(cmd.Context(), cfg, book); err != nil {
				return err
			}

			if cfg.IsJSON() {
				return cfg.PrintJSON(map[string]any{"removed": args})
			}
			for _, name := range args {
				if _, err := fmt.Fprintf(cfg.Out, "Removed contact %s\n", name); err != nil {
					return err
				}
			}
			return nil
		},
	}
}
