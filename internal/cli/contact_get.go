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

	"github.com/a2aproject/a2a-go/v2/a2a"
)

// contactDetailView is the JSON view of a contact with its card.
type contactDetailView struct {
	Contact contactView    `json:"contact"`
	Card    *a2a.AgentCard `json:"card"`
	// FetchedAt is when the card was fetched, or nil for a card embedded in the contact.
	FetchedAt *time.Time `json:"fetchedAt,omitempty"`
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

			card, fetchedAt, ok := cfg.cachedContactCard(cfg.cardCache(), c)
			if !ok || (refresh && c.CardURL() != "") {
				ctx, cancel := context.WithTimeout(withServiceParams(cmd.Context(), cfg), cfg.timeout)
				defer cancel()
				agent, err := agentFromEntry(ctx, cfg, c.Entry)
				if err != nil {
					return err
				}
				card, fetchedAt = agent.card, time.Now().UTC().Truncate(time.Second)
				cfg.cacheCard(agent.cardURL, card, fetchedAt)
			}

			view := contactDetailView{Contact: newContactView(c), Card: card}
			if !fetchedAt.IsZero() {
				view.FetchedAt = &fetchedAt
			}
			if cfg.IsJSON() {
				return cfg.PrintJSON(view)
			}
			fetched := "embedded in the contact"
			if view.FetchedAt != nil {
				fetched = fetchedAt.UTC().Format(time.RFC3339)
			}
			rows := [][]string{
				{"Contact:", c.Name},
				{"Identifier:", c.Entry.Identifier},
				{"Card:", dashIfEmpty(c.CardURL())},
				{"Source:", dashIfEmpty(c.Source)},
				{"Note:", dashIfEmpty(c.Note)},
				{"Added:", c.AddedAt.UTC().Format(time.RFC3339)},
				{"Fetched:", fetched},
			}
			for _, row := range rows {
				if _, err := fmt.Fprintf(cfg.Out, "%-12s %s\n", row[0], row[1]); err != nil {
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
