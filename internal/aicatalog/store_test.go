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
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

const fixtureStore = "testdata/ai-catalog-cli-store"

// catalogServer serves catalog documents by path and lets tests change them.
type catalogServer struct {
	*httptest.Server
	mu   sync.Mutex
	docs map[string]string
}

func newCatalogServer(t *testing.T, docs map[string]string) *catalogServer {
	t.Helper()
	s := &catalogServer{docs: docs}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		doc, ok := s.docs[r.URL.Path]
		s.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/ai-catalog+json")
		if _, err := w.Write([]byte(doc)); err != nil {
			t.Errorf("writing %s: %v", r.URL.Path, err)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *catalogServer) set(path, doc string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs[path] = doc
}

func (s *catalogServer) fetcher() *Fetcher { return &Fetcher{Client: s.Client()} }

func copyStore(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copying %s: %v", src, err)
	}
	return dst
}

func mustOpen(t *testing.T, dir string) *Registry {
	t.Helper()
	reg, err := (&Store{Dir: dir}).Open()
	if err != nil {
		t.Fatalf("Store.Open() error = %v", err)
	}
	return reg
}

func identifiers(entries []Entry) []string {
	ids := []string{}
	for _, e := range entries {
		ids = append(ids, e.Catalog+" "+e.Identifier)
	}
	return ids
}

func listObjects(t *testing.T, dir string) []string {
	t.Helper()
	files, err := os.ReadDir(filepath.Join(dir, objectsDir))
	if err != nil {
		t.Fatalf("os.ReadDir() error = %v", err)
	}
	names := []string{}
	for _, f := range files {
		names = append(names, f.Name())
	}
	return names
}

func TestStoreReadsAICatalogCLIStore(t *testing.T) {
	t.Parallel()
	reg := mustOpen(t, fixtureStore)
	if reg.Problem != nil {
		t.Fatalf("Store.Open() Problem = %v, want nil", reg.Problem)
	}

	wantCatalogs := []Catalog{{
		Name:        "corp",
		Source:      "file:///home/user/catalogs/corp.json",
		ContentHash: "1ca21b2f25e1e7ffdbe3d7628b1b79c6f73ac36e1e69b1e4d9df6e7549926c2a",
		EntryCount:  3,
		LastUpdated: "2026-11-12T14:40:39.268982908+00:00",
	}}
	if diff := cmp.Diff(wantCatalogs, reg.Catalogs()); diff != "" {
		t.Fatalf("Registry.Catalogs() wrong result (-want +got) diff = %s", diff)
	}

	entries, warnings, err := reg.Entries(t.Context(), "CORP")
	if err != nil {
		t.Fatalf("Registry.Entries() error = %v", err)
	}
	if len(warnings) > 0 {
		t.Fatalf("Registry.Entries() warnings = %v, want none", warnings)
	}
	wantAgents := []string{"corp urn:air:acme.com:agent:finance", "corp urn:air:acme.com:agent:hr"}
	if diff := cmp.Diff(wantAgents, identifiers(Agents(entries))); diff != "" {
		t.Fatalf("Agents() wrong result (-want +got) diff = %s", diff)
	}
}

func TestStoreMissingIsEmpty(t *testing.T) {
	t.Parallel()
	reg := mustOpen(t, filepath.Join(t.TempDir(), "missing"))
	if reg.Problem != nil {
		t.Fatalf("Store.Open() Problem = %v, want nil", reg.Problem)
	}
	if got := reg.Catalogs(); len(got) != 0 {
		t.Fatalf("Registry.Catalogs() = %v, want empty", got)
	}
}

func TestRegistryAddUpdateRemove(t *testing.T) {
	t.Parallel()
	srv := newCatalogServer(t, map[string]string{
		"/.well-known/ai-catalog.json": `{"specVersion":"1.0","entries":[
			{"identifier":"urn:a:agent","type":"application/a2a-agent-card+json","url":"/a.json"},
			{"identifier":"urn:a:nested","type":"application/ai-catalog+json","url":"/nested.json"}]}`,
		"/nested.json": `{"specVersion":"1.0","entries":[
			{"identifier":"urn:b:agent","type":"application/a2a-agent-card+json","url":"/b.json"}]}`,
	})
	dir := t.TempDir()
	ctx := t.Context()

	res, err := mustOpen(t, dir).Add(ctx, srv.fetcher(), "Corp", srv.URL)
	if err != nil {
		t.Fatalf("Registry.Add() error = %v", err)
	}
	wantAdded := Catalog{Name: "Corp", Source: srv.URL + "/.well-known/ai-catalog.json", EntryCount: 2}
	if diff := cmp.Diff(wantAdded, res.Catalog, cmpopts.IgnoreFields(Catalog{}, "ContentHash", "LastUpdated")); diff != "" {
		t.Fatalf("Registry.Add() wrong result (-want +got) diff = %s", diff)
	}

	reg := mustOpen(t, dir)
	entries, _, err := reg.Entries(ctx, "")
	if err != nil {
		t.Fatalf("Registry.Entries() error = %v", err)
	}
	wantAgents := []string{"Corp urn:a:agent", "Corp urn:b:agent"}
	if diff := cmp.Diff(wantAgents, identifiers(Agents(entries))); diff != "" {
		t.Fatalf("Registry.Entries() wrong result (-want +got) diff = %s", diff)
	}

	res, err = reg.Update(ctx, srv.fetcher(), "corp")
	if err != nil {
		t.Fatalf("Registry.Update() error = %v", err)
	}
	if res.Changed {
		t.Fatalf("Registry.Update() Changed = true, want false for unchanged content")
	}

	srv.set("/nested.json", `{"specVersion":"1.0","entries":[]}`)
	srv.set("/.well-known/ai-catalog.json", `{"specVersion":"1.0","entries":[
		{"identifier":"urn:c:agent","type":"application/a2a-agent-card+json","url":"/c.json"}]}`)
	res, err = reg.Update(ctx, srv.fetcher(), "corp")
	if err != nil {
		t.Fatalf("Registry.Update() error = %v", err)
	}
	if !res.Changed || res.Catalog.EntryCount != 1 {
		t.Fatalf("Registry.Update() = %+v, want Changed with 1 entry", res)
	}
	entries, _, err = mustOpen(t, dir).Entries(ctx, "")
	if err != nil {
		t.Fatalf("Registry.Entries() error = %v", err)
	}
	if diff := cmp.Diff([]string{"Corp urn:c:agent"}, identifiers(entries)); diff != "" {
		t.Fatalf("Registry.Entries() after update wrong result (-want +got) diff = %s", diff)
	}

	removed, err := mustOpen(t, dir).Remove(ctx, srv.URL+"/.well-known/ai-catalog.json")
	if err != nil {
		t.Fatalf("Registry.Remove() error = %v", err)
	}
	if removed.Name != "Corp" {
		t.Fatalf("Registry.Remove() Name = %q, want %q", removed.Name, "Corp")
	}
	reg = mustOpen(t, dir)
	if got := reg.Catalogs(); len(got) != 0 {
		t.Fatalf("Registry.Catalogs() after remove = %v, want empty", got)
	}
	for _, hash := range reg.refs {
		if hash == res.Catalog.ContentHash {
			t.Fatalf("refs still map to the removed catalog: %v", reg.refs)
		}
	}
	if slices.Contains(listObjects(t, dir), res.Catalog.ContentHash+".json") {
		t.Fatalf("stored object of the removed catalog was not deleted")
	}
}

func TestRegistryAddKeepsOtherEntriesIntact(t *testing.T) {
	t.Parallel()
	dir := copyStore(t, fixtureStore)
	before := readRegistryEntries(t, dir)
	srv := newCatalogServer(t, map[string]string{"/c.json": `{"specVersion":"1.0","entries":[]}`})

	if _, err := mustOpen(t, dir).Add(t.Context(), srv.fetcher(), "other", srv.URL+"/c.json"); err != nil {
		t.Fatalf("Registry.Add() error = %v", err)
	}

	after := readRegistryEntries(t, dir)
	if diff := cmp.Diff(before, after[:1]); diff != "" {
		t.Fatalf("existing registry entry changed (-want +got) diff = %s", diff)
	}
	added := after[1]
	ext := added["extensions"].(map[string]any)[extensionKey].(map[string]any)
	if added["identifier"] != registryIdentifier("other") || ext["sourceUrl"] != srv.URL+"/c.json" {
		t.Fatalf("added registry entry = %v, want ai-catalog-cli bookkeeping", added)
	}
}

func readRegistryEntries(t *testing.T, dir string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, registryFile))
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	var doc struct {
		Entries []map[string]any `json:"entries"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return doc.Entries
}

func TestRegistryIdentifierMatchesAICatalogCLI(t *testing.T) {
	t.Parallel()
	if got, want := registryIdentifier("Corp"), "urn:ai-catalog:local:19f4c684"; got != want {
		t.Fatalf("registryIdentifier() = %v, want %v", got, want)
	}
}

func TestRegistryAddErrors(t *testing.T) {
	t.Parallel()
	srv := newCatalogServer(t, map[string]string{
		"/c.json":    `{"specVersion":"1.0","entries":[]}`,
		"/card.json": `{"name":"not a catalog"`,
	})

	tests := []struct {
		name    string
		catName string
		source  string
		wantErr string
	}{
		{name: "name taken ignoring case", catName: "CORP", source: srv.URL + "/c.json", wantErr: "already registered"},
		{name: "source taken", catName: "other", source: "file:///home/user/catalogs/corp.json", wantErr: `already registered as "corp"`},
		{name: "not found", catName: "other", source: srv.URL + "/missing.json", wantErr: "404"},
		{name: "not a catalog", catName: "other", source: srv.URL + "/card.json", wantErr: "not an AI Catalog"},
		{name: "empty name", catName: " ", source: srv.URL + "/c.json", wantErr: "name must not be empty"},
		{name: "unsupported scheme", catName: "other", source: "ftp://example.com/c.json", wantErr: "unsupported catalog source"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := copyStore(t, fixtureStore)
			_, err := mustOpen(t, dir).Add(t.Context(), srv.fetcher(), tt.catName, tt.source)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Registry.Add() error = %v, want containing %q", err, tt.wantErr)
			}
			if got := mustOpen(t, dir).Catalogs(); len(got) != 1 {
				t.Fatalf("Registry.Catalogs() = %v, want only the fixture catalog", got)
			}
		})
	}
}

func TestRegistryRemoveKeepsSharedObjects(t *testing.T) {
	t.Parallel()
	dir := copyStore(t, fixtureStore)
	ctx := t.Context()
	nested := "file://" + filepath.ToSlash(writeFile(t, "hr.json", `{"specVersion":"1.0","entries":[]}`))
	if _, err := mustOpen(t, dir).Add(ctx, &Fetcher{}, "hr", nested); err != nil {
		t.Fatalf("Registry.Add() error = %v", err)
	}
	reg := mustOpen(t, dir)
	reg.refs["file:///home/user/catalogs/hr.json"] = reg.find("hr").ContentHash
	if err := reg.saveRefs(); err != nil {
		t.Fatalf("saveRefs() error = %v", err)
	}
	before := listObjects(t, dir)

	if _, err := mustOpen(t, dir).Remove(ctx, "hr"); err != nil {
		t.Fatalf("Registry.Remove() error = %v", err)
	}

	if diff := cmp.Diff(before, listObjects(t, dir)); diff != "" {
		t.Fatalf("Registry.Remove() deleted an object still used as a nested catalog (-want +got) diff = %s", diff)
	}
}

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	return path
}

func TestRegistryUnknownCatalog(t *testing.T) {
	t.Parallel()
	dir := copyStore(t, fixtureStore)
	reg := mustOpen(t, dir)
	var unknown *UnknownCatalogError
	if _, err := reg.Update(t.Context(), &Fetcher{}, "nope"); !errors.As(err, &unknown) {
		t.Fatalf("Registry.Update() error = %v, want *UnknownCatalogError", err)
	}
	if _, err := reg.Remove(t.Context(), "nope"); !errors.As(err, &unknown) {
		t.Fatalf("Registry.Remove() error = %v, want *UnknownCatalogError", err)
	}
	if _, _, err := reg.Entries(t.Context(), "nope"); !errors.As(err, &unknown) {
		t.Fatalf("Registry.Entries() error = %v, want *UnknownCatalogError", err)
	}
}

func TestStoreUnsupportedFormat(t *testing.T) {
	t.Parallel()
	validEntry := `{"identifier":"urn:ai-catalog:local:1","displayName":"ok","type":"application/ai-catalog+json",
		"extensions":{"org.ai-catalog.ai-catalog-cli":{"sourceUrl":"https://ok.example/c.json",
		"contentHash":"1ca21b2f25e1e7ffdbe3d7628b1b79c6f73ac36e1e69b1e4d9df6e7549926c2a"}}}`

	tests := []struct {
		name         string
		registry     string
		refs         string
		wantCatalogs []string
	}{
		{name: "registry is not JSON", registry: `{`},
		{name: "registry has unknown major version", registry: `{"specVersion":"2.0","entries":[]}`},
		{name: "entries is not a list", registry: `{"specVersion":"1.0","entries":{}}`},
		{
			name:         "entry without bookkeeping",
			registry:     `{"specVersion":"1.0","entries":[` + validEntry + `,{"identifier":"x","displayName":"bad","type":"application/ai-catalog+json"}]}`,
			wantCatalogs: []string{"ok"},
		},
		{
			name:         "entry with invalid hash",
			registry:     `{"specVersion":"1.0","entries":[{"identifier":"x","displayName":"bad","type":"application/ai-catalog+json","extensions":{"org.ai-catalog.ai-catalog-cli":{"sourceUrl":"https://x","contentHash":"../../etc/passwd"}}}]}`,
			wantCatalogs: []string{},
		},
		{
			name:         "refs is not a map",
			registry:     `{"specVersion":"1.0","entries":[` + validEntry + `]}`,
			refs:         `[]`,
			wantCatalogs: []string{"ok"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, registryFile), []byte(tt.registry), 0o644); err != nil {
				t.Fatalf("os.WriteFile() error = %v", err)
			}
			if tt.refs != "" {
				if err := os.WriteFile(filepath.Join(dir, refsFile), []byte(tt.refs), 0o644); err != nil {
					t.Fatalf("os.WriteFile() error = %v", err)
				}
			}

			reg := mustOpen(t, dir)
			var unsupported *UnsupportedFormatError
			if !errors.As(reg.Problem, &unsupported) {
				t.Fatalf("Store.Open() Problem = %v, want *UnsupportedFormatError", reg.Problem)
			}
			names := []string{}
			for _, c := range reg.Catalogs() {
				names = append(names, c.Name)
			}
			if diff := cmp.Diff(tt.wantCatalogs, names, cmpopts.EquateEmpty()); diff != "" {
				t.Fatalf("Registry.Catalogs() wrong result (-want +got) diff = %s", diff)
			}
			if _, err := reg.Add(t.Context(), &Fetcher{}, "new", "https://new.example/c.json"); !errors.As(err, &unsupported) {
				t.Fatalf("Registry.Add() error = %v, want *UnsupportedFormatError", err)
			}
			if _, err := reg.Remove(t.Context(), "ok"); !errors.As(err, &unsupported) {
				t.Fatalf("Registry.Remove() error = %v, want *UnsupportedFormatError", err)
			}
			data, err := os.ReadFile(filepath.Join(dir, registryFile))
			if err != nil {
				t.Fatalf("os.ReadFile() error = %v", err)
			}
			if string(data) != tt.registry {
				t.Fatalf("registry was modified: %s", data)
			}
		})
	}
}

func TestLoadNestedCatalogs(t *testing.T) {
	t.Parallel()
	srv := newCatalogServer(t, map[string]string{
		"/a.json": `{"specVersion":"1.0","entries":[
			{"identifier":"urn:a","type":"application/a2a-agent-card+json","url":"a-card.json"},
			{"identifier":"urn:to-b","type":"application/ai-catalog+json","url":"b.json"},
			{"identifier":"urn:inline","type":"application/ai-catalog+json","data":{"specVersion":"1.0","entries":[
				{"identifier":"urn:i","type":"application/a2a-agent-card+json","data":{"name":"I"}}]}},
			{"identifier":"urn:missing","type":"application/ai-catalog+json","url":"missing.json"},
			{"identifier":"urn:local","type":"application/ai-catalog+json","url":"file:///etc/passwd"}]}`,
		"/b.json": `{"specVersion":"1.0","entries":[
			{"identifier":"urn:b","type":"application/a2a-agent-card+json","url":"b-card.json"},
			{"identifier":"urn:back","type":"application/ai-catalog+json","url":"a.json"}]}`,
	})
	source := srv.URL + "/a.json"
	data := srv.docs["/a.json"]

	entries, warnings, err := Load(t.Context(), srv.fetcher(), source, []byte(data))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	wantAgents := []string{source + " urn:a", source + " urn:b", source + " urn:i"}
	if diff := cmp.Diff(wantAgents, identifiers(Agents(entries))); diff != "" {
		t.Fatalf("Load() agents wrong result (-want +got) diff = %s", diff)
	}
	sort.Strings(warnings)
	wantWarnings := []string{"circular reference", "404", "may not be loaded from"}
	if len(warnings) != len(wantWarnings) {
		t.Fatalf("Load() warnings = %q, want %d", warnings, len(wantWarnings))
	}
	for _, want := range wantWarnings {
		if !strings.Contains(strings.Join(warnings, "\n"), want) {
			t.Fatalf("Load() warnings = %q, want one containing %q", warnings, want)
		}
	}

	agent, err := FindAgent(entries, "urn:b", source)
	if err != nil {
		t.Fatalf("FindAgent() error = %v", err)
	}
	cardURL, err := CardURL(agent)
	if err != nil {
		t.Fatalf("CardURL() error = %v", err)
	}
	if want := srv.URL + "/b-card.json"; cardURL != want {
		t.Fatalf("CardURL() = %v, want %v", cardURL, want)
	}
}

func TestLoadStopsAtMaxDepth(t *testing.T) {
	t.Parallel()
	docs := map[string]string{}
	for i := 0; i <= maxDepth+1; i++ {
		docs["/"+string(rune('0'+i))+".json"] = `{"specVersion":"1.0","entries":[
			{"identifier":"urn:agent:` + string(rune('0'+i)) + `","type":"application/a2a-agent-card+json","url":"card.json"},
			{"identifier":"urn:next","type":"application/ai-catalog+json","url":"` + string(rune('0'+i+1)) + `.json"}]}`
	}
	srv := newCatalogServer(t, docs)

	entries, warnings, err := Load(t.Context(), srv.fetcher(), srv.URL+"/0.json", []byte(docs["/0.json"]))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := len(Agents(entries)); got != maxDepth+1 {
		t.Fatalf("Load() found %d agents, want %d", got, maxDepth+1)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "nesting deeper") {
		t.Fatalf("Load() warnings = %q, want one about nesting depth", warnings)
	}
}

func TestResolveReference(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		base    string
		ref     string
		want    string
		wantErr bool
	}{
		{name: "relative to https", base: "https://a.example/c/cat.json", ref: "card.json", want: "https://a.example/c/card.json"},
		{name: "absolute https", base: "https://a.example/cat.json", ref: "https://b.example/card.json", want: "https://b.example/card.json"},
		{name: "http from http", base: "http://127.0.0.1:8080/cat.json", ref: "card.json", want: "http://127.0.0.1:8080/card.json"},
		{name: "relative to file", base: "file:///home/u/cat.json", ref: "../x/sub.json", want: "file:///home/x/sub.json"},
		{name: "http from file", base: "file:///home/u/cat.json", ref: "http://localhost/card.json", want: "http://localhost/card.json"},
		{name: "file from https", base: "https://a.example/cat.json", ref: "file:///etc/passwd", wantErr: true},
		{name: "http from https", base: "https://a.example/cat.json", ref: "http://a.example/card.json", wantErr: true},
		{name: "other scheme", base: "https://a.example/cat.json", ref: "ftp://a.example/card.json", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveReference(tt.base, tt.ref)
			if tt.wantErr != (err != nil) {
				t.Fatalf("resolveReference() error = %v, want error = %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("resolveReference() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFetcherRejectsLargeDocuments(t *testing.T) {
	t.Parallel()
	path := writeFile(t, "big.json", strings.Repeat(" ", maxDocumentBytes+1))
	if _, err := (&Fetcher{}).Fetch(t.Context(), "file://"+path); err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("Fetcher.Fetch() error = %v, want a size error", err)
	}
}
