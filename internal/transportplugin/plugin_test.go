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

package transportplugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestDiscover(t *testing.T) {
	dir := t.TempDir()
	writeExecutable(t, filepath.Join(dir, "a2a-transport-foo"), "#!/bin/sh\n")
	writeExecutable(t, filepath.Join(dir, "a2a-transport-bar"), "#!/bin/sh\n")
	t.Setenv("PATH", dir)

	t.Run("finds installed plugin", func(t *testing.T) {
		got, err := findPlugin("foo")
		if err != nil {
			t.Fatalf("Discover(foo) error = %v", err)
		}
		if want := filepath.Join(dir, "a2a-transport-foo"); got != want {
			t.Fatalf("Discover(foo) = %q, want %q", got, want)
		}
	})

	t.Run("normalizes case to binary suffix", func(t *testing.T) {
		got, err := findPlugin("FOO")
		if err != nil {
			t.Fatalf("Discover(FOO) error = %v", err)
		}
		if want := filepath.Join(dir, "a2a-transport-foo"); got != want {
			t.Fatalf("Discover(FOO) = %q, want %q", got, want)
		}
	})

	t.Run("missing plugin lists available", func(t *testing.T) {
		_, err := findPlugin("missing")
		if err == nil {
			t.Fatal("Discover(missing) error = nil, want error")
		}
		for _, want := range []string{"a2a-transport-missing", "bar", "foo"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("Discover(missing) error = %v, want it to contain %q", err, want)
			}
		}
	})
}

func TestListQueriesInfo(t *testing.T) {
	dir := t.TempDir()
	writeExecutable(t, filepath.Join(dir, "a2a-transport-good"), infoScript(`{"name":"good","version":"1.2.3","protocol":"1.0","description":"a good plugin"}`))
	writeExecutable(t, filepath.Join(dir, "a2a-transport-broken"), "#!/bin/sh\nexit 3\n")
	t.Setenv("PATH", dir)

	got := List(t.Context())
	if len(got) != 2 {
		t.Fatalf("List() returned %d plugins, want 2", len(got))
	}

	byName := map[string]Discovered{}
	for _, d := range got {
		byName[d.Name] = d
	}

	good, ok := byName["good"]
	if !ok {
		t.Fatalf("List() missing 'good' plugin, got %+v", got)
	}
	if good.InfoErr != nil {
		t.Fatalf("List() good.InfoErr = %v, want nil", good.InfoErr)
	}
	wantInfo := struct {
		Name, Version, Protocol, Description string
	}{"good", "1.2.3", "1.0", "a good plugin"}
	gotInfo := struct {
		Name, Version, Protocol, Description string
	}{good.Info.Name, good.Info.Version, string(good.Info.Protocol), good.Info.Description}
	if diff := cmp.Diff(wantInfo, gotInfo, cmpopts.EquateEmpty()); diff != "" {
		t.Fatalf("List() good info wrong result (-want +got) diff = %s", diff)
	}

	broken, ok := byName["broken"]
	if !ok {
		t.Fatalf("List() missing 'broken' plugin, got %+v", got)
	}
	if broken.InfoErr == nil {
		t.Fatal("List() broken.InfoErr = nil, want an error")
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", path, err)
	}
}

func infoScript(json string) string {
	return "#!/bin/sh\nif [ \"$1\" = \"info\" ]; then\n  printf '%s\\n' '" + json + "'\nfi\n"
}
