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

	"github.com/a2aproject/a2a-cli/internal/contacts"
	"github.com/a2aproject/a2a-go/v2/a2a"
)

// contactDetailView is the JSON view of a contact with its card.
type contactDetailView struct {
	Contact   contacts.Contact `json:"contact"`
	Card      *a2a.AgentCard   `json:"card"`
	FetchedAt time.Time        `json:"fetchedAt"`
}

func newContactGetCmd(cfg *globalConfig) *cobra.Command {
	var refresh bool
	cmd := &cobra.Command{
		Use:   "get <name>",
		Short: "Show a contact and its agent card",
		Long:  "Show a contact and its agent card. The card is read from the local cache, and fetched when it isn't cached or --refresh is set.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			book, err := cfg.loadContacts()
			if err != nil {
				return err
			}
			c, err := lookupContact(book, args[0])
			if err != nil {
				return err
			}

			var card *a2a.AgentCard
			var fetchedAt time.Time
			if e, ok := cfg.cardCache().Get(c.Card); ok && e.Found() && !refresh {
				card, fetchedAt = e.Card, e.FetchedAt
			} else if card, fetchedAt, err = fetchContactCard(cmd.Context(), cfg, c.Card); err != nil {
				return err
			}

			if cfg.IsJSON() {
				return cfg.PrintJSON(contactDetailView{Contact: c, Card: card, FetchedAt: fetchedAt})
			}
			rows := [][]string{
				{"Contact:", c.Name},
				{"Card:", c.Card},
				{"Note:", dashIfEmpty(c.Note)},
				{"Added:", c.AddedAt.Format(time.RFC3339)},
				{"Fetched:", fetchedAt.UTC().Format(time.RFC3339)},
			}
			for _, row := range rows {
				if _, err := fmt.Fprintf(cfg.Out, "%-9s %s\n", row[0], row[1]); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintln(cfg.Out); err != nil {
				return err
			}
			return cfg.PrintCard(card)
		},
	}
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Fetch the agent card even when it is cached")
	return cmd
}
