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
	"net"
	"testing"
)

func TestGuardAllow(t *testing.T) {
	t.Parallel()

	lookup := map[string][]net.IP{
		"public.example":   {net.ParseIP("93.184.216.34")},
		"internal.example": {net.ParseIP("10.0.0.5")},
		"meta.example":     {net.ParseIP("169.254.169.254")},
	}
	fakeLookup := func(host string) ([]net.IP, error) { return lookup[host], nil }

	tests := []struct {
		name  string
		guard Guard
		host  string
		want  bool
	}{
		{
			name:  "public host allowed",
			guard: Guard{lookupIP: fakeLookup},
			host:  "public.example",
			want:  true,
		},
		{
			name:  "private host blocked by default",
			guard: Guard{lookupIP: fakeLookup},
			host:  "internal.example",
			want:  false,
		},
		{
			name:  "link-local metadata host blocked",
			guard: Guard{lookupIP: fakeLookup},
			host:  "meta.example",
			want:  false,
		},
		{
			name:  "loopback literal blocked by default",
			guard: Guard{lookupIP: fakeLookup},
			host:  "127.0.0.1",
			want:  false,
		},
		{
			name:  "localhost blocked by default",
			guard: Guard{lookupIP: fakeLookup},
			host:  "localhost",
			want:  false,
		},
		{
			name:  "allow-private admits loopback",
			guard: Guard{AllowPrivate: true, lookupIP: fakeLookup},
			host:  "127.0.0.1",
			want:  true,
		},
		{
			name:  "allowlist admits private host",
			guard: Guard{Allowlist: []string{"internal.example"}, lookupIP: fakeLookup},
			host:  "internal.example",
			want:  true,
		},
		{
			name:  "unresolvable host blocked",
			guard: Guard{lookupIP: fakeLookup},
			host:  "nonexistent.example",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, reason := tt.guard.Allow(tt.host)
			if got != tt.want {
				t.Fatalf("Guard.Allow(%q) = %v (%q), want %v", tt.host, got, reason, tt.want)
			}
		})
	}
}
