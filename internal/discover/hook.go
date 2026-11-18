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
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/Agent-Card/ai-catalog-go/catalog"

	"github.com/a2aproject/a2a-cli/internal/cardcache"
)

const (
	positiveTTL = cardcache.FoundTTL
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
	Contacts []ContactEntry
	// Cards holds the cards of discovered agents.
	Cards *cardcache.Cache
	// StateDir holds per-session state.
	StateDir string
	// Discovery enables probing origins of URLs in the conversation.
	Discovery bool
	// ScanTools are the tool names whose input and output are scanned for URLs.
	ScanTools []string
	// KnownCatalogs are the source URLs of registered AI Catalogs, which
	// discovery doesn't report.
	KnownCatalogs []string
	// Probe starts fetching the cards of origins without waiting for the result.
	Probe func(origins []string) error
	// ProbeWait is how long an event that started probes waits for them, so
	// agents found are reported by the same event.
	ProbeWait time.Duration
	Now       func() time.Time
	Logf      func(format string, args ...any)
	// sleep pauses between checks of the card cache while waiting for probes.
	sleep func(time.Duration)
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

	blocks, probed := h.runOnce(ev)
	// Without a session the state that keeps the second pass from probing
	// again isn't saved.
	if len(probed) > 0 && h.ProbeWait > 0 && ev.SessionID != "" {
		h.waitForProbes(probed)
		more, _ := h.runOnce(ev)
		blocks = append(blocks, more...)
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

// runOnce handles ev against the session state and returns the blocks to add
// and the origins whose probes it started.
func (h *Hook) runOnce(ev event) ([]string, []string) {
	st := newSessionState()
	var sess *openSession
	if ev.SessionID != "" {
		sess = sessionStore{dir: h.StateDir}.open(ev.SessionID)
		st = &sess.state
	}
	blocks, probed := h.handle(ev, st, sess != nil)
	if sess != nil {
		if err := sess.close(); err != nil {
			h.logf("%v", err)
		}
	}
	return blocks, probed
}

// waitForProbes returns once the card and catalog of every origin are cached,
// or after ProbeWait.
func (h *Hook) waitForProbes(origins []string) {
	const interval = 100 * time.Millisecond
	sleep := h.sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	for waited := time.Duration(0); waited < h.ProbeWait; waited += interval {
		sleep(interval)
		if !slices.ContainsFunc(origins, h.probing) {
			return
		}
	}
}

// probing reports whether a document of origin has no usable cache entry.
func (h *Hook) probing(origin string) bool {
	for _, ref := range []string{origin, CatalogRef(origin)} {
		if e, ok := h.Cards.Get(ref); !ok || (!e.Found() && !e.Fresh(h.now(), negativeTTL)) {
			return true
		}
	}
	return false
}

func (h *Hook) handle(ev event, st *sessionState, tracked bool) ([]string, []string) {
	now := h.now()
	contactsBlock := FormatContacts(h.Contacts)
	digest := hashName(contactsBlock)

	if ev.HookEventName == "SessionStart" {
		return h.sessionStart(ev.Source, st, contactsBlock, digest, now), nil
	}

	var blocks []string
	if tracked {
		blocks = append(blocks, contactsChange(st, contactsBlock, digest))
	}
	if !h.Discovery {
		return blocks, nil
	}
	found, probed := h.discover(ExtractOrigins(h.scanTexts(ev)...), st, now)
	return append(blocks, FormatDiscovered(found)), probed
}

// contactsChange returns the block that brings a conversation given the
// contacts block st records up to date with contactsBlock, or "".
func contactsChange(st *sessionState, contactsBlock, digest string) string {
	if digest == st.ContactsDigest {
		return ""
	}
	previous := st.ContactsDigest
	st.ContactsDigest = digest
	switch {
	case contactsBlock != "":
		st.HadContacts = true
		return contactsUpdatedPrefix + contactsBlock
	case previous != "":
		return contactsEmptied
	}
	return ""
}

func (h *Hook) sessionStart(source string, st *sessionState, contactsBlock, digest string, now time.Time) []string {
	switch source {
	case "resume":
		// A resumed conversation still holds the contacts block it was given.
		return []string{contactsChange(st, contactsBlock, digest)}
	case "startup", "clear":
		*st = *newSessionState()
		if err := (sessionStore{dir: h.StateDir}).prune(now); err != nil {
			h.logf("pruning session state: %v", err)
		}
	}
	blocks := []string{contactsBlock}
	if contactsBlock != "" {
		st.HadContacts = true
	} else if source == "compact" && st.HadContacts {
		// The summary may still treat removed contacts as trusted.
		blocks = []string{contactsEmptied}
	}
	if source == "compact" && h.Discovery {
		blocks = append(blocks, FormatDiscovered(h.injectedEntries(st)))
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

// discover returns the agents to inject now and the origins it started probes
// for: origins seen in this event or earlier whose card or catalog is cached
// and that the session hasn't been told about. Origins without a usable cache
// entry are probed in the background and reported by a later event once their
// documents are cached.
func (h *Hook) discover(origins []string, st *sessionState, now time.Time) ([]DiscoveredEntry, []string) {
	skip := h.contactOrigins()

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
		if skip[o] || slices.Contains(candidates, o) || h.settled(o, st, now) {
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
		entry, refresh, unknown := h.lookup(o, st, now)
		if entry.Card != nil || len(entry.Agents) > 0 {
			found = append(found, entry)
		}
		if !unknown {
			delete(st.Pending, o)
			if refresh {
				toProbe = append(toProbe, o)
			}
			continue
		}
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
	if len(toProbe) > 0 && h.Probe != nil {
		if err := h.Probe(toProbe); err != nil {
			h.logf("%v", err)
			return found, nil
		}
	}
	return found, toProbe
}

// lookup returns what the cache holds about origin that the session hasn't
// been told yet, marking it injected. refresh reports a found document that is
// due to be fetched again, and unknown a document that has no usable entry.
func (h *Hook) lookup(origin string, st *sessionState, now time.Time) (entry DiscoveredEntry, refresh, unknown bool) {
	entry.Origin = origin
	catalogRef := CatalogRef(origin)
	for _, ref := range []string{origin, catalogRef} {
		if _, done := st.Injected[ref]; done {
			continue
		}
		if ref == catalogRef && h.knownCatalog(ref) {
			continue
		}
		e, ok := h.Cards.Get(ref)
		switch {
		case ok && e.Found():
			if ref == catalogRef {
				entry.Agents = e.Agents
			} else {
				entry.Card = e.Card
			}
			st.Injected[ref] = now
			refresh = refresh || !e.Fresh(now, positiveTTL)
		case ok && e.Fresh(now, negativeTTL):
		default:
			unknown = true
		}
	}
	return entry, refresh, unknown
}

// settled reports whether each document of origin was reported to the session
// or is cached as missing, so a mention of origin has nothing to add.
func (h *Hook) settled(origin string, st *sessionState, now time.Time) bool {
	catalogRef := CatalogRef(origin)
	for _, ref := range []string{origin, catalogRef} {
		if _, done := st.Injected[ref]; done || (ref == catalogRef && h.knownCatalog(ref)) {
			continue
		}
		if e, ok := h.Cards.Get(ref); !ok || e.Found() || !e.Fresh(now, negativeTTL) {
			return false
		}
	}
	return true
}

func (h *Hook) knownCatalog(ref string) bool {
	return slices.ContainsFunc(h.KnownCatalogs, func(c string) bool { return strings.EqualFold(c, ref) })
}

// injectedEntries returns the discovered agents injected earlier, except those
// that have since become contacts.
func (h *Hook) injectedEntries(st *sessionState) []DiscoveredEntry {
	skip := h.contactOrigins()
	byOrigin := map[string]*DiscoveredEntry{}
	for ref := range st.Injected {
		origin, isCatalog := strings.CutSuffix(ref, catalog.WellKnownPath)
		if skip[origin] || (isCatalog && h.knownCatalog(ref)) {
			continue
		}
		e, ok := h.Cards.Get(ref)
		if !ok || !e.Found() {
			continue
		}
		entry := byOrigin[origin]
		if entry == nil {
			entry = &DiscoveredEntry{Origin: origin}
			byOrigin[origin] = entry
		}
		if isCatalog {
			entry.Agents = e.Agents
		} else {
			entry.Card = e.Card
		}
	}
	var entries []DiscoveredEntry
	for _, o := range slices.Sorted(maps.Keys(byOrigin)) {
		if e := byOrigin[o]; e.Card != nil || len(e.Agents) > 0 {
			entries = append(entries, *e)
		}
	}
	return entries
}

func (h *Hook) contactOrigins() map[string]bool {
	origins := map[string]bool{}
	for _, c := range h.Contacts {
		if c.Origin != "" {
			origins[c.Origin] = true
		}
	}
	return origins
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
