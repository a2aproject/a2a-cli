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

package cryptoutil

import (
	"strings"
	"testing"
)

func TestRandomString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		n        int
		alphabet string
	}{
		{name: "alphanumeric", n: 40, alphabet: Alphanumeric},
		{name: "short alphabet", n: 8, alphabet: "ab"},
		{name: "empty", n: 0, alphabet: Alphanumeric},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := RandomString(tc.n, tc.alphabet)
			if len(got) != tc.n {
				t.Fatalf("RandomString() = %q, want %d characters", got, tc.n)
			}
			for _, c := range got {
				if !strings.ContainsRune(tc.alphabet, c) {
					t.Fatalf("RandomString() = %q, want characters from %q", got, tc.alphabet)
				}
			}
		})
	}
}

func TestHash(t *testing.T) {
	t.Parallel()
	want := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got := Hash("hello"); got != want {
		t.Fatalf("Hash() = %v, want %v", got, want)
	}
}
