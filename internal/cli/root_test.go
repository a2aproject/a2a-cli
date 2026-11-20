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

package cli

import "testing"

// outputFlagFromArgs recovers an explicit -o/--output value when pflag stopped
// parsing before reaching it, so every accepted spelling has to be recognised.
func TestOutputFlagFromArgs(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
		ok   bool
	}{
		{"short separated", []string{"send", "-o", "json", "hi"}, "json", true},
		{"long separated", []string{"send", "--output", "json", "hi"}, "json", true},
		{"short equals", []string{"send", "-o=json", "hi"}, "json", true},
		{"long equals", []string{"send", "--output=json", "hi"}, "json", true},
		{"after unknown flag", []string{"send", "--bogus", "-o=json", "hi"}, "json", true},
		{"after unknown flag separated", []string{"send", "--bogus", "-o", "json", "hi"}, "json", true},
		{"absent", []string{"send", "hi"}, "", false},
		{"trailing short", []string{"send", "hi", "-o"}, "", false},
		{"value looks like flag", []string{"send", "-o", "--weird"}, "--weird", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := outputFlagFromArgs(c.args)
			if got != c.want || ok != c.ok {
				t.Fatalf("outputFlagFromArgs(%q) = (%q, %v), want (%q, %v)", c.args, got, ok, c.want, c.ok)
			}
		})
	}
}
