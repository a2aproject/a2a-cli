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
	"context"
	"encoding/json"
	"time"
)

// event holds the fields of a Claude Code hook payload that this tool reads. The
// full payload carries more; unknown fields are ignored. Origins are extracted
// from the entire raw payload (see Run), so this only captures routing metadata.
type event struct {
	HookEventName string `json:"hook_event_name"`
	SessionID     string `json:"session_id"`
}

// hookOutput is the JSON a Claude Code hook prints to add context to the model.
type hookOutput struct {
	HookSpecificOutput hookSpecificOutput `json:"hookSpecificOutput"`
}

type hookSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}

// RunConfig configures a single discovery pass over a harness event.
type RunConfig struct {
	// Input is the raw hook payload (JSON) received on stdin. Origins are
	// extracted from the whole payload, so URLs are found wherever they appear
	// (prompt, tool input, tool result).
	Input []byte
	// ExtraOrigins are probed in addition to any found in Input (manual use).
	ExtraOrigins []string
	// Guard, Timeout, Cache, Logf are passed through to discovery.
	Guard   Guard
	Timeout time.Duration
	Cache   *Cache
	Logf    func(format string, args ...any)
	// SessionID overrides the payload's session id (for tests / manual runs).
	SessionID string
}

// Run performs discovery for one harness event and returns the hook stdout to
// emit: a JSON object adding the discovered agents to context, or "" when there
// is nothing to add. It never returns an error for a probe miss — a hook must
// stay silent rather than disrupt the turn — but it does surface malformed input.
func Run(ctx context.Context, cfg RunConfig) (string, error) {
	var ev event
	if len(cfg.Input) > 0 {
		if err := json.Unmarshal(cfg.Input, &ev); err != nil {
			return "", err
		}
	}
	session := cfg.SessionID
	if session == "" {
		session = ev.SessionID
	}

	origins := ExtractOrigins(string(cfg.Input))
	origins = append(origins, cfg.ExtraOrigins...)
	origins = dedupe(origins)

	candidates := cfg.filterCandidates(session, origins)
	findings := Discover(ctx, Options{
		Timeout: cfg.Timeout,
		Guard:   cfg.Guard,
		Logf:    cfg.Logf,
	}, candidates)

	cfg.record(session, candidates, findings)

	if len(findings) == 0 {
		return "", nil
	}

	out := hookOutput{HookSpecificOutput: hookSpecificOutput{
		HookEventName:     hookEventName(ev.HookEventName),
		AdditionalContext: FormatContext(findings),
	}}
	data, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// filterCandidates drops origins already surfaced this session or recently known
// not to be agents, so the hook does no redundant work.
func (cfg RunConfig) filterCandidates(session string, origins []string) []string {
	if cfg.Cache == nil {
		return origins
	}
	var out []string
	for _, o := range origins {
		if cfg.Cache.InjectedInSession(session, o) {
			continue
		}
		if found, fresh := cfg.Cache.Fresh(o); fresh && !found {
			continue
		}
		out = append(out, o)
	}
	return out
}

func (cfg RunConfig) record(session string, probed []string, findings []Finding) {
	if cfg.Cache == nil {
		return
	}
	found := make(map[string]bool, len(findings))
	for _, f := range findings {
		found[f.Origin] = true
	}
	for _, o := range probed {
		cfg.Cache.Record(o, found[o])
	}
	var injected []string
	for _, f := range findings {
		injected = append(injected, f.Origin)
	}
	cfg.Cache.MarkInjected(session, injected)
}

func hookEventName(name string) string {
	if name == "" {
		return "UserPromptSubmit"
	}
	return name
}

func dedupe(in []string) []string {
	seen := make(map[string]bool, len(in))
	var out []string
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
