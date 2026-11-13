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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/a2aproject/a2a-cli/internal/clicfg"
	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-cli/internal/polling"
	"github.com/a2aproject/a2a-cli/internal/testutil"
	"github.com/a2aproject/a2a-go/v2/a2a"
)

// startCatalogServer serves AI Catalog documents by path.
func startCatalogServer(t *testing.T, docs map[string]string) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		doc, ok := docs[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/ai-catalog+json")
		if _, err := w.Write([]byte(doc)); err != nil {
			t.Errorf("writing %s: %v", r.URL.Path, err)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func agentCatalog(entries ...string) string {
	return `{"specVersion":"1.0","entries":[` + strings.Join(entries, ",") + `]}`
}

func agentCatalogEntry(id, cardURL string) string {
	return fmt.Sprintf(`{"identifier":%q,"type":"application/a2a-agent-card+json","url":%q}`, id, cardURL)
}

func runCatalogCMD(t *testing.T, dir string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	return runCMDCapturingStderrWithConfig(t, deps{poller: polling.Stream, cfgLoader: clicfg.LoadEmpty, catalogDir: dir}, args...)
}

func mustRunCatalogCMD(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, stderr, err := runCatalogCMD(t, dir, args...)
	if err != nil {
		t.Fatalf("runCatalogCMD(%q) error = %v, stderr = %s", strings.Join(args, " "), err, stderr)
	}
	return out
}

func TestCatalogCommands(t *testing.T) {
	t.Parallel()
	agentURL := startTestServer(t)
	catalogURL := startCatalogServer(t, map[string]string{
		"/.well-known/ai-catalog.json": agentCatalog(
			agentCatalogEntry("urn:air:test:agent:echo", agentURL+"/.well-known/agent-card.json"),
			`{"identifier":"urn:air:test:mcp:tools","type":"application/mcp-server-card+json","url":"/mcp.json"}`,
			`{"identifier":"urn:air:test:catalog:lab","type":"application/ai-catalog+json","url":"/lab.json"}`,
		),
		"/lab.json": agentCatalog(`{"identifier":"urn:air:test:agent:lab","displayName":"Lab","description":"Experimental agent","type":"application/a2a-agent-card+json","data":{"name":"Lab"}}`),
	})
	dir := t.TempDir()

	out := mustRunCatalogCMD(t, dir, "catalog", "add", "test", catalogURL)
	if want := fmt.Sprintf("Added catalog %q from %s/.well-known/ai-catalog.json (3 entries)\n", "test", catalogURL); out != want {
		t.Fatalf("a2a catalog add = %q, want %q", out, want)
	}

	var catalogs []catalogView
	if err := json.Unmarshal([]byte(mustRunCatalogCMD(t, dir, "catalog", "list", "-o", "json")), &catalogs); err != nil {
		t.Fatalf("json.Unmarshal(catalog list) error = %v", err)
	}
	wantCatalogs := []catalogView{{Name: "test", Source: catalogURL + "/.well-known/ai-catalog.json", Entries: 3}}
	if diff := cmp.Diff(wantCatalogs, catalogs, cmpopts.IgnoreFields(catalogView{}, "LastUpdated")); diff != "" {
		t.Fatalf("a2a catalog list wrong result (-want +got) diff = %s", diff)
	}

	var agents []catalogAgentView
	if err := json.Unmarshal([]byte(mustRunCatalogCMD(t, dir, "catalog", "search", "-o", "json")), &agents); err != nil {
		t.Fatalf("json.Unmarshal(catalog search) error = %v", err)
	}
	wantAgents := []catalogAgentView{
		{Identifier: "urn:air:test:agent:echo", URL: agentURL + "/.well-known/agent-card.json", Catalog: "test"},
		{Identifier: "urn:air:test:agent:lab", DisplayName: "Lab", Description: "Experimental agent", Catalog: "test"},
	}
	if diff := cmp.Diff(wantAgents, agents); diff != "" {
		t.Fatalf("a2a catalog search wrong result (-want +got) diff = %s", diff)
	}

	out = mustRunCatalogCMD(t, dir, "catalog", "search", "experimental")
	if !strings.Contains(out, "urn:air:test:agent:lab") || strings.Contains(out, "urn:air:test:agent:echo") {
		t.Fatalf("a2a catalog search experimental = %q, want only the lab agent", out)
	}

	out = mustRunCatalogCMD(t, dir, "catalog", "update")
	if want := "Catalog \"test\" up to date\n"; out != want {
		t.Fatalf("a2a catalog update = %q, want %q", out, want)
	}

	out = mustRunCatalogCMD(t, dir, "catalog", "remove", "TEST")
	if want := "Removed catalog \"test\"\n"; out != want {
		t.Fatalf("a2a catalog remove = %q, want %q", out, want)
	}
	out = mustRunCatalogCMD(t, dir, "catalog", "list")
	if !strings.HasPrefix(out, "No catalogs registered") {
		t.Fatalf("a2a catalog list after remove = %q, want no catalogs", out)
	}
}

func TestCatalogCommandErrors(t *testing.T) {
	t.Parallel()
	catalogURL := startCatalogServer(t, map[string]string{"/c.json": agentCatalog()})

	tests := []struct {
		name     string
		args     []string
		wantCode string
	}{
		{name: "remove unknown", args: []string{"catalog", "remove", "nope"}, wantCode: clierr.CodeUsage},
		{name: "update unknown", args: []string{"catalog", "update", "nope"}, wantCode: clierr.CodeUsage},
		{name: "search unknown catalog", args: []string{"catalog", "search", "--catalog", "nope"}, wantCode: clierr.CodeUsage},
		{name: "add without source", args: []string{"catalog", "add", "x"}, wantCode: clierr.CodeUsage},
		{name: "add missing catalog", args: []string{"catalog", "add", "x", catalogURL + "/missing.json"}, wantCode: clierr.CodeInternal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := runCatalogCMD(t, t.TempDir(), tt.args...)
			if got := clierr.Classify(err); got == nil || got.Code != tt.wantCode {
				t.Fatalf("a2a %s error = %v, want code %s", strings.Join(tt.args, " "), err, tt.wantCode)
			}
		})
	}
}

func TestAgentCardFromCatalog(t *testing.T) {
	t.Parallel()
	agentURL := startTestServer(t)
	cardURL := agentURL + "/.well-known/agent-card.json"
	catalogURL := startCatalogServer(t, map[string]string{
		"/corp.json":   agentCatalog(agentCatalogEntry("urn:air:test:agent:echo", cardURL), agentCatalogEntry("urn:air:test:agent:other", cardURL)),
		"/lab.json":    agentCatalog(agentCatalogEntry("urn:air:test:agent:lab", cardURL), agentCatalogEntry("urn:air:test:agent:other", cardURL)),
		"/single.json": agentCatalog(agentCatalogEntry("urn:air:test:agent:single", cardURL), `{"identifier":"urn:x","type":"application/mcp-server-card+json","url":"/x"}`),
		"/inline.json": agentCatalog(fmt.Sprintf(`{"identifier":"urn:air:test:agent:inline","type":"application/a2a-agent-card+json","data":%s}`,
			mustMarshalCard(t, newAgentCard(agentURL, a2a.AgentCapabilities{Streaming: true})))),
	})
	dir := t.TempDir()
	mustRunCatalogCMD(t, dir, "catalog", "add", "corp", catalogURL+"/corp.json")
	mustRunCatalogCMD(t, dir, "catalog", "add", "lab", catalogURL+"/lab.json")

	tests := []struct {
		name string
		args []string
	}{
		{name: "identifier across registered catalogs", args: []string{"-a", "urn:air:test:agent:echo"}},
		{name: "identifier in a registered catalog", args: []string{"--catalog", "LAB", "-a", "urn:air:test:agent:other"}},
		{name: "identifier in an unregistered catalog", args: []string{"--catalog", catalogURL + "/single.json", "-a", "urn:air:test:agent:single"}},
		{name: "catalog with a single agent", args: []string{"-a", catalogURL + "/single.json"}},
		{name: "card embedded in the catalog", args: []string{"--catalog", catalogURL + "/inline.json", "-a", "urn:air:test:agent:inline"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out := mustRunCatalogCMD(t, dir, append([]string{"send", "-o", "json", "hello"}, tt.args...)...)
			task := mustDecodeTask(t, out)
			if got := testutil.AllArtifactText(task); got != "hello" {
				t.Fatalf("a2a send %v artifact text = %q, want %q", tt.args, got, "hello")
			}
		})
	}

	t.Run("card get with positional identifier", func(t *testing.T) {
		t.Parallel()
		out := mustRunCatalogCMD(t, dir, "card", "get", "urn:air:test:agent:echo", "-o", "json")
		var card a2a.AgentCard
		if err := json.Unmarshal([]byte(out), &card); err != nil {
			t.Fatalf("json.Unmarshal(card get output) error = %v", err)
		}
		if card.Name != "Test Echo" {
			t.Fatalf("a2a card get card.Name = %q, want %q", card.Name, "Test Echo")
		}
	})
}

func mustMarshalCard(t *testing.T, card *a2a.AgentCard) string {
	t.Helper()
	data, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("json.Marshal(card) error = %v", err)
	}
	return string(data)
}

func TestAgentCardFromCatalogErrors(t *testing.T) {
	t.Parallel()
	catalogURL := startCatalogServer(t, map[string]string{
		"/corp.json": agentCatalog(agentCatalogEntry("urn:air:test:agent:a", "/a.json"), agentCatalogEntry("urn:air:test:agent:shared", "/s.json")),
		"/lab.json":  agentCatalog(agentCatalogEntry("urn:air:test:agent:shared", "/s.json")),
		"/none.json": agentCatalog(`{"identifier":"urn:x","type":"application/mcp-server-card+json","url":"/x"}`),
	})
	dir := t.TempDir()
	mustRunCatalogCMD(t, dir, "catalog", "add", "corp", catalogURL+"/corp.json")
	mustRunCatalogCMD(t, dir, "catalog", "add", "lab", catalogURL+"/lab.json")

	tests := []struct {
		name     string
		dir      string
		args     []string
		wantCode string
		wantMsg  string
		wantHint string
	}{
		{
			name:     "unknown identifier lists the agents",
			dir:      dir,
			args:     []string{"-a", "urn:air:test:agent:nope"},
			wantCode: clierr.CodeCardInvalid,
			wantMsg:  `no A2A agent "urn:air:test:agent:nope" in the registered catalogs`,
			wantHint: "A2A agents in the registered catalogs: urn:air:test:agent:a, urn:air:test:agent:shared",
		},
		{
			name:     "no catalogs registered",
			dir:      t.TempDir(),
			args:     []string{"-a", "urn:air:test:agent:a"},
			wantCode: clierr.CodeCardInvalid,
			wantHint: "a2a catalog add",
		},
		{
			name:     "identifier in two catalogs",
			dir:      dir,
			args:     []string{"-a", "urn:air:test:agent:shared"},
			wantCode: clierr.CodeCardInvalid,
			wantMsg:  "listed by more than one catalog: corp, lab",
			wantHint: "--catalog",
		},
		{
			name:     "not an agent",
			dir:      dir,
			args:     []string{"--catalog", catalogURL + "/none.json", "-a", "urn:x"},
			wantCode: clierr.CodeCardInvalid,
			wantMsg:  "application/mcp-server-card+json entry, not an A2A agent",
		},
		{
			name:     "catalog with several agents",
			dir:      dir,
			args:     []string{"-a", catalogURL + "/corp.json"},
			wantCode: clierr.CodeCardInvalid,
			wantMsg:  "AI Catalog with 2 A2A agents",
			wantHint: "urn:air:test:agent:a, urn:air:test:agent:shared",
		},
		{
			name:     "catalog with no agents",
			dir:      dir,
			args:     []string{"-a", catalogURL + "/none.json"},
			wantCode: clierr.CodeCardInvalid,
			wantMsg:  "AI Catalog with no A2A agents",
		},
		{
			name:     "unknown catalog name",
			dir:      dir,
			args:     []string{"--catalog", "nope", "-a", "urn:air:test:agent:a"},
			wantCode: clierr.CodeUsage,
			wantHint: "a2a catalog list",
		},
		{
			name:     "unregistered catalog not found",
			dir:      dir,
			args:     []string{"--catalog", catalogURL + "/missing.json", "-a", "urn:air:test:agent:a"},
			wantCode: clierr.CodeCardInvalid,
			wantMsg:  "404",
		},
		{
			name:     "catalog with endpoint",
			dir:      dir,
			args:     []string{"--catalog", "corp", "-e", "http://127.0.0.1:1", "--transport", "rest"},
			wantCode: clierr.CodeUsage,
		},
		{
			name:     "catalog without agent card",
			dir:      dir,
			args:     []string{"--catalog", "corp"},
			wantCode: clierr.CodeUsage,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := runCatalogCMD(t, tt.dir, append([]string{"send", "hello"}, tt.args...)...)
			var ce *clierr.Error
			if !errors.As(err, &ce) {
				t.Fatalf("a2a send %v error = %v, want *clierr.Error", tt.args, err)
			}
			if ce.Code != tt.wantCode {
				t.Fatalf("a2a send %v Code = %v, want %v (error: %v)", tt.args, ce.Code, tt.wantCode, err)
			}
			if !strings.Contains(ce.Message, tt.wantMsg) {
				t.Fatalf("a2a send %v Message = %q, want containing %q", tt.args, ce.Message, tt.wantMsg)
			}
			if !strings.Contains(ce.Hint, tt.wantHint) {
				t.Fatalf("a2a send %v Hint = %q, want containing %q", tt.args, ce.Hint, tt.wantHint)
			}
		})
	}
}
