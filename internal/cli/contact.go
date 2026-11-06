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

	"github.com/a2aproject/a2a-cli/internal/cardcache"
	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-cli/internal/contacts"
	"github.com/a2aproject/a2a-go/v2/a2a"
)

func newContactCmd(cfg *globalConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "contact",
		Aliases: []string{"contacts"},
		Short:   "Manage the contact book of trusted agents",
		Long: "Manage the contact book: named A2A agents you trust, stored in ~/.config/a2a-cli/contacts.yaml.\n\n" +
			"A contact name can be passed to --agent-card (-a) in place of a card reference, and the\n" +
			"a2a agent plugin hooks tell your coding agent about your contacts at the start of a session.",
	}
	cmd.AddCommand(
		newContactAddCmd(cfg),
		newContactRmCmd(cfg),
		newContactGetCmd(cfg),
		newContactListCmd(cfg),
	)
	return cmd
}

// contactView is the JSON view of a contact.
type contactView struct {
	contacts.Contact
	Agent string `json:"agent,omitempty"`
}

func (g *globalConfig) loadContacts() (*contacts.Book, error) {
	path := g.contactsFile
	if path == "" {
		var err error
		if path, err = contacts.DefaultPath(); err != nil {
			return nil, err
		}
	}
	return contacts.Load(path)
}

func (g *globalConfig) cacheRoot() (string, error) {
	if g.cacheDir != "" {
		return g.cacheDir, nil
	}
	return cardcache.DefaultDir()
}

// cardCache returns the on-disk card cache, or nil when no cache directory is available.
func (g *globalConfig) cardCache() *cardcache.Cache {
	dir, err := g.cacheRoot()
	if err != nil {
		g.logf("card cache disabled: %v", err)
		return nil
	}
	return cardcache.New(dir)
}

// lookupContact returns the named contact or a usage error naming the contact book.
func lookupContact(book *contacts.Book, name string) (contacts.Contact, error) {
	c, ok := book.Get(name)
	if !ok {
		return contacts.Contact{}, clierr.Usage(fmt.Sprintf("no contact named %q in %s", name, book.Path()))
	}
	return c, nil
}

// fetchContactCard resolves the card at ref and stores it in the card cache.
func fetchContactCard(ctx context.Context, cfg *globalConfig, ref string) (*a2a.AgentCard, time.Time, error) {
	ctx, cancel := context.WithTimeout(ctx, cfg.timeout)
	defer cancel()
	card, err := resolveCard(withServiceParams(ctx, cfg), cfg, ref)
	if err != nil {
		return nil, time.Time{}, err
	}
	fetchedAt := time.Now()
	if err := cfg.cardCache().Put(cardcache.Entry{Ref: ref, FetchedAt: fetchedAt, Card: card}); err != nil {
		cfg.logf("caching card for %s: %v", ref, err)
	}
	return card, fetchedAt, nil
}

// resolveAgentCardRef returns the URL of the --agent-card reference, looking a
// contact name up in the contact book.
func (g *globalConfig) resolveAgentCardRef() (string, error) {
	raw := g.agentCard.String()
	if contacts.IsName(raw) {
		book, err := g.loadContacts()
		if err != nil {
			return "", err
		}
		if c, ok := book.Get(raw); ok {
			g.logf("using contact %q: %s", raw, c.Card)
			return c.Card, nil
		}
	}
	if err := g.agentCard.Validate(); err != nil {
		return "", clierr.Usage(err.Error())
	}
	return g.agentCard.URL(), nil
}
