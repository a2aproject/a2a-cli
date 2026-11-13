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

// Package discover surfaces A2A agents to an agent harness through its hooks:
// the user's contacts on session start and after compaction, and agents found
// by probing the origins of URLs that enter the conversation.
package discover

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/a2aproject/a2a-cli/internal/contacts"
)

// urlPattern stops a URL at ASCII delimiters and at any Unicode separator,
// control or format character, so the origin can't carry a line break or a
// bidirectional override into the injected context.
var urlPattern = regexp.MustCompile(`https?://[^\s\p{Z}\p{Cc}\p{Cf}"'<>` + "`" + `)\]}|\\]+`)

// ExtractOrigins returns the distinct origins (scheme://host[:port]) of the
// http(s) URLs in texts, in first-seen order.
func ExtractOrigins(texts ...string) []string {
	var origins []string
	seen := make(map[string]bool)
	for _, text := range texts {
		for _, raw := range urlPattern.FindAllString(text, -1) {
			origin := contacts.Origin(strings.TrimRight(raw, ".,;:!?"))
			if origin == "" || seen[origin] {
				continue
			}
			seen[origin] = true
			origins = append(origins, origin)
		}
	}
	return origins
}

// jsonStrings returns every string value nested in a JSON document. URLs are
// matched against decoded strings so JSON escapes don't leak into them.
func jsonStrings(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return []string{string(raw)}
	}
	var out []string
	var walk func(any)
	walk = func(v any) {
		switch v := v.(type) {
		case string:
			out = append(out, v)
		case []any:
			for _, item := range v {
				walk(item)
			}
		case map[string]any:
			for _, item := range v {
				walk(item)
			}
		}
	}
	walk(v)
	return out
}
