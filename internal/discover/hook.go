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
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/a2aproject/a2a-cli/internal/cardcache"
	"github.com/a2aproject/a2a-cli/internal/contacts"
)

const (
	positiveTTL = 24 * time.Hour
	negativeTTL = 6 * time.Hour
	// probeRetry is how long a pending origin waits before its probe is restarted.
	probeRetry    = time.Minute
	pendingMaxAge = 10 * time.Minute
	// maxNewOrigins bounds the probes a single event can trigger.
	maxNewOrigins = 5
	maxPending    = 20
)

const contactsUpdatedPrefix = "The user's A2A contact book changed; this list replaces any earlier one. "

const contactsEmptied = "The user's A2A contact book is now empty: agents listed as contacts earlier in this conversation are no longer trusted."

// DefaultScanTools are the tools whose calls are scanned for URLs to probe.
var DefaultScanTools = []string{"WebFetch", "WebSearch"}

func ttl(e cardcache.Entry) time.Duration {
	if e.Found() {
		return positiveTTL
	}
	return negativeTTL
}

// Hook turns one harness hook event into the context to add to the conversation.
type Hook struct {
	Contacts []contacts.Contact
	Cards    *cardcache.Cache
	// StateDir holds per-session state.
	StateDir string
	// Discovery enables probing origins of URLs in the conversation.
	Discovery bool
	// ScanTools are the tool names whose input and output are scanned for URLs.
	ScanTools []string
	// Probe starts fetching the cards of origins without waiting for the result.
	Probe func(origins []string) error
	Now   func() time.Time
	Logf  func(format string, args ...any)
}

type event struct {
	HookEventName string          `json:"hook_event_name"`
	SessionID     string          `json:"session_id"`
	Source        string          `json:"source"`
	Prompt        string          `json:"prompt"`
	ToolName      string          `json:"tool_name"`
	ToolInput     json.RawMessage `json:"tool_input"`
	ToolResponse  json.RawMessage `json:"tool_response"`
	ToolCalls     []toolCall      `json:"tool_calls"`
}

type toolCall struct {
	ToolName     string          `json:"tool_name"`
	ToolInput    json.RawMessage `json:"tool_input"`
	ToolResponse json.RawMessage `json:"tool_response"`
}

type hookOutput struct {
	HookSpecificOutput hookSpecificOutput `json:"hookSpecificOutput"`
}

type hookSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}

// Run handles a hook event read from the harness and returns the JSON to print
// for it, or "" when there is nothing to add.
func (h *Hook) Run(input []byte) (string, error) {
	var ev event
	if err := json.Unmarshal(input, &ev); err != nil {
		return "", fmt.Errorf("parsing hook event: %w", err)
	}

	st := &sessionState{Injected: map[string]time.Time{}, Pending: map[string]time.Time{}}
	var sess *openSession
	if ev.SessionID != "" {
		sess = sessionStore{dir: h.StateDir}.open(ev.SessionID)
		st = &sess.state
	}
	blocks := h.handle(ev, st, sess != nil)
	if sess != nil {
		if err := sess.close(); err != nil {
			h.logf("%v", err)
		}
	}

	var nonEmpty []string
	for _, b := range blocks {
		if b != "" {
			nonEmpty = append(nonEmpty, b)
		}
	}
	if len(nonEmpty) == 0 {
		return "", nil
	}
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(hookOutput{HookSpecificOutput: hookSpecificOutput{
		HookEventName:     ev.HookEventName,
		AdditionalContext: strings.Join(nonEmpty, "\n\n"),
	}}); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

func (h *Hook) handle(ev event, st *sessionState, tracked bool) []string {
	now := h.now()
	contactsBlock := FormatContacts(h.contactEntries())
	digest := hashName(contactsBlock)

	if ev.HookEventName == "SessionStart" {
		return h.sessionStart(ev.Source, st, contactsBlock, digest, now)
	}

	var blocks []string
	if tracked && digest != st.ContactsDigest {
		switch {
		case contactsBlock != "":
			blocks = append(blocks, contactsUpdatedPrefix+contactsBlock)
		case st.ContactsDigest != "":
			blocks = append(blocks, contactsEmptied)
		}
		st.ContactsDigest = digest
	}
	if h.Discovery {
		blocks = append(blocks, FormatDiscovered(h.discover(ExtractOrigins(h.scanTexts(ev)...), st, now)))
	}
	return blocks
}

func (h *Hook) sessionStart(source string, st *sessionState, contactsBlock, digest string, now time.Time) []string {
	var blocks []string
	switch source {
	case "startup", "clear":
		*st = sessionState{Injected: map[string]time.Time{}, Pending: map[string]time.Time{}}
		if err := (sessionStore{dir: h.StateDir}).prune(now); err != nil {
			h.logf("pruning session state: %v", err)
		}
	case "compact":
		if h.Discovery {
			blocks = append(blocks, FormatDiscovered(h.injectedEntries(st)))
		}
	}
	// A resumed conversation still holds the contacts block it was given.
	if source != "resume" || digest != st.ContactsDigest {
		blocks = append([]string{contactsBlock}, blocks...)
	}
	st.ContactsDigest = digest
	return blocks
}

func (h *Hook) scanTexts(ev event) []string {
	scanTools := h.ScanTools
	if scanTools == nil {
		scanTools = DefaultScanTools
	}
	var texts []string
	switch ev.HookEventName {
	case "UserPromptSubmit":
		texts = append(texts, ev.Prompt)
	case "PostToolUse":
		if slices.Contains(scanTools, ev.ToolName) {
			texts = append(texts, jsonStrings(ev.ToolInput)...)
			texts = append(texts, jsonStrings(ev.ToolResponse)...)
		}
	case "PostToolBatch":
		for _, c := range ev.ToolCalls {
			if slices.Contains(scanTools, c.ToolName) {
				texts = append(texts, jsonStrings(c.ToolInput)...)
				texts = append(texts, jsonStrings(c.ToolResponse)...)
			}
		}
	}
	return texts
}

// discover returns the agents to inject now: origins seen in this event or
// earlier whose card is cached and that the session hasn't been told about.
// Origins without a usable cache entry are probed in the background and
// reported by a later event once their card is cached.
func (h *Hook) discover(origins []string, st *sessionState, now time.Time) []DiscoveredEntry {
	skip := map[string]bool{}
	for _, c := range h.Contacts {
		if o := c.Origin(); o != "" {
			skip[o] = true
		}
	}

	var candidates []string
	for o, started := range st.Pending {
		if now.Sub(started) > pendingMaxAge {
			delete(st.Pending, o)
			continue
		}
		candidates = append(candidates, o)
	}
	slices.Sort(candidates)
	added := 0
	for _, o := range origins {
		if _, injected := st.Injected[o]; injected || skip[o] || slices.Contains(candidates, o) {
			continue
		}
		if added == maxNewOrigins {
			break
		}
		added++
		candidates = append(candidates, o)
	}

	var found []DiscoveredEntry
	var toProbe []string
	for _, o := range candidates {
		if skip[o] {
			delete(st.Pending, o)
			continue
		}
		e, ok := h.Cards.Get(o)
		switch {
		case ok && e.Found():
			found = append(found, DiscoveredEntry{Origin: o, Card: e.Card})
			st.Injected[o] = now
			delete(st.Pending, o)
			if !e.Fresh(now, positiveTTL) {
				toProbe = append(toProbe, o)
			}
		case ok && e.Fresh(now, negativeTTL):
			delete(st.Pending, o)
		default:
			started, pending := st.Pending[o]
			if pending && now.Sub(started) < probeRetry {
				continue
			}
			if !pending && len(st.Pending) >= maxPending {
				continue
			}
			st.Pending[o] = now
			toProbe = append(toProbe, o)
		}
	}
	if len(toProbe) > 0 && h.Probe != nil {
		if err := h.Probe(toProbe); err != nil {
			h.logf("%v", err)
		}
	}
	return found
}

func (h *Hook) contactEntries() []ContactEntry {
	entries := make([]ContactEntry, 0, len(h.Contacts))
	for _, c := range h.Contacts {
		entry := ContactEntry{Contact: c}
		if e, ok := h.Cards.Get(c.Card); ok {
			entry.Card = e.Card
		}
		entries = append(entries, entry)
	}
	return entries
}

func (h *Hook) injectedEntries(st *sessionState) []DiscoveredEntry {
	origins := make([]string, 0, len(st.Injected))
	for o := range st.Injected {
		origins = append(origins, o)
	}
	slices.Sort(origins)
	var entries []DiscoveredEntry
	for _, o := range origins {
		if e, ok := h.Cards.Get(o); ok && e.Found() {
			entries = append(entries, DiscoveredEntry{Origin: o, Card: e.Card})
		}
	}
	return entries
}

func (h *Hook) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

func (h *Hook) logf(format string, args ...any) {
	if h.Logf != nil {
		h.Logf(format, args...)
	}
}
