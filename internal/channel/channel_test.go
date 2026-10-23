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

package channel

import "testing"

func TestTruncate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		in    string
		limit int
		want  string
	}{
		{name: "short", in: "abc", limit: 5, want: "abc"},
		{name: "ascii", in: "abcdef", limit: 3, want: "abc"},
		{name: "inside a character", in: "ab✓cd", limit: 4, want: "ab"},
		{name: "after a character", in: "ab✓cd", limit: 5, want: "ab✓"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := Truncate(tc.in, tc.limit); got != tc.want {
				t.Fatalf("Truncate(%q, %d) = %q, want %q", tc.in, tc.limit, got, tc.want)
			}
		})
	}
}
