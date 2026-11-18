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
	"net/url"
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
	// MaxProbeWait caps ProbeWait, which must leave the hook well within the
	// harness's timeout.
	MaxProbeWait = 3 * time.Second
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
	// Cards holds the cards and AI Catalogs of discovered agents.
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
	// Probe starts fetching the cards and catalogs of origins without waiting
	// for the result.
	Probe func(origins []string) error
	// ProbeWait is how long an event that started probes of origins it
	// mentioned waits for them, so agents found are reported by the same
	// event. It is capped at MaxProbeWait.
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

	blocks, started, saved := h.runOnce(ev)
	// The second pass reads the state the first one saved, so without it the
	// session would be told the same thing twice.
	if wait := min(h.ProbeWait, MaxProbeWait); len(started) > 0 && wait > 0 && saved {
		h.waitForProbes(started, wait)
		blocks = append(blocks, h.collect(ev.SessionID, started))
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

// runOnce handles ev against the session state and returns the blocks to add,
// the origins it started probing, and whether it saved the session state.
func (h *Hook) runOnce(ev event) ([]string, []string, bool) {
	if ev.SessionID == "" {
		blocks, started := h.handle(ev, newSessionState(), false)
		return blocks, started, false
	}
	sess := sessionStore{dir: h.StateDir}.open(ev.SessionID)
	blocks, started := h.handle(ev, &sess.state, true)
	if err := sess.close(); err != nil {
		h.logf("%v", err)
		return blocks, started, false
	}
	return blocks, started, sess.lock != nil
}

// collect reports the agents found at origins since their probes started. It
// starts no probes, so waiting for probes can't trigger more of them.
func (h *Hook) collect(sessionID string, origins []string) string {
	sess := sessionStore{dir: h.StateDir}.open(sessionID)
	if sess.lock == nil {
		// Another event holds the session and reports the agents itself.
		return ""
	}
	st, now := &sess.state, h.now()
	var found []DiscoveredEntry
	for _, o := range origins {
		entry, _, unknown := h.lookup(o, st, now)
		if entry.found() {
			found = append(found, entry)
		}
		if !unknown {
			st.donePending(o)
		}
	}
	block := h.report(found, st, now)
	if err := sess.close(); err != nil {
		h.logf("%v", err)
		return ""
	}
	return block
}

// waitForProbes returns once the documents of every origin are cached, or
// after wait.
func (h *Hook) waitForProbes(origins []string, wait time.Duration) {
	const interval = 100 * time.Millisecond
	sleep := h.sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	for waited := time.Duration(0); waited < wait; waited += interval {
		sleep(interval)
		if !slices.ContainsFunc(origins, h.probing) {
			return
		}
	}
}

// probing reports whether a document of origin has no usable cache entry.
func (h *Hook) probing(origin string) bool {
	for _, ref := range h.refs(origin) {
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
	found, started := h.discover(ExtractOrigins(h.scanTexts(ev)...), st, now)
	return append(blocks, h.report(found, st, now)), started
}

// report renders the discovered agents. Agents that don't fit in the block are
// marked as not yet told and pending, so a later event reports them.
func (h *Hook) report(found []DiscoveredEntry, st *sessionState, now time.Time) string {
	block, dropped := FormatDiscovered(found)
	for _, d := range dropped {
		if d.Card != nil {
			delete(st.Injected, d.Origin)
		}
		if len(d.Agents) > 0 {
			delete(st.Injected, CatalogRef(d.Origin))
		}
		if _, pending := st.Pending[d.Origin]; !pending {
			st.Pending[d.Origin] = now
		}
	}
	return block
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
		block, _ := FormatDiscovered(h.injectedEntries(st))
		blocks = append(blocks, block)
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

// discover returns the agents to inject now and the origins it started
// probing: origins seen in this event or earlier whose card or catalog is
// cached and that the session hasn't been told about. Origins without a usable
// cache entry are probed in the background and reported once their documents
// are cached.
func (h *Hook) discover(origins []string, st *sessionState, now time.Time) ([]DiscoveredEntry, []string) {
	var candidates []string
	for o, started := range st.Pending {
		if now.Sub(started) > pendingMaxAge {
			st.donePending(o)
			continue
		}
		candidates = append(candidates, o)
	}
	slices.Sort(candidates)
	added := 0
	for _, o := range origins {
		if slices.Contains(candidates, o) || h.settled(o, st, now) {
			continue
		}
		if added == maxNewOrigins {
			break
		}
		added++
		candidates = append(candidates, o)
	}

	var found []DiscoveredEntry
	var toProbe, started []string
	for _, o := range candidates {
		entry, refresh, unknown := h.lookup(o, st, now)
		if entry.found() {
			found = append(found, entry)
		}
		if !unknown {
			st.donePending(o)
			if refresh {
				toProbe = append(toProbe, o)
			}
			continue
		}
		_, pending := st.Pending[o]
		if pending && now.Sub(st.lastProbe(o)) < probeRetry {
			continue
		}
		if !pending {
			if len(st.Pending) >= maxPending {
				continue
			}
			st.Pending[o] = now
			started = append(started, o)
		}
		st.Probed[o] = now
		toProbe = append(toProbe, o)
	}
	if len(toProbe) > 0 && h.Probe != nil {
		if err := h.Probe(toProbe); err != nil {
			h.logf("%v", err)
			return found, nil
		}
	}
	return found, started
}

// refs returns the cache references of the documents discovery reports for
// origin: its agent card, unless a contact's card is there, and its AI
// Catalog, unless the user registered it.
func (h *Hook) refs(origin string) []string {
	var refs []string
	if !slices.ContainsFunc(h.Contacts, func(c ContactEntry) bool { return c.Origin == origin }) {
		refs = append(refs, origin)
	}
	if ref := CatalogRef(origin); !h.knownCatalog(ref) {
		refs = append(refs, ref)
	}
	return refs
}

// lookup returns what the cache holds about origin that the session hasn't
// been told yet, marking it injected. refresh reports a found document that is
// due to be fetched again, and unknown a document that has no usable entry.
func (h *Hook) lookup(origin string, st *sessionState, now time.Time) (entry DiscoveredEntry, refresh, unknown bool) {
	entry.Origin = origin
	for _, ref := range h.refs(origin) {
		if _, done := st.Injected[ref]; done {
			continue
		}
		e, ok := h.Cards.Get(ref)
		switch {
		case ok && e.Found():
			if ref == origin {
				entry.Card = e.Card
			} else {
				entry.Agents = e.Agents
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
	for _, ref := range h.refs(origin) {
		if _, done := st.Injected[ref]; done {
			continue
		}
		if e, ok := h.Cards.Get(ref); !ok || e.Found() || !e.Fresh(now, negativeTTL) {
			return false
		}
	}
	return true
}

func (h *Hook) knownCatalog(ref string) bool {
	key := catalogKey(ref)
	return slices.ContainsFunc(h.KnownCatalogs, func(c string) bool { return catalogKey(c) == key })
}

// catalogKey normalizes a catalog URL for comparison: the scheme and host are
// lowercased, and a default port and the fragment are dropped.
func catalogKey(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	if port := u.Port(); (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80") {
		u.Host = u.Hostname()
		if strings.Contains(u.Host, ":") {
			u.Host = "[" + u.Host + "]"
		}
	}
	u.Fragment = ""
	u.RawFragment = ""
	return u.String()
}

// injectedEntries returns the discovered agents injected earlier, except those
// that have since become contacts or registered catalogs.
func (h *Hook) injectedEntries(st *sessionState) []DiscoveredEntry {
	byOrigin := map[string]*DiscoveredEntry{}
	for ref := range st.Injected {
		origin, isCatalog := strings.CutSuffix(ref, catalog.WellKnownPath)
		if !slices.Contains(h.refs(origin), ref) {
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
		if e := byOrigin[o]; e.found() {
			entries = append(entries, *e)
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
