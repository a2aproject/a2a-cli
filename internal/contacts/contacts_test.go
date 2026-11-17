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

package contacts

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Agent-Card/ai-catalog-go/catalog"
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/google/go-cmp/cmp"

	"github.com/a2aproject/a2a-cli/internal/aicatalog"
)

var testNow = time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)

// signedEntry is a catalog entry with members a2a-cli does not model, which a
// copy must keep as listed.
const signedEntry = `{"identifier":"urn:air:acme.com:agent:finance","type":"application/a2a-agent-card+json",` +
	`"url":"agents/finance.json","publisher":{"identifier":"did:web:acme.com","displayName":"Acme"},` +
	`"trustManifest":{"identity":"did:web:acme.com","issuedAt":"2026-01-01T00:00:00Z","signature":"e30..c2ln","vendorField":{"kept":true}},` +
	`"extensions":{"com.acme":{"tier":"gold"}}}`

func catalogEntry(t *testing.T, raw string) aicatalog.Entry {
	t.Helper()
	var entry catalog.CatalogEntry
	if err := json.Unmarshal([]byte(raw), &entry); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return aicatalog.Entry{CatalogEntry: entry, Catalog: "acme", Base: "https://acme.com/ai-catalog.json", Raw: json.RawMessage(raw)}
}

func mustFromCard(t *testing.T, name, cardURL string) Contact {
	t.Helper()
	c, err := FromCard(name, "", cardURL, &a2a.AgentCard{Name: "Agent " + name, Description: "Does " + name}, testNow)
	if err != nil {
		t.Fatalf("FromCard() error = %v", err)
	}
	return c
}

func mustFromEntry(t *testing.T, name, raw string) Contact {
	t.Helper()
	c, err := FromEntry(name, "", catalogEntry(t, raw), testNow)
	if err != nil {
		t.Fatalf("FromEntry() error = %v", err)
	}
	return c
}

func mustLoad(t *testing.T, path string) *Book {
	t.Helper()
	b, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	return b
}

func names(contacts []Contact) []string {
	list := []string{}
	for _, c := range contacts {
		list = append(list, c.Name)
	}
	return list
}

func TestIsName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		want bool
	}{
		{name: "nimbus", want: true},
		{name: "fin-2_b", want: true},
		{name: "0agent", want: true},
		{name: "Nimbus", want: false},
		{name: "agent.example", want: false},
		{name: "urn:x", want: false},
		{name: "./card.json", want: false},
		{name: "-agent", want: false},
		{name: "localhost", want: false},
		{name: strings.Repeat("a", 64), want: false},
		{name: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsName(tt.name); got != tt.want {
				t.Fatalf("IsName(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestOrigin(t *testing.T) {
	t.Parallel()
	tests := []struct {
		ref  string
		want string
	}{
		{ref: "https://Nimbus.example/.well-known/agent-card.json", want: "https://nimbus.example"},
		{ref: "http://localhost:8080/card.json", want: "http://localhost:8080"},
		{ref: "https://[::1]:8443/card.json", want: "https://[::1]:8443"},
		{ref: "file:///tmp/card.json", want: ""},
		{ref: "http://a;id.example/card.json", want: ""},
		{ref: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			t.Parallel()
			if got := Origin(tt.ref); got != tt.want {
				t.Fatalf("Origin(%q) = %v, want %v", tt.ref, got, tt.want)
			}
		})
	}
}

func TestBookRoundTrip(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "a2a-cli", "catalog.json")
	b := mustLoad(t, path)
	if got := b.List(); len(got) != 0 {
		t.Fatalf("Book.List() = %v, want empty for a missing file", got)
	}

	fin, err := FromEntry("fin", "books", catalogEntry(t, signedEntry), testNow)
	if err != nil {
		t.Fatalf("FromEntry() error = %v", err)
	}
	for _, c := range []Contact{mustFromCard(t, "weather", "https://weather.example/.well-known/agent-card.json"), fin} {
		if err := b.Add(c, false); err != nil {
			t.Fatalf("Book.Add(%s) error = %v", c.Name, err)
		}
	}
	if err := b.Save(); err != nil {
		t.Fatalf("Book.Save() error = %v", err)
	}

	loaded := mustLoad(t, path)
	if diff := cmp.Diff([]string{"fin", "weather"}, names(loaded.List())); diff != "" {
		t.Fatalf("Book.List() wrong result (-want +got) diff = %s", diff)
	}
	got, ok := loaded.Get("fin")
	if !ok {
		t.Fatalf("Book.Get(fin) ok = false, want true")
	}
	want := Contact{Name: "fin", Note: "books", AddedAt: testNow, Source: "https://acme.com/ai-catalog.json"}
	if diff := cmp.Diff(want, got, cmp.FilterPath(func(p cmp.Path) bool { return p.String() == "Entry" }, cmp.Ignore())); diff != "" {
		t.Fatalf("Book.Get(fin) wrong result (-want +got) diff = %s", diff)
	}
	if got.CardURL() != "https://acme.com/agents/finance.json" || got.Origin() != "https://acme.com" {
		t.Fatalf("Contact.CardURL(), Origin() = %v, %v; want the absolute card URL and its origin", got.CardURL(), got.Origin())
	}
	if got.Entry.Catalog != CatalogLabel || got.Entry.Base != loaded.URL() {
		t.Fatalf("Contact.Entry catalog, base = %q, %q; want %q, %q", got.Entry.Catalog, got.Entry.Base, CatalogLabel, loaded.URL())
	}
	if _, err := catalog.ParseFile(path); err != nil {
		t.Fatalf("catalog.ParseFile() error = %v, want the book to be an AI Catalog", err)
	}

	weather, _ := loaded.Get("weather")
	wantWeather := catalog.CatalogEntry{
		Identifier:  "urn:ai-catalog:a2a-cli:weather",
		DisplayName: "Agent weather",
		Description: "Does weather",
		Type:        catalog.MediaTypeA2AAgentCard,
		URL:         "https://weather.example/.well-known/agent-card.json",
	}
	if diff := cmp.Diff(wantWeather, weather.Entry.CatalogEntry, cmp.FilterPath(func(p cmp.Path) bool { return p.String() == "Extensions" }, cmp.Ignore())); diff != "" {
		t.Fatalf("contact entry from a card wrong result (-want +got) diff = %s", diff)
	}
}

func TestFromEntryKeepsEntryAsListed(t *testing.T) {
	t.Parallel()
	c := mustFromEntry(t, "fin", signedEntry)
	var orig, copied map[string]json.RawMessage
	if err := json.Unmarshal([]byte(signedEntry), &orig); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if err := json.Unmarshal(c.Entry.Raw, &copied); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	for _, key := range []string{"identifier", "type", "publisher", "trustManifest"} {
		if string(orig[key]) != string(copied[key]) {
			t.Fatalf("copied %s = %s, want %s as listed", key, copied[key], orig[key])
		}
	}
	var ext map[string]json.RawMessage
	if err := json.Unmarshal(copied["extensions"], &ext); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if string(ext["com.acme"]) != `{"tier":"gold"}` || ext[ExtensionKey] == nil {
		t.Fatalf("copied extensions = %s, want the vendor extension and the contact bookkeeping", copied["extensions"])
	}
}

func TestFromEntryRejectsNonAgents(t *testing.T) {
	t.Parallel()
	entry := catalogEntry(t, `{"identifier":"urn:mcp","type":"application/mcp-server-card+json","url":"https://x.example"}`)
	if _, err := FromEntry("mcp", "", entry, testNow); err == nil {
		t.Fatalf("FromEntry() error = nil, want an error for an MCP entry")
	}
}

func TestFromEntryNullExtensions(t *testing.T) {
	t.Parallel()
	c := mustFromEntry(t, "fin", `{"identifier":"urn:x","type":"application/a2a-agent-card+json","url":"https://x.example/card.json","extensions":null}`)
	if c.Name != "fin" {
		t.Fatalf("FromEntry() name = %q, want %q", c.Name, "fin")
	}
}

func TestContactURLs(t *testing.T) {
	t.Parallel()
	subject := `,"trustManifest":{"identity":"did:web:acme.com","signature":"e30..c2ln","subject":{"identifier":"urn:x","type":"application/a2a-agent-card+json","url":"agents/x.json"}}`

	tests := []struct {
		name        string
		contact     func(t *testing.T) Contact
		wantCardURL string
		wantOrigin  string
	}{
		{
			name: "relative entry URL made absolute",
			contact: func(t *testing.T) Contact {
				return mustFromEntry(t, "x", `{"identifier":"urn:x","type":"application/a2a-agent-card+json","url":"agents/x.json"}`)
			},
			wantCardURL: "https://acme.com/agents/x.json",
			wantOrigin:  "https://acme.com",
		},
		{
			name: "relative entry URL bound by the trust manifest made absolute",
			contact: func(t *testing.T) Contact {
				return mustFromEntry(t, "x", `{"identifier":"urn:x","type":"application/a2a-agent-card+json","url":"agents/x.json"`+subject+`}`)
			},
			wantCardURL: "https://acme.com/agents/x.json",
			wantOrigin:  "https://acme.com",
		},
		{
			name: "http card",
			contact: func(t *testing.T) Contact {
				return mustFromCard(t, "x", "http://127.0.0.1:8080/.well-known/agent-card.json")
			},
			wantCardURL: "http://127.0.0.1:8080/.well-known/agent-card.json",
			wantOrigin:  "http://127.0.0.1:8080",
		},
		{
			name:        "file card",
			contact:     func(t *testing.T) Contact { return mustFromCard(t, "x", "file:///tmp/card.json") },
			wantCardURL: "file:///tmp/card.json",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := tt.contact(t)
			b := mustLoad(t, filepath.Join(t.TempDir(), "catalog.json"))
			if err := b.Add(c, false); err != nil {
				t.Fatalf("Book.Add() error = %v", err)
			}
			if err := b.Save(); err != nil {
				t.Fatalf("Book.Save() error = %v", err)
			}
			loaded, _ := mustLoad(t, b.Path()).Get("x")
			for _, got := range []Contact{c, loaded} {
				if got.Entry.URL != tt.wantCardURL || got.CardURL() != tt.wantCardURL || got.Origin() != tt.wantOrigin {
					t.Fatalf("Contact entry URL, CardURL(), Origin() = %q, %q, %q; want %q, %q, %q", got.Entry.URL, got.CardURL(), got.Origin(), tt.wantCardURL, tt.wantCardURL, tt.wantOrigin)
				}
			}
		})
	}
}

func TestBookAddConflicts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		contact Contact
		replace bool
		wantErr *ExistsError
		wantURL string
	}{
		{name: "name taken", contact: mustFromCard(t, "fin", "https://other.example"), wantErr: &ExistsError{Name: "fin"}},
		{name: "same agent under another name", contact: mustFromEntry(t, "money", signedEntry), wantErr: &ExistsError{Name: "fin", SameAgent: true}},
		{name: "same identifier with a version", contact: mustFromEntry(t, "money", strings.Replace(signedEntry, `"type"`, `"version":"2.0.0","type"`, 1)), wantErr: &ExistsError{Name: "fin", SameAgent: true}},
		{name: "replace", contact: mustFromCard(t, "fin", "https://other.example"), replace: true, wantURL: "https://other.example"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := mustLoad(t, filepath.Join(t.TempDir(), "catalog.json"))
			if err := b.Add(mustFromEntry(t, "fin", signedEntry), false); err != nil {
				t.Fatalf("Book.Add() error = %v", err)
			}

			err := b.Add(tt.contact, tt.replace)

			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("Book.Add() error = %v", err)
				}
				if got, _ := b.Get("fin"); got.CardURL() != tt.wantURL {
					t.Fatalf("Book.Get(fin) CardURL() = %v, want %v", got.CardURL(), tt.wantURL)
				}
				return
			}
			var exists *ExistsError
			if !errors.As(err, &exists) {
				t.Fatalf("Book.Add() error = %v, want *ExistsError", err)
			}
			if diff := cmp.Diff(tt.wantErr, exists); diff != "" {
				t.Fatalf("Book.Add() wrong result (-want +got) diff = %s", diff)
			}
		})
	}
}

func TestBookRemove(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "catalog.json")
	b := mustLoad(t, path)
	for _, name := range []string{"a", "b", "c"} {
		if err := b.Add(mustFromCard(t, name, "https://"+name+".example"), false); err != nil {
			t.Fatalf("Book.Add() error = %v", err)
		}
	}
	if err := b.Remove("a"); err != nil {
		t.Fatalf("Book.Remove() error = %v", err)
	}
	var notFound *NotFoundError
	if err := b.Remove("a"); !errors.As(err, &notFound) {
		t.Fatalf("Book.Remove() error = %v, want *NotFoundError", err)
	}
	if err := b.Save(); err != nil {
		t.Fatalf("Book.Save() error = %v", err)
	}
	loaded := mustLoad(t, path)
	if diff := cmp.Diff([]string{"b", "c"}, names(loaded.List())); diff != "" {
		t.Fatalf("Book.List() wrong result (-want +got) diff = %s", diff)
	}
	if c, _ := loaded.Get("c"); c.CardURL() != "https://c.example" {
		t.Fatalf("Book.Get(c) CardURL() = %v, want https://c.example", c.CardURL())
	}
}

func TestBookKeepsForeignContent(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "catalog.json")
	content := `{"specVersion":"1.0","host":{"displayName":"mine"},"entries":[` +
		`{"identifier":"urn:other","type":"application/mcp-server-card+json","url":"https://x.example"}],"extensions":{"com.example":1}}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	b := mustLoad(t, path)
	if err := b.Add(mustFromCard(t, "a", "https://a.example"), false); err != nil {
		t.Fatalf("Book.Add() error = %v", err)
	}
	if err := b.Save(); err != nil {
		t.Fatalf("Book.Save() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	for _, want := range []string{`"urn:other"`, `"com.example": 1`, `"displayName": "mine"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("saved book = %s, want it to keep %s", data, want)
		}
	}
}

func TestLoadInvalid(t *testing.T) {
	t.Parallel()
	contact := func(name, typ string) string {
		return `{"identifier":"urn:` + name + `","type":"` + typ + `","url":"https://x","extensions":{"org.a2aproject.a2a-cli":{"name":"` + name + `"}}}`
	}
	tests := []struct {
		name    string
		content string
	}{
		{name: "not JSON", content: "contacts:\n  a: {}\n"},
		{name: "unknown major version", content: `{"specVersion":"2.0","entries":[]}`},
		{name: "invalid contact name", content: `{"specVersion":"1.0","entries":[` + contact("Bad Name", catalog.MediaTypeA2AAgentCard) + `]}`},
		{name: "contact that is not an agent", content: `{"specVersion":"1.0","entries":[` + contact("a", "application/json") + `]}`},
		{name: "duplicate name", content: `{"specVersion":"1.0","entries":[` + contact("a", catalog.MediaTypeA2AAgentCard) + `,` + contact("a", catalog.MediaTypeA2AAgentCard) + `]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "catalog.json")
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatalf("os.WriteFile() error = %v", err)
			}
			if _, err := Load(path); err == nil {
				t.Fatalf("Load() error = nil, want an error")
			}
		})
	}
}
