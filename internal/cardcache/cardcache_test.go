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

package cardcache

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

func TestKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ref  string
		want string
	}{
		{ref: "https://Agent.example", want: "https://agent.example/.well-known/agent-card.json"},
		{ref: "https://agent.example/", want: "https://agent.example/.well-known/agent-card.json"},
		{ref: "https://agent.example/.well-known/agent-card.json", want: "https://agent.example/.well-known/agent-card.json"},
		{ref: "https://agent.example/custom/card.json#frag", want: "https://agent.example/custom/card.json"},
		{ref: "file:///tmp/card.json", want: "file:///tmp/card.json"},
	}

	for _, tc := range tests {
		t.Run(tc.ref, func(t *testing.T) {
			t.Parallel()
			if got := Key(tc.ref); got != tc.want {
				t.Fatalf("Key(%q) = %q, want %q", tc.ref, got, tc.want)
			}
		})
	}
}

func TestCachePutGet(t *testing.T) {
	t.Parallel()
	cache := New(t.TempDir())
	entry := Entry{
		Ref:       "https://agent.example",
		FetchedAt: time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC),
		Card:      &a2a.AgentCard{Name: "Agent", Skills: []a2a.AgentSkill{{ID: "s", Name: "Skill"}}},
	}
	if err := cache.Put(entry); err != nil {
		t.Fatalf("Cache.Put() error = %v", err)
	}

	got, ok := cache.Get("https://agent.example/.well-known/agent-card.json")
	if !ok {
		t.Fatalf("Cache.Get() found no entry")
	}
	if diff := cmp.Diff(entry, got); diff != "" {
		t.Fatalf("Cache.Get() wrong result (-want +got) diff = %s", diff)
	}
	if _, ok := cache.Get("https://other.example"); ok {
		t.Fatalf("Cache.Get(other) found an entry, want none")
	}
}

func TestNilCache(t *testing.T) {
	t.Parallel()
	var cache *Cache
	if err := cache.Put(Entry{Ref: "https://agent.example"}); err != nil {
		t.Fatalf("Cache.Put() error = %v", err)
	}
	if _, ok := cache.Get("https://agent.example"); ok {
		t.Fatalf("Cache.Get() found an entry in a nil cache")
	}
}
