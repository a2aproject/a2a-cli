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
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"

	"github.com/a2aproject/a2a-cli/internal/aicatalog"
	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-cli/internal/contacts"
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2acompat/a2av0"
)

const (
	// maxListedAgents caps the agents named in a hint.
	maxListedAgents = 10
	// allCatalogsScope describes the scope of a lookup without --catalog.
	allCatalogsScope = "the registered catalogs"
)

// resolvedAgent is the agent --agent-card names.
type resolvedAgent struct {
	card *a2a.AgentCard
	// entry is the catalog entry that lists the agent, or nil for an agent
	// named by its card URL.
	entry *aicatalog.Entry
	// cardURL is where the card was fetched from, or "" for a card embedded
	// in its entry.
	cardURL string
}

func resolveAgentCard(ctx context.Context, cfg *globalConfig) (*a2a.AgentCard, error) {
	agent, err := resolveAgent(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return agent.card, nil
}

// resolveAgent resolves --agent-card. With --catalog the reference is an
// identifier looked up in that catalog. Otherwise a contact name or identifier
// resolves to the contact, an identifier that is not a contact is looked up in
// the registered catalogs, and anything else is a card reference. A card
// reference that serves an AI Catalog resolves to the catalog's only A2A agent.
func resolveAgent(ctx context.Context, cfg *globalConfig) (*resolvedAgent, error) {
	ref := cfg.agentCard.String()
	if cfg.catalog != "" {
		return agentFromCatalog(ctx, cfg, cfg.catalog, ref)
	}
	if contacts.IsName(ref) || aicatalog.IsIdentifier(ref) {
		book, err := cfg.loadContacts()
		if err != nil {
			return nil, err
		}
		if c, ok := book.Get(ref); ok {
			cfg.logf("using contact %q", ref)
			return agentFromEntry(ctx, cfg, c.Entry)
		}
		if aicatalog.IsIdentifier(ref) {
			if entry, err := aicatalog.FindAgent(book.Entries(), ref, contacts.CatalogLabel); err == nil {
				cfg.logf("found %s in contacts", ref)
				return agentFromEntry(ctx, cfg, entry)
			}
			return agentFromCatalog(ctx, cfg, "", ref)
		}
	}

	if err := cfg.agentCard.Validate(); err != nil {
		return nil, clierr.Usage(err.Error())
	}
	cardURL := cfg.agentCard.URL()
	card, raw, err := fetchCardDocument(ctx, cfg, cardURL, true)
	if err != nil {
		return nil, err
	}
	if card != nil {
		return &resolvedAgent{card: card, cardURL: cardRequestURL(cardURL)}, nil
	}

	cfg.logf("%s is an AI Catalog, looking for its A2A agent", cardURL)
	base := cardRequestURL(cardURL)
	entries, warnings, err := aicatalog.Load(ctx, cfg.catalogFetcher(), base, raw)
	cfg.logWarnings(warnings)
	if err != nil {
		return nil, catalogError(err)
	}
	entry, err := aicatalog.SingleAgent(entries, ref)
	if err != nil {
		return nil, catalogError(err)
	}
	return agentFromEntry(ctx, cfg, entry)
}

func agentFromCatalog(ctx context.Context, cfg *globalConfig, scope, id string) (*resolvedAgent, error) {
	entries, label, err := catalogEntries(ctx, cfg, scope, true)
	if err != nil {
		return nil, err
	}
	entry, err := aicatalog.FindAgent(entries, id, label)
	if err != nil {
		return nil, catalogError(err)
	}
	cfg.logf("found %s in %s", id, entry.Catalog)
	return agentFromEntry(ctx, cfg, entry)
}

// agentFromEntry returns the agent a catalog entry embeds or points to.
func agentFromEntry(ctx context.Context, cfg *globalConfig, entry aicatalog.Entry) (*resolvedAgent, error) {
	if len(entry.Data) > 0 {
		card, err := a2av0.NewAgentCardParser()(entry.Data)
		if err != nil {
			return nil, clierr.CardResolution(fmt.Errorf("card parsing failed: %w", err))
		}
		return &resolvedAgent{card: card, entry: &entry}, nil
	}
	cardURL, err := aicatalog.CardURL(entry)
	if err != nil {
		return nil, clierr.CardResolution(err)
	}
	card, _, err := fetchCardDocument(ctx, cfg, cardURL, false)
	if err != nil {
		return nil, err
	}
	return &resolvedAgent{card: card, entry: &entry, cardURL: cardURL}, nil
}

// catalogEntries returns the entries of the catalog scope, a registered catalog
// name or source or a catalog URL or path, or of every registered catalog when
// scope is empty. It also returns a description of the scope for messages.
// skipContacts leaves out the registered copy of the contact book from every
// registered catalog, for lookups that consult the contact book itself.
func catalogEntries(ctx context.Context, cfg *globalConfig, scope string, skipContacts bool) ([]aicatalog.Entry, string, error) {
	reg, err := cfg.openCatalogs()
	if err != nil {
		return nil, "", err
	}
	if scope == "" {
		entries, warnings, err := reg.Entries(ctx, "")
		cfg.logWarnings(warnings)
		if err != nil || !skipContacts {
			return entries, allCatalogsScope, err
		}
		path, err := cfg.contactsPath()
		if err != nil {
			return nil, "", err
		}
		if c, ok := reg.Lookup(contacts.FileURL(path)); ok {
			entries = slices.DeleteFunc(entries, func(e aicatalog.Entry) bool { return e.Catalog == c.Name })
		}
		return entries, allCatalogsScope, nil
	}
	if c, ok := reg.Lookup(scope); ok {
		entries, warnings, err := reg.Entries(ctx, c.Name)
		cfg.logWarnings(warnings)
		return entries, fmt.Sprintf("catalog %q", c.Name), err
	}
	if !isCatalogLocation(scope) {
		return nil, "", catalogError(&aicatalog.UnknownCatalogError{Name: scope})
	}

	source, err := aicatalog.NormalizeSource(scope)
	if err != nil {
		return nil, "", clierr.Usage(fmt.Sprintf("invalid --catalog: %v", err))
	}
	if err := cfg.initTLS(); err != nil {
		return nil, "", err
	}
	cfg.logf("fetching catalog %s", source)
	fetcher := cfg.catalogFetcher()
	data, err := fetcher.Fetch(ctx, source)
	if err != nil {
		return nil, "", catalogError(err)
	}
	entries, warnings, err := aicatalog.Load(ctx, fetcher, source, data)
	cfg.logWarnings(warnings)
	if err != nil {
		return nil, "", catalogError(err)
	}
	return entries, source, nil
}

// isCatalogLocation reports whether a --catalog value that names no registered
// catalog is a URL or local path rather than a mistyped name.
func isCatalogLocation(scope string) bool {
	if strings.Contains(scope, "://") {
		return true
	}
	_, err := os.Stat(scope)
	return err == nil
}

// cardRequestURL returns the URL the card resolver requests for ref, which has
// the well-known path appended when it has no path.
func cardRequestURL(ref string) string {
	u, err := url.Parse(ref)
	if err != nil || (u.Path != "" && u.Path != "/") {
		return ref
	}
	return u.JoinPath(".well-known", "agent-card.json").String()
}

func (g *globalConfig) openCatalogs() (*aicatalog.Registry, error) {
	dir := g.catalogDir
	if dir == "" {
		var err error
		if dir, err = aicatalog.DefaultDir(); err != nil {
			return nil, err
		}
	}
	reg, err := (&aicatalog.Store{Dir: dir}).Open()
	if err != nil {
		return nil, err
	}
	if reg.Problem != nil {
		if _, err := fmt.Fprintf(g.stderr(), "warning: %v; a2a will read it but not modify it\n", reg.Problem); err != nil {
			return nil, err
		}
	}
	return reg, nil
}

func (g *globalConfig) catalogFetcher() *aicatalog.Fetcher {
	return &aicatalog.Fetcher{Client: &http.Client{Transport: g.httpTransport}}
}

func (g *globalConfig) logWarnings(warnings []string) {
	for _, w := range warnings {
		g.logf("warning: %s", w)
	}
}

// catalogError classifies a catalog failure and adds remediation.
func catalogError(err error) error {
	if _, ok := errors.AsType[*aicatalog.UnknownCatalogError](err); ok {
		return &clierr.Error{
			Code:    clierr.CodeUsage,
			Message: err.Error(),
			Hint:    "run `a2a catalog list` to see registered catalogs, or pass a catalog URL",
			Exit:    2,
		}
	}
	ce := clierr.CardResolution(err)
	var (
		notFound  *aicatalog.NotFoundError
		ambiguous *aicatalog.AmbiguousError
		noSingle  *aicatalog.NoSingleAgentError
	)
	switch {
	case errors.As(err, &notFound):
		ce.Hint = "run `a2a catalog search` to find an agent's identifier"
		if len(notFound.Agents) == 0 && notFound.Scope == allCatalogsScope {
			ce.Hint = "register a catalog with `a2a catalog add <name> <url>`, or pass one with --catalog"
		} else if len(notFound.Agents) > 0 {
			ce.Hint = "A2A agents in " + notFound.Scope + ": " + listAgents(notFound.Agents)
		}
	case errors.As(err, &ambiguous):
		ce.Hint = "pick a catalog with --catalog <name>"
	case errors.As(err, &noSingle) && len(noSingle.Agents) > 0:
		ce.Hint = "A2A agents in the catalog: " + listAgents(noSingle.Agents)
	}
	return ce
}

func listAgents(agents []aicatalog.Entry) string {
	var ids []string
	for _, a := range agents {
		if !slices.Contains(ids, a.Identifier) {
			ids = append(ids, a.Identifier)
		}
	}
	list := strings.Join(ids[:min(len(ids), maxListedAgents)], ", ")
	if more := len(ids) - maxListedAgents; more > 0 {
		list += fmt.Sprintf(" and %d more", more)
	}
	return list
}

// warnAll prints warnings to stderr.
func (g *globalConfig) warnAll(warnings []string) error {
	for _, w := range warnings {
		if _, err := fmt.Fprintf(g.stderr(), "warning: %s\n", w); err != nil {
			return err
		}
	}
	return nil
}
