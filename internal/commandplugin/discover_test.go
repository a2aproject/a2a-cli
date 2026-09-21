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

package commandplugin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestDiscover(t *testing.T) {
	dir := t.TempDir()
	writeExecutable(t, filepath.Join(dir, "a2a-foo"), "#!/bin/sh\n")
	writeExecutable(t, filepath.Join(dir, "a2a-bar"), "#!/bin/sh\n")
	// transport plugin must be excluded
	writeExecutable(t, filepath.Join(dir, "a2a-transport-baz"), "#!/bin/sh\n")
	// non-matching binary must be excluded
	writeExecutable(t, filepath.Join(dir, "kubectl-foo"), "#!/bin/sh\n")
	t.Setenv("PATH", dir)

	t.Run("finds installed plugins", func(t *testing.T) {
		got := discover()
		if len(got) != 2 {
			t.Fatalf("discover() returned %d plugins, want 2: %+v", len(got), got)
		}
		names := []string{got[0].Name, got[1].Name}
		want := []string{"bar", "foo"}
		if diff := cmp.Diff(want, names, cmpopts.SortSlices(func(a, b string) bool { return a < b })); diff != "" {
			t.Fatalf("discover() wrong names (-want +got) diff = %s", diff)
		}
	})

	t.Run("excludes transport plugins", func(t *testing.T) {
		got := discover()
		for _, d := range got {
			if d.Name == "transport-baz" {
				t.Fatalf("discover() included transport plugin %q, want it excluded", d.Name)
			}
		}
	})

	t.Run("first PATH match wins", func(t *testing.T) {
		dir2 := t.TempDir()
		writeExecutable(t, filepath.Join(dir2, "a2a-foo"), "#!/bin/sh\n")
		t.Setenv("PATH", dir+string(filepath.ListSeparator)+dir2)

		got := discover()
		for _, d := range got {
			if d.Name == "foo" {
				if d.Path != filepath.Join(dir, "a2a-foo") {
					t.Fatalf("discover() foo.Path = %q, want first match %q", d.Path, filepath.Join(dir, "a2a-foo"))
				}
				return
			}
		}
		t.Fatal("discover() missing 'foo' plugin")
	})
}

func TestListQueriesInfo(t *testing.T) {
	dir := t.TempDir()
	writeExecutable(t, filepath.Join(dir, "a2a-good"), infoScript(`{"name":"good","version":"1.0.0","description":"a good plugin"}`))
	writeExecutable(t, filepath.Join(dir, "a2a-broken"), "#!/bin/sh\nexit 3\n")
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
	wantInfo := struct{ Name, Version, Description string }{"good", "1.0.0", "a good plugin"}
	gotInfo := struct{ Name, Version, Description string }{good.Info.Name, good.Info.Version, good.Info.Description}
	if diff := cmp.Diff(wantInfo, gotInfo); diff != "" {
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

func TestDiscoverNonExecutableSkipped(t *testing.T) {
	dir := t.TempDir()
	// write file without execute bit
	path := filepath.Join(dir, "a2a-nox")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
	t.Setenv("PATH", dir)

	got := discover()
	for _, d := range got {
		if d.Name == "nox" {
			t.Fatalf("discover() included non-executable plugin %q", d.Name)
		}
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", path, err)
	}
}

func infoScript(jsonStr string) string {
	return "#!/bin/sh\nif [ \"$1\" = \"info\" ]; then\n  printf '%s\\n' '" + jsonStr + "'\nfi\n"
}
