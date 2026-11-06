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

package contacts

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestIsName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want bool
	}{
		{name: "nimbus", want: true},
		{name: "order-bot_2", want: true},
		{name: "9lives", want: true},
		{name: "", want: false},
		{name: "Nimbus", want: false},
		{name: "-leading", want: false},
		{name: "nimbus.coffee", want: false},
		{name: "host:8080", want: false},
		{name: "./card.json", want: false},
		{name: "localhost", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := IsName(tc.name); got != tc.want {
				t.Fatalf("IsName(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestOrigin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ref  string
		want string
	}{
		{ref: "https://Nimbus.example", want: "https://nimbus.example"},
		{ref: "http://localhost:8080/.well-known/agent-card.json", want: "http://localhost:8080"},
		{ref: "file:///tmp/card.json", want: ""},
		{ref: "not a url", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.ref, func(t *testing.T) {
			t.Parallel()
			if got := Origin(tc.ref); got != tc.want {
				t.Fatalf("Origin(%q) = %q, want %q", tc.ref, got, tc.want)
			}
		})
	}
}

func TestBookRoundTrip(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "a2a-cli", "contacts.yaml")
	added := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)

	book, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%q) error = %v", path, err)
	}
	for _, c := range []Contact{
		{Name: "zeta", Card: "https://zeta.example", AddedAt: added},
		{Name: "alpha", Card: "https://alpha.example", Note: "first", AddedAt: added},
	} {
		if err := book.Add(c, false); err != nil {
			t.Fatalf("Book.Add(%q) error = %v", c.Name, err)
		}
	}
	if err := book.Save(); err != nil {
		t.Fatalf("Book.Save() error = %v", err)
	}

	reloaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%q) error = %v", path, err)
	}
	want := []Contact{
		{Name: "alpha", Card: "https://alpha.example", Note: "first", AddedAt: added},
		{Name: "zeta", Card: "https://zeta.example", AddedAt: added},
	}
	if diff := cmp.Diff(want, reloaded.List()); diff != "" {
		t.Fatalf("Load().List() wrong result (-want +got) diff = %s", diff)
	}
}

func TestBookAddRemove(t *testing.T) {
	t.Parallel()
	book, err := Load(filepath.Join(t.TempDir(), "contacts.yaml"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	c := Contact{Name: "nimbus", Card: "https://nimbus.example"}
	if err := book.Add(c, false); err != nil {
		t.Fatalf("Book.Add() error = %v", err)
	}

	if err := book.Add(c, false); !errors.Is(err, ErrExists) {
		t.Fatalf("Book.Add(duplicate) error = %v, want %v", err, ErrExists)
	}
	c.Note = "replaced"
	if err := book.Add(c, true); err != nil {
		t.Fatalf("Book.Add(replace) error = %v", err)
	}
	if got, _ := book.Get("nimbus"); got.Note != "replaced" {
		t.Fatalf("Book.Get().Note = %q, want %q", got.Note, "replaced")
	}
	if err := book.Add(Contact{Name: "Bad.Name", Card: "x"}, false); err == nil {
		t.Fatalf("Book.Add(invalid name) error = nil, want error")
	}
	if err := book.Remove("nimbus"); err != nil {
		t.Fatalf("Book.Remove() error = %v", err)
	}
	if err := book.Remove("nimbus"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Book.Remove(missing) error = %v, want %v", err, ErrNotFound)
	}
}

func TestLoadSkipsInvalidEntries(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "contacts.yaml")
	content := "contacts:\n  ok:\n    card: https://ok.example\n  Bad.Name:\n    card: https://bad.example\n  nocard: {}\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	book, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := []Contact{{Name: "ok", Card: "https://ok.example"}}
	if diff := cmp.Diff(want, book.List()); diff != "" {
		t.Fatalf("Load().List() wrong result (-want +got) diff = %s", diff)
	}
}
