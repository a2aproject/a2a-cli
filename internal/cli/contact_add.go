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
	"time"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-cli/internal/contacts"
	"github.com/a2aproject/a2a-cli/internal/flagparse"
)

func newContactAddCmd(cfg *globalConfig) *cobra.Command {
	var (
		note    string
		replace bool
	)
	cmd := &cobra.Command{
		Use:   "add <name> <card-ref>",
		Short: "Add a trusted agent to the contact book",
		Long: "Fetch the agent card at <card-ref> (a host/origin, a full card URL, or a local file path)\n" +
			"and save the agent under <name>. Names use lowercase letters, digits, '-' and '_'.",
		Example: "  a2a contact add nimbus nimbus.coffee --note \"Order status for my coffee subscription\"",
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := contacts.ValidateName(name); err != nil {
				return clierr.Usage(err.Error())
			}
			var ref flagparse.URLOrPath
			if err := ref.Set(args[1]); err != nil {
				return err
			}
			if err := ref.Validate(); err != nil {
				return clierr.Usage(err.Error())
			}

			book, err := cfg.loadContacts()
			if err != nil {
				return err
			}
			if _, exists := book.Get(name); exists && !replace {
				return clierr.Usage(fmt.Sprintf("contact %q already exists; pass --replace to overwrite it", name))
			}

			card, _, err := fetchContactCard(cmd.Context(), cfg, ref.URL())
			if err != nil {
				return err
			}

			c := contacts.Contact{Name: name, Card: ref.URL(), Note: note, AddedAt: time.Now().UTC().Truncate(time.Second)}
			if err := book.Add(c, replace); err != nil {
				return err
			}
			if err := book.Save(); err != nil {
				return err
			}

			if cfg.IsJSON() {
				return cfg.PrintJSON(contactView{Contact: c, Agent: card.Name})
			}
			_, err = fmt.Fprintf(cfg.Out, "Added contact %s: %s (%s)\n", name, card.Name, c.Card)
			return err
		},
	}
	cmd.Flags().StringVar(&note, "note", "", "Note about the agent, shown to your coding agent with the contact")
	cmd.Flags().BoolVar(&replace, "replace", false, "Overwrite an existing contact with the same name")
	return cmd
}
