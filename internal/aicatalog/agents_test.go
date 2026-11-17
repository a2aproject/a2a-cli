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
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Agent-Card/ai-catalog-go/catalog"
	"github.com/google/go-cmp/cmp"
)

func agentEntry(cat, id, version string) Entry {
	return Entry{
		CatalogEntry: catalog.CatalogEntry{Identifier: id, Version: version, Type: catalog.MediaTypeA2AAgentCard, URL: "https://x.example/" + id},
		Catalog:      cat,
	}
}

func TestIsIdentifier(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}
	file, err := filepath.Rel(cwd, filepath.Join(dir, "urn:file.json"))
	if err != nil {
		t.Fatalf("filepath.Rel() error = %v", err)
	}
	if err := os.WriteFile(file, []byte("{}"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	tests := []struct {
		ref  string
		want bool
	}{
		{ref: "urn:air:acme.com:agent:finance", want: true},
		{ref: "URN:air:acme.com:agent:finance", want: true},
		{ref: "urn:", want: false},
		{ref: "https://acme.com", want: false},
		{ref: "acme.com", want: false},
		{ref: "./card.json", want: false},
		{ref: file, want: false},
	}
	for _, tt := range tests {
		if got := IsIdentifier(tt.ref); got != tt.want {
			t.Fatalf("IsIdentifier(%q) = %v, want %v", tt.ref, got, tt.want)
		}
	}
}

func TestNormalizeSource(t *testing.T) {
	t.Parallel()
	path := writeFile(t, "cat.json", "{}")

	tests := []struct {
		source  string
		want    string
		wantErr bool
	}{
		{source: "acme.com", want: "https://acme.com/.well-known/ai-catalog.json"},
		{source: "https://acme.com/", want: "https://acme.com/.well-known/ai-catalog.json"},
		{source: "https://acme.com/catalogs/finance.json", want: "https://acme.com/catalogs/finance.json"},
		{source: "http://localhost:8080", want: "http://localhost:8080/.well-known/ai-catalog.json"},
		{source: path, want: "file://" + filepath.ToSlash(path)},
		{source: "file:///tmp/c.json", want: "file:///tmp/c.json"},
		{source: "", wantErr: true},
		{source: "https://", wantErr: true},
		{source: "ftp://acme.com/c.json", wantErr: true},
	}
	for _, tt := range tests {
		got, err := NormalizeSource(tt.source)
		if tt.wantErr != (err != nil) {
			t.Fatalf("NormalizeSource(%q) error = %v, want error = %v", tt.source, err, tt.wantErr)
		}
		if got != tt.want {
			t.Fatalf("NormalizeSource(%q) = %v, want %v", tt.source, got, tt.want)
		}
	}
}

func TestFindAgent(t *testing.T) {
	t.Parallel()
	mcp := Entry{CatalogEntry: catalog.CatalogEntry{Identifier: "urn:mcp", Type: catalog.MediaTypeMCPServerCard}, Catalog: "corp"}
	entries := []Entry{
		agentEntry("corp", "urn:finance", "1.2.0"),
		agentEntry("corp", "urn:finance", "1.10.0"),
		agentEntry("corp", "urn:finance", "1.9.0"),
		agentEntry("corp", "urn:shared", ""),
		agentEntry("lab", "urn:shared", ""),
		mcp,
	}

	t.Run("picks the latest version", func(t *testing.T) {
		t.Parallel()
		got, err := FindAgent(entries, "urn:finance", "scope")
		if err != nil {
			t.Fatalf("FindAgent() error = %v", err)
		}
		if got.Version != "1.10.0" {
			t.Fatalf("FindAgent() Version = %v, want %v", got.Version, "1.10.0")
		}
	})

	t.Run("listed by two catalogs", func(t *testing.T) {
		t.Parallel()
		_, err := FindAgent(entries, "urn:shared", "scope")
		var ambiguous *AmbiguousError
		if !errors.As(err, &ambiguous) {
			t.Fatalf("FindAgent() error = %v, want *AmbiguousError", err)
		}
		if diff := cmp.Diff([]string{"corp", "lab"}, ambiguous.Catalogs); diff != "" {
			t.Fatalf("FindAgent() Catalogs wrong result (-want +got) diff = %s", diff)
		}
	})

	t.Run("not an agent", func(t *testing.T) {
		t.Parallel()
		_, err := FindAgent(entries, "urn:mcp", "scope")
		var notFound *NotFoundError
		if !errors.As(err, &notFound) || notFound.Type != catalog.MediaTypeMCPServerCard {
			t.Fatalf("FindAgent() error = %v, want *NotFoundError with the entry type", err)
		}
	})

	t.Run("missing lists the agents in scope", func(t *testing.T) {
		t.Parallel()
		_, err := FindAgent(entries, "urn:nope", "scope")
		var notFound *NotFoundError
		if !errors.As(err, &notFound) {
			t.Fatalf("FindAgent() error = %v, want *NotFoundError", err)
		}
		if diff := cmp.Diff([]string{"corp urn:finance", "corp urn:finance", "corp urn:finance", "corp urn:shared", "lab urn:shared"}, identifiers(notFound.Agents)); diff != "" {
			t.Fatalf("FindAgent() Agents wrong result (-want +got) diff = %s", diff)
		}
	})
}

func TestSingleAgent(t *testing.T) {
	t.Parallel()
	mcp := Entry{CatalogEntry: catalog.CatalogEntry{Identifier: "urn:mcp", Type: catalog.MediaTypeMCPServerCard}}

	tests := []struct {
		name       string
		entries    []Entry
		wantID     string
		wantAgents int
	}{
		{name: "one agent among other entries", entries: []Entry{mcp, agentEntry("c", "urn:a", "")}, wantID: "urn:a"},
		{name: "same agent listed twice", entries: []Entry{agentEntry("c", "urn:a", "1"), agentEntry("c", "urn:a", "1")}, wantID: "urn:a"},
		{name: "no agents", entries: []Entry{mcp}, wantAgents: 0},
		{name: "two agents", entries: []Entry{agentEntry("c", "urn:a", ""), agentEntry("c", "urn:b", "")}, wantAgents: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := SingleAgent(tt.entries, "src")
			if tt.wantID != "" {
				if err != nil || got.Identifier != tt.wantID {
					t.Fatalf("SingleAgent() = %v, %v, want %v", got.Identifier, err, tt.wantID)
				}
				return
			}
			var noSingle *NoSingleAgentError
			if !errors.As(err, &noSingle) || len(noSingle.Agents) != tt.wantAgents {
				t.Fatalf("SingleAgent() error = %v, want *NoSingleAgentError with %d agents", err, tt.wantAgents)
			}
		})
	}
}

func TestSearch(t *testing.T) {
	t.Parallel()
	finance := agentEntry("corp", "urn:air:acme.com:agent:finance", "")
	finance.Tags = []string{"Money"}
	hr := agentEntry("corp", "urn:air:acme.com:agent:hr", "")
	hr.Description = "Hiring and payroll"
	mcp := Entry{CatalogEntry: catalog.CatalogEntry{Identifier: "urn:air:acme.com:mcp:money", Type: catalog.MediaTypeMCPServerCard}}
	entries := []Entry{finance, hr, mcp}

	tests := []struct {
		query string
		want  []string
	}{
		{query: "", want: []string{"corp urn:air:acme.com:agent:finance", "corp urn:air:acme.com:agent:hr"}},
		{query: "money", want: []string{"corp urn:air:acme.com:agent:finance"}},
		{query: "PAYROLL", want: []string{"corp urn:air:acme.com:agent:hr"}},
		{query: "nothing", want: []string{}},
	}
	for _, tt := range tests {
		if diff := cmp.Diff(tt.want, identifiers(Search(entries, tt.query))); diff != "" {
			t.Fatalf("Search(%q) wrong result (-want +got) diff = %s", tt.query, diff)
		}
	}
}

func TestIsCatalogDocument(t *testing.T) {
	t.Parallel()
	tests := []struct {
		doc  string
		want bool
	}{
		{doc: `{"specVersion":"1.0","entries":[]}`, want: true},
		{doc: `{"name":"agent","supportedInterfaces":[]}`, want: false},
		{doc: `{"specVersion":"1.0"}`, want: false},
		{doc: `not json`, want: false},
	}
	for _, tt := range tests {
		if got := IsCatalogDocument([]byte(tt.doc)); got != tt.want {
			t.Fatalf("IsCatalogDocument(%s) = %v, want %v", tt.doc, got, tt.want)
		}
	}
}
