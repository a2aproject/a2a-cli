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

// Package aicatalog keeps registered AI Catalogs in the local store shared with
// ai-catalog-cli, and finds the A2A agents those catalogs list.
//
// The store layout is not part of the AI Catalog specification. This package
// follows ai-catalog-cli: catalog.json is an AI Catalog whose entries are the
// registered catalogs, refs.json maps each fetched URL to the SHA-256 of its
// content, and objects/ holds every fetched document named by that hash.
package aicatalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Agent-Card/ai-catalog-go/catalog"

	"github.com/a2aproject/a2a-cli/internal/atomicfile"
)

// EnvDir names the environment variable that overrides the store directory.
const EnvDir = "AI_CATALOG_CACHE_DIR"

const (
	registryFile = "catalog.json"
	refsFile     = "refs.json"
	objectsDir   = "objects"

	// extensionKey namespaces the bookkeeping ai-catalog-cli keeps on a registry entry.
	extensionKey = "org.ai-catalog.ai-catalog-cli"

	registrySpecVersion = "1.0"
	registryHostName    = "ai-catalog local registry"
)

// DefaultDir returns the store directory: $AI_CATALOG_CACHE_DIR, or ~/.ai-catalog.
func DefaultDir() (string, error) {
	if dir := os.Getenv(EnvDir); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".ai-catalog"), nil
}

// Store is a local catalog store rooted at Dir.
type Store struct {
	Dir string
}

// Catalog is a registered catalog.
type Catalog struct {
	// Name is the name the catalog was registered under, unique ignoring case.
	Name string
	// Source is the URL the catalog was fetched from.
	Source string
	// ContentHash is the SHA-256 of the stored document.
	ContentHash string
	// EntryCount is the number of entries that are not catalogs, nested ones included.
	EntryCount int
	// LastUpdated is when the catalog was last fetched, in RFC 3339.
	LastUpdated string
	// SourceType is "oci" for a catalog imported from an OCI image layout.
	SourceType string
}

// UnknownCatalogError reports a name or source that matches no registered catalog.
type UnknownCatalogError struct {
	Name string
}

func (e *UnknownCatalogError) Error() string {
	return fmt.Sprintf("no registered catalog matches %q", e.Name)
}

// UnsupportedFormatError reports a store this package does not recognize and
// therefore does not modify.
type UnsupportedFormatError struct {
	Dir string
	Err error
}

func (e *UnsupportedFormatError) Error() string {
	return fmt.Sprintf("unrecognized AI Catalog store format in %s: %v", e.Dir, e.Err)
}

func (e *UnsupportedFormatError) Unwrap() error { return e.Err }

// Registry is the content of a store, read by [Store.Open].
type Registry struct {
	store   *Store
	members map[string]json.RawMessage
	entries []*registryEntry
	refs    map[string]string

	// Problem is an [*UnsupportedFormatError] when the store is not in a format
	// this package recognizes. Such a store is read on a best-effort basis and
	// every method that would modify it fails.
	Problem error
}

type registryEntry struct {
	Catalog
	raw map[string]json.RawMessage
	ext map[string]json.RawMessage
	err error
}

type bookkeeping struct {
	SourceURL   string `json:"sourceUrl"`
	ContentHash string `json:"contentHash"`
	EntryCount  int    `json:"entryCount"`
	LastUpdated string `json:"lastUpdated"`
	SourceType  string `json:"sourceType"`
}

// Open reads the registry. A missing store reads as an empty one.
func (s *Store) Open() (*Registry, error) {
	r := &Registry{store: s, refs: map[string]string{}}
	var problems []error

	data, err := os.ReadFile(s.path(registryFile))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		r.members = newRegistryMembers()
	case err != nil:
		return nil, fmt.Errorf("reading catalog registry: %w", err)
	default:
		if err := r.parseRegistry(data); err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", registryFile, err))
		}
	}
	for _, e := range r.entries {
		if e.err != nil {
			problems = append(problems, fmt.Errorf("%s: entry %q: %w", registryFile, e.Name, e.err))
		}
	}

	data, err = os.ReadFile(s.path(refsFile))
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, fmt.Errorf("reading catalog refs: %w", err)
	default:
		if err := json.Unmarshal(data, &r.refs); err != nil {
			r.refs = map[string]string{}
			problems = append(problems, fmt.Errorf("%s: %w", refsFile, err))
		}
	}

	if len(problems) > 0 {
		r.Problem = &UnsupportedFormatError{Dir: s.Dir, Err: errors.Join(problems...)}
	}
	return r, nil
}

func newRegistryMembers() map[string]json.RawMessage {
	return map[string]json.RawMessage{
		"specVersion": mustMarshal(registrySpecVersion),
		"host":        mustMarshal(catalog.HostInfo{DisplayName: registryHostName}),
		"entries":     json.RawMessage("[]"),
	}
}

func (r *Registry) parseRegistry(data []byte) error {
	if err := json.Unmarshal(data, &r.members); err != nil {
		return err
	}
	if r.members == nil {
		return errors.New("not a JSON object")
	}
	var version string
	if err := json.Unmarshal(r.members["specVersion"], &version); err != nil {
		return fmt.Errorf("specVersion: %w", err)
	}
	if major, _, _ := strings.Cut(version, "."); major != "1" {
		return fmt.Errorf("unsupported specVersion %q", version)
	}
	var raws []map[string]json.RawMessage
	if err := json.Unmarshal(r.members["entries"], &raws); err != nil {
		return fmt.Errorf("entries: %w", err)
	}
	for _, raw := range raws {
		r.entries = append(r.entries, parseRegistryEntry(raw))
	}
	return nil
}

func parseRegistryEntry(raw map[string]json.RawMessage) *registryEntry {
	e := &registryEntry{raw: raw}
	data, err := json.Marshal(raw)
	if err != nil {
		e.err = err
		return e
	}
	var parsed catalog.CatalogEntry
	if err := json.Unmarshal(data, &parsed); err != nil {
		e.err = err
		return e
	}
	e.Name = parsed.DisplayName
	if e.Name == "" {
		e.Name = parsed.Identifier
	}
	if parsed.Type != catalog.MediaTypeCatalog {
		e.err = fmt.Errorf("type %q is not %q", parsed.Type, catalog.MediaTypeCatalog)
		return e
	}
	extData, ok := parsed.Extensions[extensionKey]
	if !ok {
		e.err = fmt.Errorf("missing %q extension", extensionKey)
		return e
	}
	var b bookkeeping
	if err := json.Unmarshal(extData, &e.ext); err != nil {
		e.err = fmt.Errorf("%q extension: %w", extensionKey, err)
		return e
	}
	if err := json.Unmarshal(extData, &b); err != nil {
		e.err = fmt.Errorf("%q extension: %w", extensionKey, err)
		return e
	}
	e.Source = b.SourceURL
	e.ContentHash = b.ContentHash
	e.EntryCount = b.EntryCount
	e.LastUpdated = b.LastUpdated
	e.SourceType = b.SourceType
	switch {
	case parsed.DisplayName == "":
		e.err = errors.New("missing displayName")
	case e.Source == "":
		e.err = errors.New("missing sourceUrl")
	case !isContentHash(e.ContentHash):
		e.err = fmt.Errorf("invalid contentHash %q", e.ContentHash)
	}
	return e
}

// Catalogs returns the registered catalogs in registration order.
func (r *Registry) Catalogs() []Catalog {
	catalogs := make([]Catalog, 0, len(r.entries))
	for _, e := range r.entries {
		if e.err == nil {
			catalogs = append(catalogs, e.Catalog)
		}
	}
	return catalogs
}

// Lookup returns the registered catalog with the name or source nameOrSource,
// compared ignoring case.
func (r *Registry) Lookup(nameOrSource string) (Catalog, bool) {
	if e := r.find(nameOrSource); e != nil {
		return e.Catalog, true
	}
	return Catalog{}, false
}

func (r *Registry) find(nameOrSource string) *registryEntry {
	for _, e := range r.entries {
		if e.err == nil && (strings.EqualFold(e.Name, nameOrSource) || strings.EqualFold(e.Source, nameOrSource)) {
			return e
		}
	}
	return nil
}

func (r *Registry) writable() error {
	if r.Problem != nil {
		return fmt.Errorf("refusing to modify the store: %w", r.Problem)
	}
	return nil
}

func (r *Registry) save() error {
	raws := make([]map[string]json.RawMessage, 0, len(r.entries))
	for _, e := range r.entries {
		raws = append(raws, e.raw)
	}
	entries, err := json.Marshal(raws)
	if err != nil {
		return err
	}
	r.members["entries"] = entries
	data, err := json.MarshalIndent(r.members, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.Write(r.store.path(registryFile), data, 0o644)
}

func (r *Registry) saveRefs() error {
	data, err := json.MarshalIndent(r.refs, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.Write(r.store.path(refsFile), data, 0o644)
}

func (e *registryEntry) set(c Catalog, objectURL string) error {
	e.Catalog = c
	if e.ext == nil {
		e.ext = map[string]json.RawMessage{}
	}
	e.ext["sourceUrl"] = mustMarshal(c.Source)
	e.ext["contentHash"] = mustMarshal(c.ContentHash)
	e.ext["entryCount"] = mustMarshal(c.EntryCount)
	e.ext["lastUpdated"] = mustMarshal(c.LastUpdated)

	extensions := map[string]json.RawMessage{}
	if data, ok := e.raw["extensions"]; ok {
		if err := json.Unmarshal(data, &extensions); err != nil {
			return fmt.Errorf("decoding extensions of %q: %w", c.Name, err)
		}
	}
	ext, err := json.Marshal(e.ext)
	if err != nil {
		return err
	}
	extensions[extensionKey] = ext
	e.raw["extensions"] = mustMarshal(extensions)
	e.raw["url"] = mustMarshal(objectURL)
	e.raw["updatedAt"] = mustMarshal(c.LastUpdated)
	return nil
}

func (s *Store) path(name string) string { return filepath.Join(s.Dir, name) }

func (s *Store) objectPath(hash string) (string, error) {
	if !isContentHash(hash) {
		return "", fmt.Errorf("invalid content hash %q", hash)
	}
	return filepath.Join(s.Dir, objectsDir, hash+".json"), nil
}

// objectURL is the file URL ai-catalog-cli records as a registry entry's url.
func (s *Store) objectURL(hash string) string {
	path, err := s.objectPath(hash)
	if err != nil {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

func (s *Store) readObject(hash string) ([]byte, error) {
	path, err := s.objectPath(hash)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading stored catalog: %w", err)
	}
	return data, nil
}

func (s *Store) writeObject(data []byte) (string, error) {
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	path, err := s.objectPath(hash)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err == nil {
		return hash, nil
	}
	if err := atomicfile.Write(path, data, 0o644); err != nil {
		return "", err
	}
	return hash, nil
}

func (s *Store) removeObject(hash string) error {
	path, err := s.objectPath(hash)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("removing stored catalog: %w", err)
	}
	return nil
}

func isContentHash(s string) bool {
	if len(s) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func mustMarshal(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("aicatalog: marshal %T: %v", v, err))
	}
	return data
}
