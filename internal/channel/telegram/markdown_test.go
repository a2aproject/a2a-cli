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

package telegram

import "testing"

func TestToMarkdownV2(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain", in: "Done. 1+1=2!", want: `Done\. 1\+1\=2\!`},
		{name: "bold", in: "**Fixed** the test", want: `*Fixed* the test`},
		{name: "inline code", in: "ran `go test ./...`", want: "ran `go test ./...`"},
		{name: "code block", in: "```go\nx := a_b\n```", want: "```go\nx := a_b\n```"},
		{name: "unclosed code block", in: "```\nx", want: "```\nx\n```"},
		{name: "heading", in: "## Summary (v2)", want: `*Summary \(v2\)*`},
		{name: "link", in: "see [PR #1](https://x.dev/pr?a=1)", want: `see [PR \#1](https://x.dev/pr?a=1)`},
		{name: "not a link", in: "[a] (b)", want: `\[a\] \(b\)`},
		{name: "unicode", in: "✓ Approved – ok.", want: `✓ Approved – ok\.`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ToMarkdownV2(tc.in); got != tc.want {
				t.Fatalf("ToMarkdownV2(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
