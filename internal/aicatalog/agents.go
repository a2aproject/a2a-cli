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

package aicatalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Agent-Card/ai-catalog-go/catalog"
)

// IsIdentifier reports whether an --agent-card value names a catalog entry
// rather than a host, URL or path: it is a URN and not an existing file.
func IsIdentifier(ref string) bool {
	if len(ref) <= len("urn:") || !strings.EqualFold(ref[:len("urn:")], "urn:") {
		return false
	}
	_, err := os.Stat(ref)
	return err != nil
}

// NormalizeSource turns a catalog URL, local path or bare host into the URL it
// is fetched from. A URL without a path and a bare host get the well-known path.
func NormalizeSource(source string) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", errors.New("catalog source must not be empty")
	}
	if !strings.Contains(source, "://") {
		if _, err := os.Stat(source); err == nil {
			abs, err := filepath.Abs(source)
			if err != nil {
				return "", err
			}
			return (&url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}).String(), nil
		}
		source = "https://" + source
	}
	u, err := url.Parse(source)
	if err != nil {
		return "", fmt.Errorf("invalid catalog source %q: %w", source, err)
	}
	switch u.Scheme {
	case "file":
		return u.String(), nil
	case "http", "https":
	default:
		return "", fmt.Errorf("unsupported catalog source %q: want an http, https or file URL, or a local path", source)
	}
	if u.Host == "" {
		return "", fmt.Errorf("invalid catalog source %q: missing host", source)
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = catalog.WellKnownPath
	}
	return u.String(), nil
}

// IsAgent reports whether the entry is an A2A Agent Card.
func IsAgent(e *catalog.CatalogEntry) bool {
	return e.Type == catalog.MediaTypeA2AAgentCard
}

// Agents returns the A2A agents among entries, each listed once per catalog.
func Agents(entries []Entry) []Entry {
	type key struct{ catalog, id, version string }
	seen := map[key]bool{}
	var agents []Entry
	for _, e := range entries {
		k := key{e.Catalog, e.Identifier, e.Version}
		if !IsAgent(&e.CatalogEntry) || seen[k] {
			continue
		}
		seen[k] = true
		agents = append(agents, e)
	}
	return agents
}

// Search returns the A2A agents whose identifier, display name, description or
// tags contain query, ignoring case. An empty query matches every agent.
func Search(entries []Entry, query string) []Entry {
	query = strings.ToLower(query)
	var found []Entry
	for _, e := range Agents(entries) {
		fields := append([]string{e.Identifier, e.DisplayName, e.Description}, e.Tags...)
		if slices.ContainsFunc(fields, func(f string) bool { return strings.Contains(strings.ToLower(f), query) }) {
			found = append(found, e)
		}
	}
	return found
}

// NotFoundError reports an identifier that names no A2A agent in Scope.
type NotFoundError struct {
	Identifier string
	Scope      string
	// Type is the media type of an entry with the identifier that is not an A2A agent.
	Type string
	// Agents are the A2A agents that are in Scope.
	Agents []Entry
}

func (e *NotFoundError) Error() string {
	if e.Type != "" {
		return fmt.Sprintf("%q in %s is a %s entry, not an A2A agent", e.Identifier, e.Scope, e.Type)
	}
	return fmt.Sprintf("no A2A agent %q in %s", e.Identifier, e.Scope)
}

// AmbiguousError reports an identifier listed by more than one catalog.
type AmbiguousError struct {
	Identifier string
	Catalogs   []string
}

func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("A2A agent %q is listed by more than one catalog: %s", e.Identifier, strings.Join(e.Catalogs, ", "))
}

// NoSingleAgentError reports a catalog used as an --agent-card that does not
// list exactly one A2A agent.
type NoSingleAgentError struct {
	Source string
	Agents []Entry
}

func (e *NoSingleAgentError) Error() string {
	if len(e.Agents) == 0 {
		return fmt.Sprintf("%s is an AI Catalog with no A2A agents", e.Source)
	}
	return fmt.Sprintf("%s is an AI Catalog with %d A2A agents; pick one with --catalog %s -a <identifier>", e.Source, len(e.Agents), e.Source)
}

// FindAgent returns the A2A agent with the identifier id among entries, which
// were collected from scope. When a catalog lists several versions, the
// latest is returned.
func FindAgent(entries []Entry, id, scope string) (Entry, error) {
	var matches []Entry
	var catalogs []string
	otherType := ""
	for _, e := range entries {
		if e.Identifier != id {
			continue
		}
		if !IsAgent(&e.CatalogEntry) {
			otherType = e.Type
			continue
		}
		matches = append(matches, e)
		if !slices.Contains(catalogs, e.Catalog) {
			catalogs = append(catalogs, e.Catalog)
		}
	}
	switch {
	case len(matches) == 0:
		return Entry{}, &NotFoundError{Identifier: id, Scope: scope, Type: otherType, Agents: Agents(entries)}
	case len(catalogs) > 1:
		return Entry{}, &AmbiguousError{Identifier: id, Catalogs: catalogs}
	}
	doc := catalog.AICatalog{}
	for _, m := range matches {
		doc.Entries = append(doc.Entries, m.CatalogEntry)
	}
	latest, _ := doc.GetLatest(id)
	for i := range doc.Entries {
		if &doc.Entries[i] == latest {
			return matches[i], nil
		}
	}
	return matches[0], nil
}

// SingleAgent returns the only A2A agent among the entries of the catalog at source.
func SingleAgent(entries []Entry, source string) (Entry, error) {
	agents := Agents(entries)
	if len(agents) != 1 {
		return Entry{}, &NoSingleAgentError{Source: source, Agents: agents}
	}
	return agents[0], nil
}

// CardURL returns the URL of the Agent Card an entry points to.
func CardURL(e Entry) (string, error) {
	if e.URL == "" {
		return "", fmt.Errorf("entry %q has neither data nor a URL", e.Identifier)
	}
	return resolveReference(e.Base, e.URL)
}

// IsCatalogDocument reports whether data is an AI Catalog rather than an Agent Card.
func IsCatalogDocument(data []byte) bool {
	var doc struct {
		SpecVersion *string          `json:"specVersion"`
		Entries     *json.RawMessage `json:"entries"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return false
	}
	return doc.SpecVersion != nil && doc.Entries != nil
}
