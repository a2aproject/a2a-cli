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

// Package credhelper runs credential helpers: programs that return the HTTP
// headers to send to a URI. It implements the Bazel credential helper protocol
// (https://github.com/EngFlow/credential-helper-spec), so helpers written for
// Bazel work unchanged.
package credhelper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

// Helper is a configured credential helper.
type Helper struct {
	// Spec is the configured value, for messages.
	Spec string
	// Scope is the host the helper applies to: "example.com", "*.example.com"
	// for the domain and its subdomains, or empty for every host.
	Scope string
	// Command runs the helper; "get" is appended to it.
	Command []string
}

// Response is what a helper prints for a "get" request.
type Response struct {
	Headers map[string][]string `json:"headers"`
	// Expires is when the headers stop being valid, in RFC 3339.
	Expires string `json:"expires,omitempty"`
}

type request struct {
	URI string `json:"uri"`
}

// Parse parses "[scope=]path", the form Bazel's --credential_helper takes.
// The scope is the text before the first '=' when it looks like a host
// pattern.
func Parse(spec string) (Helper, error) {
	h := Helper{Spec: spec}
	path := strings.TrimSpace(spec)
	if scope, rest, ok := strings.Cut(path, "="); ok && isScope(scope) {
		if err := checkScope(scope); err != nil {
			return Helper{}, fmt.Errorf("credential helper %q: %w", spec, err)
		}
		h.Scope, path = strings.ToLower(scope), strings.TrimSpace(rest)
	}
	if path == "" {
		return Helper{}, fmt.Errorf("credential helper %q: no path", spec)
	}
	h.Command = []string{path}
	return h, nil
}

// Entry is a credential helper from a config file.
type Entry struct {
	Scope   string   `json:"scope"`
	Command []string `json:"command"`
}

// Helper validates e.
func (e Entry) Helper() (Helper, error) {
	spec := strings.Join(e.Command, " ")
	if e.Scope != "" {
		spec = e.Scope + "=" + spec
	}
	if len(e.Command) == 0 || e.Command[0] == "" {
		return Helper{}, fmt.Errorf("credential helper %q: no command", spec)
	}
	if e.Scope != "" {
		if !isScope(e.Scope) {
			return Helper{}, fmt.Errorf("credential helper %q: scope %q must be a host or *.domain", spec, e.Scope)
		}
		if err := checkScope(e.Scope); err != nil {
			return Helper{}, fmt.Errorf("credential helper %q: %w", spec, err)
		}
	}
	return Helper{Spec: spec, Scope: strings.ToLower(e.Scope), Command: e.Command}, nil
}

func checkScope(scope string) error {
	if strings.Contains(scope, ":") {
		return fmt.Errorf("scope %q must be a host without a port", scope)
	}
	return nil
}

func isScope(s string) bool {
	if s == "" || strings.ContainsAny(s, " \t/\\\"'") {
		return false
	}
	host := strings.TrimPrefix(s, "*.")
	return host != "" && !strings.Contains(host, "*")
}

// Select returns the helper for rawURL: the one with the most specific
// matching scope, an exact host before a wildcard and a longer wildcard
// before a shorter one, with unscoped helpers last. Among equally specific
// helpers the last one wins, so a later setting overrides an earlier one.
func Select(helpers []Helper, rawURL string) (Helper, bool) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return Helper{}, false
	}
	host := strings.ToLower(u.Hostname())
	best, bestRank := Helper{}, -1
	for _, h := range helpers {
		rank := matchRank(h.Scope, host)
		if rank >= bestRank && rank >= 0 {
			best, bestRank = h, rank
		}
	}
	return best, bestRank >= 0
}

// matchRank is -1 when scope doesn't match host, and higher for more
// specific matches.
func matchRank(scope, host string) int {
	switch {
	case scope == "":
		return 0
	case scope == host:
		return 1 << 20
	case strings.HasPrefix(scope, "*."):
		domain := scope[2:]
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return len(domain)
		}
	}
	return -1
}

// Get runs the helper for uri. ctx bounds the helper's run.
func (h Helper) Get(ctx context.Context, uri string) (*Response, error) {
	in, err := json.Marshal(request{URI: uri})
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, h.Command[0], append(h.Command[1:], "get")...)
	cmd.Stdin = bytes.NewReader(in)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("credential helper %q: %w: %s", h.Spec, err, msg)
		}
		return nil, fmt.Errorf("credential helper %q: %w", h.Spec, err)
	}
	var resp Response
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("credential helper %q: invalid response: %w", h.Spec, err)
	}
	if resp.Expires != "" {
		if _, err := time.Parse(time.RFC3339, resp.Expires); err != nil {
			return nil, fmt.Errorf("credential helper %q: invalid expires: %w", h.Spec, err)
		}
	}
	for name := range resp.Headers {
		if name == "" {
			return nil, fmt.Errorf("credential helper %q: empty header name", h.Spec)
		}
	}
	return &resp, nil
}
