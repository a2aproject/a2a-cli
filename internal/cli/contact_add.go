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
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-cli/internal/contacts"
)

func newContactAddCmd(cfg *globalConfig) *cobra.Command {
	var (
		note    string
		replace bool
	)
	cmd := &cobra.Command{
		Use:   "add <name> <ref>",
		Short: "Add a trusted agent to the contact book",
		Long: "Resolve <ref> like --agent-card and save the agent under <name>. <ref> may be a host, a card URL, " +
			"a local file, the identifier of an agent in a registered AI Catalog, or, with --catalog, the identifier " +
			"of an agent in that catalog.\n\n" +
			"An agent from a catalog is copied with its identifier, publisher and trust manifest. An agent added by " +
			"its card gets the identifier " + contacts.IdentifierPrefix + "<name>. The card is fetched first, so an " +
			"unreachable agent is not saved.\n\n" +
			"Names use lowercase letters, digits, '-' and '_'.",
		Example: "  a2a contact add weather https://weather.example.com\n" +
			"  a2a contact add fin urn:air:acme.com:agent:finance --note \"Expense reports\"\n" +
			"  a2a contact add fin --catalog https://acme.com/ai-catalog.json urn:air:acme.com:agent:finance",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := contacts.ValidateName(name); err != nil {
				return clierr.Usage(err.Error())
			}
			book, err := cfg.loadContacts()
			if err != nil {
				return err
			}
			if _, exists := book.Get(name); exists && !replace {
				return contactError(&contacts.ExistsError{Name: name})
			}
			if err := cfg.agentCard.Set(args[1]); err != nil {
				return err
			}

			ctx, cancel := context.WithTimeout(withServiceParams(cmd.Context(), cfg), cfg.timeout)
			defer cancel()
			agent, err := resolveAgent(ctx, cfg)
			if err != nil {
				return err
			}
			now := time.Now().UTC().Truncate(time.Second)
			var c contacts.Contact
			if agent.entry != nil {
				c, err = contacts.FromEntry(name, note, *agent.entry, now)
			} else {
				c, err = contacts.FromCard(name, note, agent.cardURL, agent.card, now)
			}
			if err != nil {
				return err
			}
			if err := book.Add(c, replace); err != nil {
				return contactError(err)
			}
			cfg.cacheCard(agent.cardURL, agent.card, now)
			if err := saveContacts(cmd.Context(), cfg, book); err != nil {
				return err
			}

			if cfg.IsJSON() {
				return cfg.PrintJSON(newContactView(c))
			}
			_, err = fmt.Fprintf(cfg.Out, "Added contact %s: %s (%s)\n", name, agent.card.Name, c.Entry.Identifier)
			return err
		},
	}
	cmd.Flags().StringVar(&note, "note", "", "Note about the agent, shown to your coding agent with the contact")
	cmd.Flags().BoolVar(&replace, "replace", false, "Overwrite an existing contact with the same name")
	return cmd
}
