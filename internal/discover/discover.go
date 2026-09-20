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

// Package discover implements progressive, context-aware discovery of A2A
// agents. Given text that has entered an agent harness's context window (a user
// prompt, a fetched page, a tool result), it extracts origins, probes each for a
// well-known Agent Card, and reports the agents it finds so their skills can be
// surfaced back into the harness.
package discover

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
	"github.com/a2aproject/a2a-go/v2/a2acompat/a2av0"
)

// maxOrigins caps how many distinct origins a single pass will probe, bounding
// the work (and network fan-out) triggered by any one chunk of context.
const maxOrigins = 6

// Options configure a discovery pass.
type Options struct {
	// Timeout bounds each individual card probe.
	Timeout time.Duration
	// Guard decides which hosts may be probed (SSRF protection).
	Guard Guard
	// Resolver fetches and parses cards. When nil, a resolver that understands
	// both A2A v0.x and v1.0 cards is used.
	Resolver *agentcard.Resolver
	// Logf receives diagnostic messages; may be nil.
	Logf func(format string, args ...any)
}

// Finding is an A2A agent discovered at an origin.
type Finding struct {
	Origin string
	Card   *a2a.AgentCard
}

// urlPattern matches http(s) URLs embedded in free text.
var urlPattern = regexp.MustCompile(`https?://[^\s"'<>` + "`" + `)\]}]+`)

// ExtractOrigins finds http(s) URLs in text and returns their distinct origins
// (scheme://host[:port]) in first-seen order.
func ExtractOrigins(text string) []string {
	var origins []string
	seen := make(map[string]bool)
	for _, raw := range urlPattern.FindAllString(text, -1) {
		raw = strings.TrimRight(raw, ".,;:!?")
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			continue
		}
		origin := u.Scheme + "://" + u.Host
		if seen[origin] {
			continue
		}
		seen[origin] = true
		origins = append(origins, origin)
	}
	return origins
}

func (o Options) logf(format string, args ...any) {
	if o.Logf != nil {
		o.Logf(format, args...)
	}
}

func (o Options) resolver() *agentcard.Resolver {
	if o.Resolver != nil {
		return o.Resolver
	}
	timeout := o.Timeout
	if timeout <= 0 {
		timeout = 4 * time.Second
	}
	r := agentcard.NewResolver(&http.Client{Timeout: timeout})
	r.CardParser = a2av0.NewAgentCardParser()
	return r
}

// Probe fetches the Agent Card at origin's well-known path, returning a nil card
// (and nil error) when the origin is not an A2A agent.
func Probe(ctx context.Context, r *agentcard.Resolver, origin string) (*a2a.AgentCard, error) {
	card, err := r.Resolve(ctx, origin)
	if err != nil {
		return nil, err
	}
	return card, nil
}

// Discover probes each origin (after the guard admits it) and returns the agents
// found. Origins that are blocked, unreachable, or not A2A agents are skipped.
func Discover(ctx context.Context, opts Options, origins []string) []Finding {
	resolver := opts.resolver()
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 4 * time.Second
	}

	var findings []Finding
	probed := 0
	for _, origin := range origins {
		if probed >= maxOrigins {
			break
		}
		host := hostOf(origin)
		if allowed, reason := opts.Guard.Allow(host); !allowed {
			opts.logf("skipping %s: %s", origin, reason)
			continue
		}
		probed++

		probeCtx, cancel := context.WithTimeout(ctx, timeout)
		card, err := Probe(probeCtx, resolver, origin)
		cancel()
		if err != nil {
			opts.logf("no agent card at %s: %v", origin, err)
			continue
		}
		if card == nil {
			continue
		}
		opts.logf("discovered agent %q at %s", card.Name, origin)
		findings = append(findings, Finding{Origin: origin, Card: card})
	}
	return findings
}

func hostOf(origin string) string {
	u, err := url.Parse(origin)
	if err != nil {
		return origin
	}
	return u.Hostname()
}
