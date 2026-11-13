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

package discover

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/a2aproject/a2a-cli/internal/cardcache"
	"github.com/a2aproject/a2a-go/v2/a2a"
)

var testNow = time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)

type step struct {
	event string
	// cache entries are stored before the event runs.
	cache    []cardcache.Entry
	advance  time.Duration
	contacts []ContactEntry
	// want are substrings of the injected context; nil wants no output.
	want       []string
	wantAbsent []string
	wantProbe  []string
}

type hookEnv struct {
	t      *testing.T
	cards  *cardcache.Cache
	dir    string
	now    time.Time
	probed [][]string
}

func newHookEnv(t *testing.T) *hookEnv {
	t.Helper()
	dir := t.TempDir()
	return &hookEnv{t: t, cards: cardcache.New(dir), dir: dir, now: testNow}
}

func (e *hookEnv) hook(list []ContactEntry, discovery bool) *Hook {
	return &Hook{
		Contacts:  list,
		Cards:     e.cards,
		StateDir:  filepath.Join(e.dir, "sessions"),
		Discovery: discovery,
		Probe: func(origins []string) error {
			e.probed = append(e.probed, origins)
			return nil
		},
		Now: func() time.Time { return e.now },
	}
}

func (e *hookEnv) run(h *Hook, event string) string {
	e.t.Helper()
	out, err := h.Run([]byte(event))
	if err != nil {
		e.t.Fatalf("Hook.Run(%s) error = %v", event, err)
	}
	if out == "" {
		return ""
	}
	var parsed hookOutput
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		e.t.Fatalf("Hook.Run(%s) = %q, not hook JSON: %v", event, out, err)
	}
	return parsed.HookSpecificOutput.AdditionalContext
}

func card(name string) *a2a.AgentCard {
	return &a2a.AgentCard{
		Name:                name,
		Description:         name + " description",
		SupportedInterfaces: []*a2a.AgentInterface{a2a.NewAgentInterface("https://agent.example", a2a.TransportProtocolHTTPJSON)},
		Skills:              []a2a.AgentSkill{{ID: "s1", Name: "skill-one"}},
	}
}

func found(origin, name string, at time.Time) cardcache.Entry {
	return cardcache.Entry{Ref: origin, FetchedAt: at, Card: card(name)}
}

func notFound(origin string, at time.Time) cardcache.Entry {
	return cardcache.Entry{Ref: origin, FetchedAt: at, Error: "404"}
}

func start(source string) string {
	return `{"hook_event_name":"SessionStart","session_id":"s1","source":"` + source + `"}`
}

func prompt(text string) string {
	return `{"hook_event_name":"UserPromptSubmit","session_id":"s1","prompt":"` + text + `"}`
}

var nimbus = ContactEntry{Name: "nimbus", Origin: "https://nimbus.example", Note: "coffee orders", Card: card("Nimbus")}

var uncachedNimbus = ContactEntry{Name: "nimbus", Origin: "https://nimbus.example", DisplayName: "Nimbus", Description: "Coffee subscriptions"}

func TestHookRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		discovery bool
		steps     []step
	}{
		{
			name: "contacts emptied while away reported on resume",
			steps: []step{
				{event: start("startup"), contacts: []ContactEntry{nimbus}, want: []string{"- nimbus"}},
				{event: start("resume"), want: []string{"contact book is now empty"}},
				{event: prompt("hello")},
			},
		},
		{
			name: "contacts emptied before compaction reported",
			steps: []step{
				{event: start("startup"), contacts: []ContactEntry{nimbus}, want: []string{"- nimbus"}},
				{event: start("compact"), want: []string{"contact book is now empty"}},
				{event: prompt("hello")},
			},
		},
		{
			name: "empty contact book stays silent on compaction",
			steps: []step{
				{event: start("startup")},
				{event: start("compact")},
			},
		},
		{
			name:      "compaction skips discovered agents that became contacts",
			discovery: true,
			steps: []step{
				{event: prompt("https://nimbus.example"), cache: []cardcache.Entry{found(nimbus.Origin, "Nimbus", testNow)}, want: []string{"auto-discovered"}},
				{event: start("compact"), contacts: []ContactEntry{nimbus}, want: []string{"A2A contacts"}, wantAbsent: []string{"auto-discovered"}},
			},
		},
		{
			name: "contacts injected on startup but not on resume",
			steps: []step{
				{event: start("startup"), contacts: []ContactEntry{nimbus}, want: []string{"A2A contacts", "- nimbus — Nimbus: Nimbus description", "User note: coffee orders", "Skills: skill-one"}},
				{event: start("resume"), contacts: []ContactEntry{nimbus}},
			},
		},
		{
			name: "contacts reinjected after compaction",
			steps: []step{
				{event: start("startup"), contacts: []ContactEntry{uncachedNimbus}, want: []string{"- nimbus — Nimbus: Coffee subscriptions (card not cached"}},
				{event: start("compact"), contacts: []ContactEntry{nimbus}, want: []string{"- nimbus"}},
			},
		},
		{
			name: "changed contacts reinjected on the next prompt",
			steps: []step{
				{event: start("startup")},
				{event: prompt("hello"), contacts: []ContactEntry{nimbus}, want: []string{"contact book changed", "- nimbus"}},
				{event: prompt("hello"), contacts: []ContactEntry{nimbus}},
				{event: prompt("hello"), want: []string{"contact book is now empty"}},
				{event: prompt("hello")},
			},
		},
		{
			name:      "uncached origin probed in background and injected by a later event",
			discovery: true,
			steps: []step{
				{event: start("startup")},
				{event: prompt("see https://agent.example/page"), wantProbe: []string{"https://agent.example"}},
				{event: prompt("thanks"), cache: []cardcache.Entry{found("https://agent.example", "Agent", testNow)}, want: []string{"auto-discovered", "NOT verified", "- https://agent.example — Agent"}},
				{event: prompt("again https://agent.example")},
			},
		},
		{
			name:      "pending origin not probed again within the retry interval",
			discovery: true,
			steps: []step{
				{event: prompt("https://agent.example"), wantProbe: []string{"https://agent.example"}},
				{event: prompt("https://agent.example"), advance: 30 * time.Second},
				{event: prompt("https://agent.example"), advance: time.Minute, wantProbe: []string{"https://agent.example"}},
			},
		},
		{
			name:      "fresh negative entry not probed",
			discovery: true,
			steps: []step{
				{event: prompt("https://site.example"), cache: []cardcache.Entry{notFound("https://site.example", testNow)}},
			},
		},
		{
			name:      "stale negative entry probed again",
			discovery: true,
			steps: []step{
				{event: prompt("https://site.example"), cache: []cardcache.Entry{notFound("https://site.example", testNow.Add(-7*time.Hour))}, wantProbe: []string{"https://site.example"}},
			},
		},
		{
			name:      "cached agent injected immediately and stale card refreshed",
			discovery: true,
			steps: []step{
				{event: prompt("https://agent.example"), cache: []cardcache.Entry{found("https://agent.example", "Agent", testNow.Add(-48*time.Hour))}, want: []string{"- https://agent.example — Agent"}, wantProbe: []string{"https://agent.example"}},
			},
		},
		{
			name:      "contact origin not discovered",
			discovery: true,
			steps: []step{
				{event: prompt("https://nimbus.example/orders"), contacts: []ContactEntry{nimbus}, cache: []cardcache.Entry{found(nimbus.Origin, "Nimbus", testNow)}, want: []string{"A2A contacts"}, wantAbsent: []string{"auto-discovered"}},
			},
		},
		{
			name: "discovery disabled",
			steps: []step{
				{event: prompt("https://agent.example"), cache: []cardcache.Entry{found("https://agent.example", "Agent", testNow)}},
			},
		},
		{
			name:      "compaction reinjects discovered agents",
			discovery: true,
			steps: []step{
				{event: prompt("https://agent.example"), cache: []cardcache.Entry{found("https://agent.example", "Agent", testNow)}, want: []string{"- https://agent.example"}},
				{event: start("compact"), want: []string{"- https://agent.example"}},
				{event: prompt("https://agent.example")},
			},
		},
		{
			name:      "clear forgets discovered agents",
			discovery: true,
			steps: []step{
				{event: prompt("https://agent.example"), cache: []cardcache.Entry{found("https://agent.example", "Agent", testNow)}, want: []string{"- https://agent.example"}},
				{event: start("clear")},
				{event: prompt("https://agent.example"), want: []string{"- https://agent.example"}},
			},
		},
		{
			name:      "tool batch scans only scan tools",
			discovery: true,
			steps: []step{
				{
					event:     `{"hook_event_name":"PostToolBatch","session_id":"s1","tool_calls":[{"tool_name":"Read","tool_input":{"file_path":"/x"},"tool_response":"https://read.example"},{"tool_name":"WebFetch","tool_input":{"url":"https://fetched.example/a"},"tool_response":"see https://linked.example"}]}`,
					wantProbe: []string{"https://fetched.example", "https://linked.example"},
				},
			},
		},
		{
			name:      "single tool event outside scan tools ignored",
			discovery: true,
			steps: []step{
				{event: `{"hook_event_name":"PostToolUse","session_id":"s1","tool_name":"Bash","tool_input":{"command":"curl https://bash.example"}}`},
			},
		},
		{
			name:      "new origins per event capped",
			discovery: true,
			steps: []step{
				{event: prompt("https://a.example https://b.example https://c.example https://d.example https://e.example https://f.example"), wantProbe: []string{"https://a.example", "https://b.example", "https://c.example", "https://d.example", "https://e.example"}},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := newHookEnv(t)
			for i, s := range tc.steps {
				env.now = env.now.Add(s.advance)
				for _, e := range s.cache {
					if err := env.cards.Put(e); err != nil {
						t.Fatalf("Cache.Put() error = %v", err)
					}
				}
				env.probed = nil
				got := env.run(env.hook(s.contacts, tc.discovery), s.event)

				if s.want == nil && got != "" {
					t.Fatalf("step %d: Hook.Run() = %q, want no output", i, got)
				}
				for _, w := range s.want {
					if !strings.Contains(got, w) {
						t.Fatalf("step %d: Hook.Run() = %q, want it to contain %q", i, got, w)
					}
				}
				for _, a := range s.wantAbsent {
					if strings.Contains(got, a) {
						t.Fatalf("step %d: Hook.Run() = %q, want it to not contain %q", i, got, a)
					}
				}
				var gotProbe []string
				for _, p := range env.probed {
					gotProbe = append(gotProbe, p...)
				}
				if diff := cmp.Diff(s.wantProbe, gotProbe); diff != "" {
					t.Fatalf("step %d: Hook.Run() probed wrong origins (-want +got) diff = %s", i, diff)
				}
			}
		})
	}
}

func TestHookRunWithoutSession(t *testing.T) {
	t.Parallel()
	env := newHookEnv(t)
	h := env.hook([]ContactEntry{nimbus}, true)

	if got := env.run(h, `{"hook_event_name":"SessionStart","source":"startup"}`); !strings.Contains(got, "- nimbus") {
		t.Fatalf("Hook.Run(SessionStart) = %q, want contacts", got)
	}
	if got := env.run(h, `{"hook_event_name":"UserPromptSubmit","prompt":"hi"}`); got != "" {
		t.Fatalf("Hook.Run(UserPromptSubmit) = %q, want no output", got)
	}
}

func TestHookRunWithLockedSession(t *testing.T) {
	t.Parallel()
	env := newHookEnv(t)
	h := env.hook([]ContactEntry{nimbus}, true)
	env.run(h, start("startup"))
	stateFile := filepath.Join(env.dir, "sessions", hashName("s1")+".json")
	before, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("os.ReadFile(state) error = %v", err)
	}
	lock, ok := tryLock(filepath.Join(env.dir, "sessions", hashName("s1")+".lock"), sessionLockStale)
	if !ok {
		t.Fatalf("tryLock() = false, want the session lock")
	}
	t.Cleanup(func() {
		if err := lock.unlock(); err != nil {
			t.Errorf("lockFile.unlock() error = %v", err)
		}
	})
	if err := env.cards.Put(found("https://agent.example", "Agent", testNow)); err != nil {
		t.Fatalf("Cache.Put() error = %v", err)
	}

	if got := env.run(h, prompt("https://agent.example")); !strings.Contains(got, "- https://agent.example") {
		t.Fatalf("Hook.Run() = %q, want the discovered agent", got)
	}
	after, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("os.ReadFile(state) error = %v", err)
	}
	if diff := cmp.Diff(string(before), string(after)); diff != "" {
		t.Fatalf("session state written without the lock (-want +got) diff = %s", diff)
	}
}

func TestHookRunInvalidInput(t *testing.T) {
	t.Parallel()
	env := newHookEnv(t)
	if _, err := env.hook(nil, true).Run([]byte("not json")); err == nil {
		t.Fatalf("Hook.Run(not json) error = nil, want error")
	}
}
