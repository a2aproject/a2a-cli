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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"slices"

	"github.com/Agent-Card/ai-catalog-go/catalog"
)

const (
	// maxDocumentBytes caps a fetched catalog document.
	maxDocumentBytes = 10 << 20
	// maxDepth is how many levels of nested catalogs are followed.
	maxDepth = 4
)

// Fetcher retrieves catalog documents over HTTP or from local files.
type Fetcher struct {
	Client *http.Client
}

// Fetch returns the document at rawURL, an http, https or file URL.
func (f *Fetcher) Fetch(ctx context.Context, rawURL string) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid catalog URL %q: %w", rawURL, err)
	}
	switch u.Scheme {
	case "file":
		return readFileURL(u)
	case "http", "https":
	default:
		return nil, fmt.Errorf("unsupported catalog URL %q: want http, https or file", rawURL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", catalog.MediaTypeCatalog+", application/json")
	client := f.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	data, err := readCapped(resp.Body, rawURL)
	if cerr := resp.Body.Close(); cerr != nil && err == nil {
		err = cerr
	}
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &StatusError{URL: rawURL, StatusCode: resp.StatusCode, Status: resp.Status}
	}
	return data, nil
}

// StatusError reports a catalog request answered with a non-OK status.
type StatusError struct {
	URL        string
	StatusCode int
	Status     string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("fetching %s: %s", e.URL, e.Status)
}

func readFileURL(u *url.URL) ([]byte, error) {
	if u.Host != "" && u.Host != "localhost" {
		return nil, fmt.Errorf("unsupported file URL host %q", u.Host)
	}
	f, err := os.Open(u.Path)
	if err != nil {
		return nil, err
	}
	data, err := readCapped(f, u.String())
	if cerr := f.Close(); cerr != nil && err == nil {
		err = cerr
	}
	return data, err
}

func readCapped(r io.Reader, name string) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxDocumentBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", name, err)
	}
	if len(data) > maxDocumentBytes {
		return nil, fmt.Errorf("%s is larger than %d MiB", name, maxDocumentBytes>>20)
	}
	return data, nil
}

// Entry is a catalog entry and where it was found.
type Entry struct {
	catalog.CatalogEntry
	// Catalog is the name of the registered catalog the entry was found in, or
	// the URL of the catalog when it is not registered.
	Catalog string
	// Base is the URL of the catalog document that lists the entry, against
	// which a relative entry URL resolves.
	Base string
}

// walker collects the entries of a catalog and the catalogs nested in it. An
// online walker fetches nested catalogs; an offline one reads them from the
// store. Problems with a nested catalog become warnings.
type walker struct {
	store   *Store
	refs    map[string]string
	fetcher *Fetcher
	// record keeps fetched documents in the store and refs.
	record bool

	label    string
	path     []string
	entries  []Entry
	warnings []string
}

func (w *walker) walk(ctx context.Context, doc *catalog.AICatalog, base string, depth int) {
	for _, entry := range doc.Entries {
		w.entries = append(w.entries, Entry{CatalogEntry: entry, Catalog: w.label, Base: base})
		if !entry.IsNestedCatalog() {
			continue
		}
		if depth >= maxDepth {
			w.warn(entry, fmt.Errorf("nesting deeper than %d levels", maxDepth))
			continue
		}
		child, childBase, err := w.open(ctx, &entry, base)
		if err != nil {
			w.warn(entry, err)
			continue
		}
		if child == nil {
			continue
		}
		w.path = append(w.path, childBase)
		w.walk(ctx, child, childBase, depth+1)
		w.path = w.path[:len(w.path)-1]
	}
}

func (w *walker) warn(entry catalog.CatalogEntry, err error) {
	w.warnings = append(w.warnings, fmt.Sprintf("skipping nested catalog %q in %s: %v", entry.Identifier, w.label, err))
}

func (w *walker) open(ctx context.Context, entry *catalog.CatalogEntry, base string) (*catalog.AICatalog, string, error) {
	if len(entry.Data) > 0 {
		doc, err := catalog.Parse(entry.Data)
		return doc, base, err
	}
	if entry.URL == "" {
		return nil, "", nil
	}
	ref, err := resolveReference(base, entry.URL)
	if err != nil {
		return nil, "", err
	}
	if slices.Contains(w.path, ref) {
		return nil, "", fmt.Errorf("circular reference to %s", ref)
	}
	data, err := w.load(ctx, ref)
	if err != nil {
		return nil, "", err
	}
	doc, err := catalog.Parse(data)
	return doc, ref, err
}

func (w *walker) load(ctx context.Context, ref string) ([]byte, error) {
	if w.fetcher == nil {
		hash, ok := w.refs[ref]
		if !ok {
			return nil, fmt.Errorf("%s is not in the local store; run `a2a catalog update`", ref)
		}
		return w.store.readObject(hash)
	}
	data, err := w.fetcher.Fetch(ctx, ref)
	if err != nil {
		return nil, err
	}
	if _, err := catalog.Parse(data); err != nil {
		return nil, err
	}
	if w.record {
		hash, err := w.store.writeObject(data)
		if err != nil {
			return nil, err
		}
		w.refs[ref] = hash
	}
	return data, nil
}

// resolveReference resolves ref against the catalog at base. Any catalog may
// reference https URLs, a catalog served over http or read from a local file
// may reference http URLs, and only a local catalog may reference local files.
func resolveReference(base, ref string) (string, error) {
	b, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("invalid catalog URL %q: %w", base, err)
	}
	r, err := url.Parse(ref)
	if err != nil {
		return "", fmt.Errorf("invalid reference %q: %w", ref, err)
	}
	u := b.ResolveReference(r)
	if u.Scheme == "file" {
		u.Path = path.Clean(u.Path)
	}
	switch {
	case u.Scheme == "https":
	case u.Scheme == "http" && (b.Scheme == "http" || b.Scheme == "file"):
	case u.Scheme == "file" && b.Scheme == "file":
	default:
		return "", fmt.Errorf("%s may not be loaded from %s", u, base)
	}
	return u.String(), nil
}
