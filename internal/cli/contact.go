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
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/cardcache"
	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-cli/internal/contacts"
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2acompat/a2av0"
)

// contactsCatalogName is the name the contact book is registered under in the
// shared AI Catalog store.
const contactsCatalogName = "a2a-contacts"

func newContactCmd(cfg *globalConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "contact",
		Aliases: []string{"contacts"},
		Short:   "Manage the contact book of trusted agents",
		Long: "Manage the contact book: named A2A agents you trust.\n\n" +
			"The contact book is an AI Catalog at ~/.config/a2a-cli/catalog.json that only a2a writes. It is " +
			"registered in the AI Catalog store shared with ai-catalog-cli as \"" + contactsCatalogName + "\", " +
			"so other tools see your contacts too.\n\n" +
			"A contact name can be passed to --agent-card (-a) in place of a card reference, and a contact's " +
			"identifier takes precedence over the same identifier in other catalogs. The a2a agent plugin hooks " +
			"tell your coding agent about your contacts at the start of a session.",
	}
	cmd.AddCommand(
		newContactAddCmd(cfg),
		newContactRemoveCmd(cfg),
		newContactGetCmd(cfg),
		newContactListCmd(cfg),
	)
	return cmd
}

// contactView is the JSON view of a contact.
type contactView struct {
	Name        string    `json:"name"`
	Identifier  string    `json:"identifier"`
	DisplayName string    `json:"displayName,omitempty"`
	Description string    `json:"description,omitempty"`
	URL         string    `json:"url,omitempty"`
	Note        string    `json:"note,omitempty"`
	Source      string    `json:"source,omitempty"`
	AddedAt     time.Time `json:"addedAt"`
}

func newContactView(c contacts.Contact) contactView {
	return contactView{
		Name:        c.Name,
		Identifier:  c.Entry.Identifier,
		DisplayName: c.Entry.DisplayName,
		Description: c.Entry.Description,
		URL:         c.CardURL(),
		Note:        c.Note,
		Source:      c.Source,
		AddedAt:     c.AddedAt,
	}
}

func (g *globalConfig) contactsPath() (string, error) {
	if g.contactsFile != "" {
		return g.contactsFile, nil
	}
	return contacts.DefaultPath()
}

func (g *globalConfig) loadContacts() (*contacts.Book, error) {
	path, err := g.contactsPath()
	if err != nil {
		return nil, err
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

func (g *globalConfig) cacheCard(cardURL string, card *a2a.AgentCard, fetchedAt time.Time) {
	if cardURL == "" {
		return
	}
	if err := g.cardCache().Put(cardcache.Entry{Ref: cardURL, FetchedAt: fetchedAt, Card: card}); err != nil {
		g.logf("caching card for %s: %v", cardURL, err)
	}
}

// cachedContactCard returns the card a contact embeds or the one cached for its
// card URL, without making requests. The time is zero for an embedded card.
func (g *globalConfig) cachedContactCard(cache *cardcache.Cache, c contacts.Contact) (*a2a.AgentCard, time.Time, bool) {
	if len(c.Entry.Data) > 0 {
		card, err := a2av0.NewAgentCardParser()(c.Entry.Data)
		if err != nil {
			g.logf("contact %q: parsing embedded card: %v", c.Name, err)
			return nil, time.Time{}, false
		}
		return card, time.Time{}, true
	}
	e, ok := cache.Get(c.CardURL())
	if !ok || !e.Found() {
		return nil, time.Time{}, false
	}
	return e.Card, e.FetchedAt, true
}

func lookupContact(book *contacts.Book, name string) (contacts.Contact, error) {
	c, ok := book.Get(name)
	if !ok {
		return contacts.Contact{}, contactError(&contacts.NotFoundError{Name: name, Path: book.Path()})
	}
	return c, nil
}

// contactError classifies a contact book error.
func contactError(err error) error {
	if _, ok := errors.AsType[*contacts.NotFoundError](err); ok {
		return &clierr.Error{Code: clierr.CodeUsage, Message: err.Error(), Hint: "run `a2a contact list` to see your contacts", Exit: 2}
	}
	if exists, ok := errors.AsType[*contacts.ExistsError](err); ok {
		hint := "pass --replace to overwrite it"
		if exists.SameAgent {
			hint = fmt.Sprintf("remove it with `a2a contact remove %s` first, or address it by that name", exists.Name)
		}
		return &clierr.Error{Code: clierr.CodeUsage, Message: err.Error(), Hint: hint, Exit: 2}
	}
	return err
}

// saveContacts writes the contact book and registers it in the shared AI Catalog
// store. The contact book is the source of truth, so a failure to register it
// is a warning.
func saveContacts(ctx context.Context, cfg *globalConfig, book *contacts.Book) error {
	if err := book.Save(); err != nil {
		return err
	}
	if err := registerContacts(ctx, cfg, book); err != nil {
		_, werr := fmt.Fprintf(cfg.stderr(), "warning: contacts saved, but not registered in the AI Catalog store: %v\n", err)
		return werr
	}
	return nil
}

func registerContacts(ctx context.Context, cfg *globalConfig, book *contacts.Book) error {
	reg, err := cfg.openCatalogs()
	if err != nil {
		return err
	}
	if reg.Problem != nil {
		return nil
	}
	if c, ok := reg.Lookup(book.URL()); ok {
		_, err = reg.Update(ctx, cfg.catalogFetcher(), c.Name)
		return err
	}
	_, err = reg.Add(ctx, cfg.catalogFetcher(), contactsCatalogName, book.Path())
	return err
}
