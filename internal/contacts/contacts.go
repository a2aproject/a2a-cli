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

// Package contacts implements the user's contact book: named A2A agents the
// user trusts, kept as an AI Catalog document that only a2a-cli writes.
//
// Each contact is a catalog entry of type application/a2a-agent-card+json. An
// entry copied from another catalog keeps its identifier, publisher and trust
// manifest as listed there; an agent added by its card URL gets an identifier
// derived from the contact name. a2a-cli keeps the contact name and note under
// the [ExtensionKey] extension of the entry.
package contacts

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/Agent-Card/ai-catalog-go/catalog"
	"github.com/a2aproject/a2a-go/v2/a2a"

	"github.com/a2aproject/a2a-cli/internal/aicatalog"
	"github.com/a2aproject/a2a-cli/internal/atomicfile"
)

const (
	// ExtensionKey namespaces the contact bookkeeping on a catalog entry.
	ExtensionKey = "org.a2aproject.a2a-cli"
	// IdentifierPrefix starts the identifier of an agent added by its card URL.
	IdentifierPrefix = "urn:ai-catalog:a2a-cli:"
	// CatalogLabel is the [aicatalog.Entry.Catalog] of contact entries.
	CatalogLabel = "contacts"

	fileName    = "catalog.json"
	specVersion = "1.0"
	hostName    = "a2a-cli contacts"
)

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

// IsName reports whether s is a valid contact name. Names can't contain dots,
// colons or slashes, so they never look like a URL, path, identifier or
// qualified host name.
func IsName(s string) bool {
	return namePattern.MatchString(s) && s != "localhost"
}

// ValidateName returns a descriptive error when name is not a valid contact name.
func ValidateName(name string) error {
	if IsName(name) {
		return nil
	}
	return fmt.Errorf("invalid contact name %q: use lowercase letters, digits, '-' and '_' (max 63 characters, not \"localhost\")", name)
}

// DefaultPath returns the path of the user's contact book.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".config", "a2a-cli", fileName), nil
}

// Contact is a named agent in the contact book.
type Contact struct {
	Name string
	Note string
	// AddedAt is when the contact was added.
	AddedAt time.Time
	// Source is what the contact was added from: the catalog that listed the
	// agent, or the agent's card URL.
	Source string
	// Entry is the catalog entry of the agent.
	Entry aicatalog.Entry
}

// CardURL returns the absolute URL of the contact's agent card, or "" when the
// entry embeds the card.
func (c Contact) CardURL() string {
	if len(c.Entry.Data) > 0 {
		return ""
	}
	u, err := aicatalog.CardURL(c.Entry)
	if err != nil {
		return ""
	}
	return u
}

// Origin returns scheme://host of the contact's card URL, or "" when the card
// is embedded or local.
func (c Contact) Origin() string {
	return Origin(c.CardURL())
}

// Origin returns scheme://host of an http(s) URL, or "" for anything else,
// including a host that is neither a DNS name nor an IP address.
func Origin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || !validHost(u.Hostname()) {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

// validHost reports whether host is an IP address or a DNS name, allowing
// internationalized labels but none of the punctuation a URL host may carry.
func validHost(host string) bool {
	if host == "" {
		return false
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return true
	}
	for _, r := range host {
		if r != '.' && r != '-' && r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

type bookkeeping struct {
	Name    string    `json:"name"`
	Note    string    `json:"note,omitempty"`
	AddedAt time.Time `json:"addedAt"`
	Source  string    `json:"source,omitempty"`
}

// FromCard returns a contact for the agent whose card was fetched from cardURL.
func FromCard(name, note, cardURL string, card *a2a.AgentCard, now time.Time) (Contact, error) {
	raw := map[string]json.RawMessage{
		"identifier": mustMarshal(IdentifierPrefix + name),
		"type":       mustMarshal(catalog.MediaTypeA2AAgentCard),
		"url":        mustMarshal(cardURL),
	}
	if card.Name != "" {
		raw["displayName"] = mustMarshal(card.Name)
	}
	if card.Description != "" {
		raw["description"] = mustMarshal(card.Description)
	}
	return newContact(raw, bookkeeping{Name: name, Note: note, AddedAt: now, Source: cardURL})
}

// FromEntry returns a contact for an A2A agent listed by a catalog. The entry
// is copied as listed, except that a relative URL is made absolute unless the
// trust manifest binds it; a bound URL keeps resolving against the catalog
// the contact was added from.
func FromEntry(name, note string, entry aicatalog.Entry, now time.Time) (Contact, error) {
	if !aicatalog.IsAgent(&entry.CatalogEntry) {
		return Contact{}, fmt.Errorf("%q is a %s entry, not an A2A agent", entry.Identifier, entry.Type)
	}
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(entry.Raw, &raw); err != nil || raw == nil {
		return Contact{}, fmt.Errorf("copying entry %q: %v", entry.Identifier, err)
	}
	if len(entry.Data) == 0 {
		cardURL, err := aicatalog.CardURL(entry)
		if err != nil {
			return Contact{}, err
		}
		if !urlBound(entry.CatalogEntry) {
			raw["url"] = mustMarshal(cardURL)
		}
	}
	return newContact(raw, bookkeeping{Name: name, Note: note, AddedAt: now, Source: entry.Base})
}

// urlBound reports whether the entry's trust manifest signs its URL, which a
// copy must then keep as listed.
func urlBound(entry catalog.CatalogEntry) bool {
	return entry.TrustManifest != nil && entry.TrustManifest.Subject != nil && entry.TrustManifest.Subject.URL != ""
}

func newContact(raw map[string]json.RawMessage, b bookkeeping) (Contact, error) {
	if err := ValidateName(b.Name); err != nil {
		return Contact{}, err
	}
	var extensions map[string]json.RawMessage
	if data, ok := raw["extensions"]; ok {
		if err := json.Unmarshal(data, &extensions); err != nil {
			return Contact{}, fmt.Errorf("decoding entry extensions: %w", err)
		}
	}
	if extensions == nil {
		extensions = map[string]json.RawMessage{}
	}
	extensions[ExtensionKey] = mustMarshal(b)
	raw["extensions"] = mustMarshal(extensions)
	c, ok, err := parseContact(mustMarshal(raw), "")
	if err != nil {
		return Contact{}, err
	}
	if !ok {
		return Contact{}, errors.New("entry is not a contact")
	}
	return c, nil
}

// parseContact decodes a catalog entry. It reports false for an entry without
// contact bookkeeping, which is kept in the book but is not a contact.
func parseContact(raw json.RawMessage, base string) (Contact, bool, error) {
	var entry catalog.CatalogEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return Contact{}, false, err
	}
	data, ok := entry.Extensions[ExtensionKey]
	if !ok {
		return Contact{}, false, nil
	}
	var b bookkeeping
	if err := json.Unmarshal(data, &b); err != nil {
		return Contact{}, false, fmt.Errorf("entry %q: %q extension: %w", entry.Identifier, ExtensionKey, err)
	}
	switch {
	case !IsName(b.Name):
		return Contact{}, false, fmt.Errorf("entry %q: %w", entry.Identifier, ValidateName(b.Name))
	case entry.Identifier == "":
		return Contact{}, false, fmt.Errorf("contact %q has no identifier", b.Name)
	case !aicatalog.IsAgent(&entry):
		return Contact{}, false, fmt.Errorf("contact %q has type %q, want %q", b.Name, entry.Type, catalog.MediaTypeA2AAgentCard)
	case len(entry.Data) == 0 && entry.URL == "":
		return Contact{}, false, fmt.Errorf("contact %q has neither data nor a URL", b.Name)
	}
	if urlBound(entry) && b.Source != "" {
		base = b.Source
	}
	return Contact{
		Name:    b.Name,
		Note:    b.Note,
		AddedAt: b.AddedAt,
		Source:  b.Source,
		Entry:   aicatalog.Entry{CatalogEntry: entry, Catalog: CatalogLabel, Base: base, Raw: raw},
	}, true, nil
}

// Book is a contact book loaded from a file.
type Book struct {
	path    string
	members map[string]json.RawMessage
	// entries are all entries of the catalog in document order; contacts index
	// the ones that are contacts.
	entries  []json.RawMessage
	contacts map[string]int
}

// Load reads the contact book at path. A missing file is an empty book. A file
// that is not an AI Catalog, or that lists an invalid contact, is an error, so
// that saving never discards what the file holds.
func Load(path string) (*Book, error) {
	b := &Book{path: path, contacts: map[string]int{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		b.members = map[string]json.RawMessage{
			"specVersion": mustMarshal(specVersion),
			"host":        mustMarshal(catalog.HostInfo{DisplayName: hostName}),
		}
		return b, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading contacts %s: %w", path, err)
	}
	if err := b.parse(data); err != nil {
		return nil, fmt.Errorf("parsing contacts %s: %w", path, err)
	}
	return b, nil
}

func (b *Book) parse(data []byte) error {
	if err := json.Unmarshal(data, &b.members); err != nil {
		return err
	}
	if b.members == nil {
		return errors.New("not a JSON object")
	}
	var version string
	if err := json.Unmarshal(b.members["specVersion"], &version); err != nil {
		return fmt.Errorf("specVersion: %w", err)
	}
	if major, _, _ := strings.Cut(version, "."); major != "1" {
		return fmt.Errorf("unsupported specVersion %q", version)
	}
	if entries, ok := b.members["entries"]; ok {
		if err := json.Unmarshal(entries, &b.entries); err != nil {
			return fmt.Errorf("entries: %w", err)
		}
	}
	for i, raw := range b.entries {
		c, ok, err := parseContact(raw, b.URL())
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if _, dup := b.contacts[c.Name]; dup {
			return fmt.Errorf("duplicate contact %q", c.Name)
		}
		b.contacts[c.Name] = i
	}
	return nil
}

// Path returns the file the book is stored in.
func (b *Book) Path() string { return b.path }

// URL returns the file URL of the book.
func (b *Book) URL() string { return FileURL(b.path) }

// FileURL returns the file URL of the book at path.
func FileURL(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

// Get returns the named contact.
func (b *Book) Get(name string) (Contact, bool) {
	i, ok := b.contacts[name]
	if !ok {
		return Contact{}, false
	}
	c, _, err := parseContact(b.entries[i], b.URL())
	return c, err == nil
}

// List returns all contacts sorted by name.
func (b *Book) List() []Contact {
	list := make([]Contact, 0, len(b.contacts))
	for name := range b.contacts {
		if c, ok := b.Get(name); ok {
			list = append(list, c)
		}
	}
	slices.SortFunc(list, func(x, y Contact) int { return strings.Compare(x.Name, y.Name) })
	return list
}

// Entries returns the catalog entries of all contacts.
func (b *Book) Entries() []aicatalog.Entry {
	list := b.List()
	entries := make([]aicatalog.Entry, 0, len(list))
	for _, c := range list {
		entries = append(entries, c.Entry)
	}
	return entries
}

// ExistsError reports a contact that conflicts with one in the book.
type ExistsError struct {
	// Name is the contact in the book.
	Name string
	// SameAgent is set when the conflict is the agent rather than the name.
	SameAgent bool
}

func (e *ExistsError) Error() string {
	if e.SameAgent {
		return fmt.Sprintf("the agent is already a contact named %q", e.Name)
	}
	return fmt.Sprintf("contact %q already exists", e.Name)
}

// NotFoundError reports a name that is not in the book.
type NotFoundError struct {
	Name string
	Path string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("no contact named %q in %s", e.Name, e.Path)
}

// Add stores c, replacing a contact of the same name only when replace is set.
// Two contacts can't share an identifier, whatever their versions, so an
// identifier names one contact.
func (b *Book) Add(c Contact, replace bool) error {
	if err := ValidateName(c.Name); err != nil {
		return err
	}
	if _, ok := b.contacts[c.Name]; ok && !replace {
		return &ExistsError{Name: c.Name}
	}
	for _, other := range b.List() {
		if other.Name != c.Name && other.Entry.Identifier == c.Entry.Identifier {
			return &ExistsError{Name: other.Name, SameAgent: true}
		}
	}
	if i, ok := b.contacts[c.Name]; ok {
		b.entries[i] = c.Entry.Raw
		return nil
	}
	b.contacts[c.Name] = len(b.entries)
	b.entries = append(b.entries, c.Entry.Raw)
	return nil
}

// Remove deletes the named contact.
func (b *Book) Remove(name string) error {
	i, ok := b.contacts[name]
	if !ok {
		return &NotFoundError{Name: name, Path: b.path}
	}
	b.entries = slices.Delete(b.entries, i, i+1)
	delete(b.contacts, name)
	for n, j := range b.contacts {
		if j > i {
			b.contacts[n] = j - 1
		}
	}
	return nil
}

// Save writes the book back to its file atomically.
func (b *Book) Save() error {
	entries := b.entries
	if entries == nil {
		entries = []json.RawMessage{}
	}
	b.members["entries"] = mustMarshal(entries)
	data, err := json.MarshalIndent(b.members, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding contacts: %w", err)
	}
	if err := atomicfile.Write(b.path, data, 0o600); err != nil {
		return fmt.Errorf("saving contacts: %w", err)
	}
	return nil
}

func mustMarshal(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("contacts: marshal %T: %v", v, err))
	}
	return data
}
