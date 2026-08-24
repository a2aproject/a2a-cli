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

package flagparse

import (
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

func TestMetadataSet(t *testing.T) {
	t.Parallel()

	t.Run("valid object", func(t *testing.T) {
		t.Parallel()
		var m Metadata
		if err := m.Set(`{"k":"v"}`); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
		if m.Map()["k"] != "v" {
			t.Fatalf("Map()[k] = %v, want %q", m.Map()["k"], "v")
		}
	})

	t.Run("repeated flags merge, later keys win", func(t *testing.T) {
		t.Parallel()
		var m Metadata
		_ = m.Set(`{"a":1,"b":2}`)
		_ = m.Set(`{"b":3,"c":4}`)
		got := m.Map()
		if got["a"].(float64) != 1 || got["b"].(float64) != 3 || got["c"].(float64) != 4 {
			t.Fatalf("Map() = %v, want a=1 b=3 c=4", got)
		}
	})

	t.Run("rejects non-object JSON", func(t *testing.T) {
		t.Parallel()
		for _, in := range []string{`[1,2]`, `"str"`, `5`, `true`} {
			var m Metadata
			if err := m.Set(in); err == nil {
				t.Errorf("Set(%q) should error", in)
			}
		}
	})

	t.Run("rejects malformed JSON", func(t *testing.T) {
		t.Parallel()
		var m Metadata
		if err := m.Set(`{bad`); err == nil {
			t.Fatal("Set(`{bad`) should error")
		}
	})

	t.Run("Map is nil when unused", func(t *testing.T) {
		t.Parallel()
		var m Metadata
		if m.Map() != nil {
			t.Fatalf("Map() = %v, want nil", m.Map())
		}
	})

	t.Run("ApplyTo sets request metadata", func(t *testing.T) {
		t.Parallel()
		var m Metadata
		if err := m.Set(`{"trace":"123"}`); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
		req := &a2a.SendMessageRequest{}
		m.ApplyTo(req)
		if req.Metadata["trace"] != "123" {
			t.Fatalf("req.Metadata[trace] = %v, want %q", req.Metadata["trace"], "123")
		}
	})

	t.Run("ApplyTo is a no-op when unused", func(t *testing.T) {
		t.Parallel()
		var m Metadata
		req := &a2a.CancelTaskRequest{}
		m.ApplyTo(req)
		if req.Metadata != nil {
			t.Fatalf("req.Metadata = %v, want nil", req.Metadata)
		}
	})
}
