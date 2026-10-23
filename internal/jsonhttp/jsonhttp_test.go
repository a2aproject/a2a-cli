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

package jsonhttp

import (
	"strings"
	"testing"
)

func TestErrorMessage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "api error", body: `{"error":"no such agent"}`, want: "no such agent"},
		{name: "empty body", body: "", want: "404 Not Found"},
		{name: "text body", body: "page not found\n", want: "404 Not Found: page not found"},
		{name: "long body", body: strings.Repeat("x", 400), want: "404 Not Found: " + strings.Repeat("x", 300) + "…"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ErrorMessage("404 Not Found", []byte(tc.body)); got != tc.want {
				t.Fatalf("ErrorMessage() = %q, want %q", got, tc.want)
			}
		})
	}
}
