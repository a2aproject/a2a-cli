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

package clicfg

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadUserConfigPrecedence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		env       map[string]string
		local     map[string]string
		user      string
		global    map[string]string
		wantValue string
		wantKind  SourceKind
	}{
		{
			name:      "user file loaded",
			user:      "plugins-enabled: true\n",
			wantValue: "true",
			wantKind:  SourceUserFile,
		},
		{
			name:      "local wins over user",
			local:     map[string]string{"A2ACLI_PLUGINS_ENABLED": "false"},
			user:      "plugins-enabled: true\n",
			wantValue: "false",
			wantKind:  SourceLocalFile,
		},
		{
			name:      "user wins over global",
			user:      "plugins-enabled: true\n",
			global:    map[string]string{"A2ACLI_PLUGINS_ENABLED": "false"},
			wantValue: "true",
			wantKind:  SourceUserFile,
		},
		{
			name:      "env wins over user",
			env:       map[string]string{"A2ACLI_PLUGINS_ENABLED": "false"},
			user:      "plugins-enabled: true\n",
			wantValue: "false",
			wantKind:  SourceEnv,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			opts := LoadOpts{WorkingDir: dir, LookupEnv: makeEnv(tc.env)}
			if tc.local != nil {
				writeDotenv(t, filepath.Join(dir, ".env"), tc.local)
			}
			if tc.user != "" {
				opts.UserPath = filepath.Join(dir, "config.yaml")
				if err := os.WriteFile(opts.UserPath, []byte(tc.user), 0o600); err != nil {
					t.Fatalf("os.WriteFile(%q) error = %v", opts.UserPath, err)
				}
			}
			if tc.global != nil {
				opts.GlobalPath = filepath.Join(dir, ".env.global")
				writeDotenv(t, opts.GlobalPath, tc.global)
			}

			store, err := Load(opts)
			if err != nil {
				t.Fatalf("Load() error = %v, want nil", err)
			}

			value, source, ok := store.Lookup("plugins-enabled")
			if !ok {
				t.Fatalf("store.Lookup(%q) ok = false, want true", "plugins-enabled")
			}
			if value != tc.wantValue {
				t.Errorf("store.Lookup(%q) = %v, want %v", "plugins-enabled", value, tc.wantValue)
			}
			if source.Kind != tc.wantKind {
				t.Errorf("store.Lookup(%q) source.Kind = %v, want %v", "plugins-enabled", source.Kind, tc.wantKind)
			}
		})
	}
}

func TestLoadUserConfigMissingIsNotAnError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store, err := Load(LoadOpts{
		WorkingDir: dir,
		UserPath:   filepath.Join(dir, "config.yaml"),
		GlobalPath: filepath.Join(dir, ".env.global"),
		LookupEnv:  makeEnv(nil),
	})
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if _, _, ok := store.Lookup("plugins-enabled"); ok {
		t.Fatalf("store.Lookup(%q) ok = true, want false", "plugins-enabled")
	}
}

func TestSetUserValueRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.yaml")

	if err := SetUserValue(path, "plugins-enabled", true); err != nil {
		t.Fatalf("SetUserValue() error = %v, want nil", err)
	}

	store, err := Load(LoadOpts{WorkingDir: dir, UserPath: path, LookupEnv: makeEnv(nil)})
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	value, source, ok := store.Lookup("plugins-enabled")
	if !ok {
		t.Fatalf("store.Lookup(%q) ok = false, want true", "plugins-enabled")
	}
	if value != "true" {
		t.Errorf("store.Lookup(%q) = %v, want %v", "plugins-enabled", value, "true")
	}
	if source.Kind != SourceUserFile {
		t.Errorf("store.Lookup(%q) source.Kind = %v, want %v", "plugins-enabled", source.Kind, SourceUserFile)
	}
}

func TestSetUserValuePreservesExistingKeys(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("agent-card: https://kept.example.com\n"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", path, err)
	}

	if err := SetUserValue(path, "plugins-enabled", true); err != nil {
		t.Fatalf("SetUserValue() error = %v, want nil", err)
	}

	store, err := Load(LoadOpts{WorkingDir: dir, UserPath: path, LookupEnv: makeEnv(nil)})
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if value, _, ok := store.Lookup("agent-card"); !ok || value != "https://kept.example.com" {
		t.Fatalf("store.Lookup(%q) = (%v, %v), want (%v, true)", "agent-card", value, ok, "https://kept.example.com")
	}
	if value, _, ok := store.Lookup("plugins-enabled"); !ok || value != "true" {
		t.Fatalf("store.Lookup(%q) = (%v, %v), want (%v, true)", "plugins-enabled", value, ok, "true")
	}
}
