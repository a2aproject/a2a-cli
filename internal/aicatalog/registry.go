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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/Agent-Card/ai-catalog-go/catalog"
)

// Result is the outcome of fetching a catalog.
type Result struct {
	Catalog Catalog
	// Changed reports whether an update stored new content.
	Changed bool
	// Warnings describe nested catalogs that were skipped.
	Warnings []string
}

// Add fetches the catalog at source, a URL or local path, and registers it as name.
func (r *Registry) Add(ctx context.Context, f *Fetcher, name, source string) (*Result, error) {
	if err := r.writable(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("catalog name must not be empty")
	}
	sourceURL, err := NormalizeSource(source)
	if err != nil {
		return nil, err
	}
	if existing := r.find(name); existing != nil {
		return nil, fmt.Errorf("a catalog named %q is already registered", existing.Name)
	}
	if existing := r.find(sourceURL); existing != nil {
		return nil, fmt.Errorf("%s is already registered as %q", sourceURL, existing.Name)
	}

	res, err := r.fetch(ctx, f, sourceURL)
	if err != nil {
		return nil, err
	}
	res.Catalog.Name = name
	entry := &registryEntry{raw: map[string]json.RawMessage{
		"identifier":  mustMarshal(registryIdentifier(name)),
		"displayName": mustMarshal(name),
		"type":        mustMarshal(catalog.MediaTypeCatalog),
	}}
	if err := entry.set(res.Catalog, r.store.objectURL(res.Catalog.ContentHash)); err != nil {
		return nil, err
	}
	r.entries = append(r.entries, entry)
	if err := r.save(); err != nil {
		return nil, err
	}
	res.Changed = true
	return res, nil
}

// Update fetches a registered catalog from its source again.
func (r *Registry) Update(ctx context.Context, f *Fetcher, name string) (*Result, error) {
	if err := r.writable(); err != nil {
		return nil, err
	}
	entry := r.find(name)
	if entry == nil {
		return nil, &UnknownCatalogError{Name: name}
	}
	if entry.SourceType == "oci" {
		return nil, fmt.Errorf("catalog %q was imported from an OCI image layout; update it with ai-catalog", entry.Name)
	}
	res, err := r.fetch(ctx, f, entry.Source)
	if err != nil {
		return nil, err
	}
	res.Catalog.Name = entry.Name
	if res.Catalog.ContentHash == entry.ContentHash {
		res.Catalog = entry.Catalog
		return res, nil
	}
	if err := entry.set(res.Catalog, r.store.objectURL(res.Catalog.ContentHash)); err != nil {
		return nil, err
	}
	if err := r.save(); err != nil {
		return nil, err
	}
	res.Changed = true
	return res, nil
}

// Remove unregisters a catalog by name or source. Its stored document is
// deleted unless another registered catalog still uses it, directly or as a
// nested catalog.
func (r *Registry) Remove(ctx context.Context, nameOrSource string) (Catalog, error) {
	if err := r.writable(); err != nil {
		return Catalog{}, err
	}
	entry := r.find(nameOrSource)
	if entry == nil {
		return Catalog{}, &UnknownCatalogError{Name: nameOrSource}
	}
	r.entries = slices.DeleteFunc(r.entries, func(e *registryEntry) bool { return e == entry })
	if err := r.save(); err != nil {
		return Catalog{}, err
	}
	if slices.Contains(r.reachableObjects(ctx), entry.ContentHash) {
		return entry.Catalog, nil
	}
	if err := r.store.removeObject(entry.ContentHash); err != nil {
		return Catalog{}, err
	}
	maps.DeleteFunc(r.refs, func(_, hash string) bool { return hash == entry.ContentHash })
	if err := r.saveRefs(); err != nil {
		return Catalog{}, err
	}
	return entry.Catalog, nil
}

// reachableObjects returns the hashes of the documents the registered catalogs
// use, nested catalogs included.
func (r *Registry) reachableObjects(ctx context.Context) []string {
	var hashes []string
	for _, e := range r.entries {
		hashes = append(hashes, e.ContentHash)
	}
	entries, _, err := r.Entries(ctx, "")
	if err != nil {
		return hashes
	}
	for _, e := range entries {
		if !e.IsNestedCatalog() || e.URL == "" {
			continue
		}
		if ref, err := resolveReference(e.Base, e.URL); err == nil {
			if hash, ok := r.refs[ref]; ok {
				hashes = append(hashes, hash)
			}
		}
	}
	return hashes
}

// fetch stores the catalog at sourceURL and every catalog nested in it.
func (r *Registry) fetch(ctx context.Context, f *Fetcher, sourceURL string) (*Result, error) {
	data, err := f.Fetch(ctx, sourceURL)
	if err != nil {
		return nil, err
	}
	doc, err := catalog.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s is not an AI Catalog: %w", sourceURL, err)
	}
	hash, err := r.store.writeObject(data)
	if err != nil {
		return nil, err
	}
	r.refs[sourceURL] = hash

	w := &walker{store: r.store, refs: r.refs, fetcher: f, record: true, label: sourceURL, path: []string{sourceURL}}
	w.walk(ctx, doc, sourceURL, 0)
	if err := r.saveRefs(); err != nil {
		return nil, err
	}
	return &Result{
		Catalog: Catalog{
			Source:      sourceURL,
			ContentHash: hash,
			EntryCount:  countLeaves(w.entries),
			LastUpdated: time.Now().UTC().Format(time.RFC3339),
		},
		Warnings: w.warnings,
	}, nil
}

// Entries returns the entries of the registered catalog with the name or source
// nameOrSource, or of every registered catalog when it is empty, nested
// catalogs included. It reads only the local store.
func (r *Registry) Entries(ctx context.Context, nameOrSource string) ([]Entry, []string, error) {
	targets := r.entries
	if nameOrSource != "" {
		entry := r.find(nameOrSource)
		if entry == nil {
			return nil, nil, &UnknownCatalogError{Name: nameOrSource}
		}
		targets = []*registryEntry{entry}
	}
	var entries []Entry
	var warnings []string
	for _, e := range targets {
		if e.err != nil {
			continue
		}
		data, err := r.store.readObject(e.ContentHash)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("skipping catalog %q: %v", e.Name, err))
			continue
		}
		doc, err := catalog.Parse(data)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("skipping catalog %q: %v", e.Name, err))
			continue
		}
		w := &walker{store: r.store, refs: r.refs, label: e.Name, path: []string{e.Source}}
		w.walk(ctx, doc, e.Source, 0)
		entries = append(entries, w.entries...)
		warnings = append(warnings, w.warnings...)
	}
	return entries, warnings, nil
}

// Load fetches the catalog document data was served from at sourceURL, and
// the catalogs nested in it, without storing anything.
func Load(ctx context.Context, f *Fetcher, sourceURL string, data []byte) ([]Entry, []string, error) {
	doc, err := catalog.Parse(data)
	if err != nil {
		return nil, nil, fmt.Errorf("%s is not an AI Catalog: %w", sourceURL, err)
	}
	w := &walker{fetcher: f, label: sourceURL, path: []string{sourceURL}}
	w.walk(ctx, doc, sourceURL, 0)
	return w.entries, w.warnings, nil
}

// registryIdentifier derives a registry entry identifier from the catalog name
// the same way ai-catalog-cli does, so it stays stable across updates.
func registryIdentifier(name string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(name)))
	return "urn:ai-catalog:local:" + hex.EncodeToString(sum[:])[:8]
}

func countLeaves(entries []Entry) int {
	n := 0
	for _, e := range entries {
		if !e.IsNestedCatalog() {
			n++
		}
	}
	return n
}
