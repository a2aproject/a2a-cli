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
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2acompat/a2av0"
)

const (
	// maxListedAgents caps the agents named in a hint.
	maxListedAgents = 10
	// allCatalogsScope describes the scope of a lookup without --catalog.
	allCatalogsScope = "the registered catalogs"
)

// resolveAgentCard resolves --agent-card. An entry identifier, or any reference
// combined with --catalog, is looked up in AI Catalogs. A reference that serves
// an AI Catalog resolves to the catalog's only A2A agent.
func resolveAgentCard(ctx context.Context, cfg *globalConfig) (*a2a.AgentCard, error) {
	ref := cfg.agentCard.String()
	if cfg.catalog != "" || aicatalog.IsIdentifier(ref) {
		return resolveCatalogAgent(ctx, cfg, cfg.catalog, ref)
	}
	if err := cfg.agentCard.Validate(); err != nil {
		return nil, clierr.Usage(err.Error())
	}
	cardURL := cfg.agentCard.URL()
	card, raw, err := fetchCardDocument(ctx, cfg, cardURL, true)
	if err != nil || card != nil {
		return card, err
	}

	cfg.logf("%s is an AI Catalog, looking for its A2A agent", cardURL)
	base := cardRequestURL(cardURL)
	entries, warnings, err := aicatalog.Load(ctx, cfg.catalogFetcher(), base, raw)
	cfg.logWarnings(warnings)
	if err != nil {
		return nil, catalogError(err)
	}
	entry, err := aicatalog.SingleAgent(entries, cfg.agentCard.String())
	if err != nil {
		return nil, catalogError(err)
	}
	return cardFromEntry(ctx, cfg, entry)
}

func resolveCatalogAgent(ctx context.Context, cfg *globalConfig, scope, id string) (*a2a.AgentCard, error) {
	entries, label, err := catalogEntries(ctx, cfg, scope)
	if err != nil {
		return nil, err
	}
	entry, err := aicatalog.FindAgent(entries, id, label)
	if err != nil {
		return nil, catalogError(err)
	}
	cfg.logf("found %s in %s", id, entry.Catalog)
	return cardFromEntry(ctx, cfg, entry)
}

// catalogEntries returns the entries of the catalog scope, a registered catalog
// name or source or a catalog URL or path, or of every registered catalog when
// scope is empty. It also returns a description of the scope for messages.
func catalogEntries(ctx context.Context, cfg *globalConfig, scope string) ([]aicatalog.Entry, string, error) {
	reg, err := cfg.openCatalogs()
	if err != nil {
		return nil, "", err
	}
	if scope == "" {
		entries, warnings, err := reg.Entries(ctx, "")
		cfg.logWarnings(warnings)
		return entries, allCatalogsScope, err
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

// cardFromEntry returns the Agent Card a catalog entry embeds or points to.
func cardFromEntry(ctx context.Context, cfg *globalConfig, entry aicatalog.Entry) (*a2a.AgentCard, error) {
	if len(entry.Data) > 0 {
		card, err := a2av0.NewAgentCardParser()(entry.Data)
		if err != nil {
			return nil, clierr.CardResolution(fmt.Errorf("card parsing failed: %w", err))
		}
		return card, nil
	}
	cardURL, err := aicatalog.CardURL(entry)
	if err != nil {
		return nil, clierr.CardResolution(err)
	}
	card, _, err := fetchCardDocument(ctx, cfg, cardURL, false)
	return card, err
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
