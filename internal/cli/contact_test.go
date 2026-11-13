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
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Agent-Card/ai-catalog-go/catalog"
	"github.com/google/go-cmp/cmp"

	"github.com/a2aproject/a2a-cli/internal/cardcache"
	"github.com/a2aproject/a2a-cli/internal/clicfg"
	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-cli/internal/flagparse"
	"github.com/a2aproject/a2a-cli/internal/output"
	"github.com/a2aproject/a2a-cli/internal/polling"
	"github.com/a2aproject/a2a-cli/internal/testutil"
	"github.com/a2aproject/a2a-go/v2/a2a"
)

// contactEnv runs commands against an isolated contact book, AI Catalog store
// and card cache.
type contactEnv struct {
	t            *testing.T
	contactsFile string
	catalogDir   string
	cacheDir     string
	spawned      [][]string
}

func newContactEnv(t *testing.T) *contactEnv {
	t.Helper()
	dir := t.TempDir()
	return &contactEnv{
		t:            t,
		contactsFile: filepath.Join(dir, "config", "catalog.json"),
		catalogDir:   filepath.Join(dir, "ai-catalog"),
		cacheDir:     filepath.Join(dir, "cache"),
	}
}

func (e *contactEnv) run(stdin string, args ...string) (stdout, stderr string, err error) {
	e.t.Helper()
	var out, errOut bytes.Buffer
	cfg := &globalConfig{
		Printer:   output.NewPrinter(&out, output.ModeText),
		svcParams: &flagparse.ServiceParams{},
		errOut:    &errOut,
	}
	spawn := func(args []string) error {
		e.spawned = append(e.spawned, args)
		return nil
	}
	root := mustNewRoot(e.t, cfg, deps{
		poller:        polling.Stream,
		cfgLoader:     clicfg.LoadEmpty,
		spawnDetached: spawn,
		catalogDir:    e.catalogDir,
		contactsFile:  e.contactsFile,
		cacheDir:      e.cacheDir,
	})
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(args)
	err = root.Execute()
	return out.String(), errOut.String(), err
}

func (e *contactEnv) mustRun(args ...string) string {
	e.t.Helper()
	out, stderr, err := e.run("", args...)
	if err != nil {
		e.t.Fatalf("run(%v) error = %v, stderr = %s", args, err, stderr)
	}
	return out
}

func (e *contactEnv) contacts() []contactView {
	e.t.Helper()
	var views []contactView
	if err := json.Unmarshal([]byte(e.mustRun("contact", "list", "-o", "json")), &views); err != nil {
		e.t.Fatalf("json.Unmarshal(contact list) error = %v", err)
	}
	return views
}

func (e *contactEnv) searchIdentifiers(args ...string) []string {
	e.t.Helper()
	var agents []catalogAgentView
	out := e.mustRun(append([]string{"catalog", "search", "-o", "json"}, args...)...)
	if err := json.Unmarshal([]byte(out), &agents); err != nil {
		e.t.Fatalf("json.Unmarshal(catalog search) error = %v", err)
	}
	ids := []string{}
	for _, a := range agents {
		ids = append(ids, a.Catalog+" "+a.Identifier)
	}
	return ids
}

func TestContactAddListGetRemove(t *testing.T) {
	t.Parallel()
	agentURL := startTestServer(t)
	cardURL := agentURL + "/.well-known/agent-card.json"
	env := newContactEnv(t)

	if out := env.mustRun("contact", "list"); !strings.HasPrefix(out, "No contacts") {
		t.Fatalf("contact list = %q, want the empty book message", out)
	}
	out := env.mustRun("contact", "add", "echo", agentURL, "--note", "test agent")
	if want := "Added contact echo: Test Echo (urn:ai-catalog:a2a-cli:echo)\n"; out != want {
		t.Fatalf("contact add = %q, want %q", out, want)
	}

	want := []contactView{{
		Name:        "echo",
		Identifier:  "urn:ai-catalog:a2a-cli:echo",
		DisplayName: "Test Echo",
		URL:         cardURL,
		Note:        "test agent",
		Source:      cardURL,
	}}
	if diff := cmp.Diff(want, env.contacts(), cmp.FilterPath(func(p cmp.Path) bool { return p.Last().String() == ".AddedAt" }, cmp.Ignore())); diff != "" {
		t.Fatalf("contact list wrong result (-want +got) diff = %s", diff)
	}
	if _, err := catalog.ParseFile(env.contactsFile); err != nil {
		t.Fatalf("catalog.ParseFile(contacts) error = %v, want the contact book to be an AI Catalog", err)
	}
	if diff := cmp.Diff([]string{"a2a-contacts urn:ai-catalog:a2a-cli:echo"}, env.searchIdentifiers()); diff != "" {
		t.Fatalf("catalog search wrong result (-want +got) diff = %s", diff)
	}

	if out := env.mustRun("contact", "get", "echo"); !strings.Contains(out, "Contact:     echo") || !strings.Contains(out, "Test Echo") {
		t.Fatalf("contact get = %q, want contact details and card", out)
	}
	if out := env.mustRun("send", "-a", "echo", "hello contact"); !strings.Contains(out, "hello contact") {
		t.Fatalf("send -a echo = %q, want the echoed message", out)
	}
	if out := env.mustRun("card", "get", "echo"); !strings.Contains(out, "Test Echo") {
		t.Fatalf("card get echo = %q, want the contact's card", out)
	}

	if out := env.mustRun("contact", "remove", "echo"); out != "Removed contact echo\n" {
		t.Fatalf("contact remove = %q, want confirmation", out)
	}
	if got := env.contacts(); len(got) != 0 {
		t.Fatalf("contact list after remove = %v, want empty", got)
	}
	if got := env.searchIdentifiers(); len(got) != 0 {
		t.Fatalf("catalog search after remove = %v, want the registered contact book updated", got)
	}
}

func TestContactFromCatalog(t *testing.T) {
	t.Parallel()
	agentURL := startTestServer(t)
	cardURL := agentURL + "/.well-known/agent-card.json"
	signed := `{"identifier":"urn:air:test:agent:echo","type":"application/a2a-agent-card+json","url":"` + cardURL + `",` +
		`"publisher":{"identifier":"did:web:test","displayName":"Test"},"trustManifest":{"identity":"did:web:test","signature":"e30..c2ln"}}`
	catalogURL := startCatalogServer(t, map[string]string{
		"/corp.json":        agentCatalog(signed),
		"/lab.json":         agentCatalog(agentCatalogEntry("urn:air:test:agent:echo", cardURL)),
		"/remote.json":      agentCatalog(agentCatalogEntry("urn:air:test:agent:remote", "agents/card.json")),
		"/agents/card.json": mustMarshalCard(t, newAgentCard(agentURL, a2a.AgentCapabilities{})),
	})
	env := newContactEnv(t)
	env.mustRun("catalog", "add", "corp", catalogURL+"/corp.json")
	env.mustRun("catalog", "add", "lab", catalogURL+"/lab.json")

	_, _, err := env.run("", "send", "-a", "urn:air:test:agent:echo", "hi")
	if _, ok := errors.AsType[*clierr.Error](err); !ok || !strings.Contains(err.Error(), "more than one catalog") {
		t.Fatalf("send -a <identifier in two catalogs> error = %v, want the ambiguity error", err)
	}

	env.mustRun("contact", "add", "echo", "--catalog", "corp", "urn:air:test:agent:echo")

	got := env.contacts()
	if len(got) != 1 || got[0].Identifier != "urn:air:test:agent:echo" || got[0].Source != catalogURL+"/corp.json" {
		t.Fatalf("contact list = %+v, want the corp entry", got)
	}
	data, err := os.ReadFile(env.contactsFile)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	for _, want := range []string{`"did:web:test"`, `"signature": "e30..c2ln"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("contact book = %s, want the entry's publisher and trust manifest kept (%s)", data, want)
		}
	}

	for _, ref := range []string{"echo", "urn:air:test:agent:echo"} {
		out := env.mustRun("send", "-o", "json", "-a", ref, "hello")
		if got := testutil.AllArtifactText(mustDecodeTask(t, out)); got != "hello" {
			t.Fatalf("send -a %s artifact text = %q, want %q", ref, got, "hello")
		}
	}

	_, _, err = env.run("", "contact", "add", "copy", "--catalog", "lab", "urn:air:test:agent:echo")
	if ce, ok := errors.AsType[*clierr.Error](err); !ok || ce.Code != clierr.CodeUsage || !strings.Contains(ce.Message, `already a contact named "echo"`) {
		t.Fatalf("contact add <same agent> error = %v, want a usage error naming the existing contact", err)
	}

	env.mustRun("contact", "add", "remote", "--catalog", catalogURL+"/remote.json", "urn:air:test:agent:remote")
	if c := env.contacts()[1]; c.Name != "remote" || c.URL != catalogURL+"/agents/card.json" {
		t.Fatalf("contact remote = %+v, want its relative URL made absolute", c)
	}
}

func TestAgentRefWithUnreadableContactBook(t *testing.T) {
	t.Parallel()
	agentURL := startTestServer(t)
	catalogURL := startCatalogServer(t, map[string]string{
		"/corp.json": agentCatalog(agentCatalogEntry("urn:air:test:agent:echo", agentURL+"/.well-known/agent-card.json")),
	})
	env := newContactEnv(t)
	env.mustRun("catalog", "add", "corp", catalogURL+"/corp.json")
	if err := os.MkdirAll(filepath.Dir(env.contactsFile), 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(env.contactsFile, []byte("{bad"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	out, stderr, err := env.run("", "send", "-o", "json", "-a", "urn:air:test:agent:echo", "hello")
	if err != nil {
		t.Fatalf("send -a <identifier> error = %v, want the catalog agent despite the unreadable contact book", err)
	}
	if got := testutil.AllArtifactText(mustDecodeTask(t, out)); got != "hello" {
		t.Fatalf("send -a <identifier> artifact text = %q, want %q", got, "hello")
	}
	if !strings.Contains(stderr, "warning: ignoring contacts") {
		t.Fatalf("send -a <identifier> stderr = %q, want a warning about the contact book", stderr)
	}
}

func TestContactCardCacheTTL(t *testing.T) {
	t.Parallel()
	agentURL := startTestServer(t)
	cardURL := agentURL + "/.well-known/agent-card.json"

	tests := []struct {
		name     string
		cachedAt time.Time
		wantName string
	}{
		{name: "fresh card read from cache", cachedAt: time.Now().Add(-time.Hour), wantName: "Cached Echo"},
		{name: "stale card fetched again", cachedAt: time.Now().Add(-25 * time.Hour), wantName: "Test Echo"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := newContactEnv(t)
			env.mustRun("contact", "add", "echo", agentURL)
			cache := cardcache.New(env.cacheDir)
			if err := cache.Put(cardcache.Entry{Ref: cardURL, FetchedAt: tc.cachedAt, Card: &a2a.AgentCard{Name: "Cached Echo"}}); err != nil {
				t.Fatalf("Cache.Put() error = %v", err)
			}

			var got contactDetailView
			if err := json.Unmarshal([]byte(env.mustRun("contact", "get", "echo", "-o", "json")), &got); err != nil {
				t.Fatalf("json.Unmarshal(contact get) error = %v", err)
			}
			if got.Card.Name != tc.wantName {
				t.Fatalf("contact get card name = %q, want %q", got.Card.Name, tc.wantName)
			}
		})
	}
}

func TestSendToContactRefreshesCachedCard(t *testing.T) {
	t.Parallel()
	agentURL := startTestServer(t)
	cardURL := agentURL + "/.well-known/agent-card.json"
	env := newContactEnv(t)
	env.mustRun("contact", "add", "echo", agentURL)
	cache := cardcache.New(env.cacheDir)
	if err := cache.Put(cardcache.Entry{Ref: cardURL, FetchedAt: time.Now().Add(-48 * time.Hour), Card: &a2a.AgentCard{Name: "Cached Echo"}}); err != nil {
		t.Fatalf("Cache.Put() error = %v", err)
	}

	env.mustRun("send", "-a", "echo", "hello")

	e, ok := cache.Get(cardURL)
	if !ok || !e.Found() || e.Card.Name != "Test Echo" || time.Since(e.FetchedAt) > time.Minute {
		t.Fatalf("Cache.Get() = %+v, %v; want the card fetched by send", e, ok)
	}
}

func TestContactErrors(t *testing.T) {
	t.Parallel()
	agentURL := startTestServer(t)

	tests := []struct {
		name     string
		seed     [][]string
		args     []string
		wantCode string
		wantMsg  string
	}{
		{name: "invalid name", args: []string{"contact", "add", "Bad.Name", agentURL}, wantCode: clierr.CodeUsage, wantMsg: "invalid contact name"},
		{name: "duplicate", seed: [][]string{{"contact", "add", "echo", agentURL}}, args: []string{"contact", "add", "echo", agentURL}, wantCode: clierr.CodeUsage, wantMsg: `contact "echo" already exists`},
		{name: "same agent", seed: [][]string{{"contact", "add", "echo", agentURL}}, args: []string{"contact", "add", "echo2", "urn:ai-catalog:a2a-cli:echo"}, wantCode: clierr.CodeUsage, wantMsg: `already a contact named "echo"`},
		{name: "get unknown", args: []string{"contact", "get", "nope"}, wantCode: clierr.CodeUsage, wantMsg: `no contact named "nope"`},
		{name: "remove unknown", args: []string{"contact", "remove", "nope"}, wantCode: clierr.CodeUsage, wantMsg: `no contact named "nope"`},
		{name: "unreachable agent", args: []string{"contact", "add", "ghost", "http://127.0.0.1:1", "--timeout", "2s"}, wantCode: clierr.CodeIO},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := newContactEnv(t)
			for _, args := range tc.seed {
				env.mustRun(args...)
			}
			before := env.contacts()

			_, _, err := env.run("", tc.args...)

			ce, ok := errors.AsType[*clierr.Error](clierr.Classify(err))
			if !ok || ce.Code != tc.wantCode || !strings.Contains(ce.Message, tc.wantMsg) {
				t.Fatalf("run(%v) error = %v, want code %s containing %q", tc.args, err, tc.wantCode, tc.wantMsg)
			}
			if diff := cmp.Diff(before, env.contacts()); diff != "" {
				t.Fatalf("contact book changed after a failed command (-want +got) diff = %s", diff)
			}
		})
	}
}

func TestContactSavedWhenStoreIsUnsupported(t *testing.T) {
	t.Parallel()
	agentURL := startTestServer(t)
	env := newContactEnv(t)
	if err := os.MkdirAll(env.catalogDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(env.catalogDir, "catalog.json"), []byte(`{"specVersion":"9.0"}`), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	_, stderr, err := env.run("", "contact", "add", "echo", agentURL)
	if err != nil {
		t.Fatalf("contact add error = %v, want the contact saved", err)
	}
	if !strings.Contains(stderr, "unrecognized AI Catalog store format") {
		t.Fatalf("contact add stderr = %q, want a warning about the store", stderr)
	}
	if got := env.contacts(); len(got) != 1 {
		t.Fatalf("contact list = %v, want the contact saved", got)
	}
}

func TestHookCmd(t *testing.T) {
	t.Parallel()
	agentURL := startTestServer(t)
	env := newContactEnv(t)
	env.mustRun("contact", "add", "echo", agentURL, "--note", "says it back")

	out, _, err := env.run(`{"hook_event_name":"SessionStart","session_id":"s1","source":"startup"}`, "hook")
	if err != nil {
		t.Fatalf("hook(SessionStart) error = %v", err)
	}
	for _, want := range []string{`"hookEventName":"SessionStart"`, "- echo — Test Echo", "User note: says it back"} {
		if !strings.Contains(out, want) {
			t.Fatalf("hook(SessionStart) = %q, want it to contain %q", out, want)
		}
	}

	out, _, err = env.run(`{"hook_event_name":"UserPromptSubmit","session_id":"s1","prompt":"look at https://new.example/x and `+agentURL+`"}`, "hook", "--discovery-allow-host", "new.example")
	if err != nil {
		t.Fatalf("hook(UserPromptSubmit) error = %v", err)
	}
	if out != "" {
		t.Fatalf("hook(UserPromptSubmit) = %q, want no output before the probe completes", out)
	}
	want := [][]string{{"hook", "probe", "--discovery-allow-host", "new.example", "--", "https://new.example"}}
	if diff := cmp.Diff(want, env.spawned); diff != "" {
		t.Fatalf("hook(UserPromptSubmit) spawned wrong probe (-want +got) diff = %s", diff)
	}
}

func TestHookCmdDiscoveryDisabled(t *testing.T) {
	t.Parallel()
	env := newContactEnv(t)

	out, _, err := env.run(`{"hook_event_name":"UserPromptSubmit","session_id":"s1","prompt":"https://new.example"}`, "hook", "--discovery=false")
	if err != nil {
		t.Fatalf("hook error = %v", err)
	}
	if out != "" || len(env.spawned) != 0 {
		t.Fatalf("hook --discovery=false = %q, spawned %v; want no output and no probe", out, env.spawned)
	}
}

func TestHookCmdInvalidInputIsSilent(t *testing.T) {
	t.Parallel()
	env := newContactEnv(t)

	out, _, err := env.run("not json", "hook")
	if err != nil || out != "" {
		t.Fatalf("hook(not json) = %q, %v; want no output and no error", out, err)
	}
}
