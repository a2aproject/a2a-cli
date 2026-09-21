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
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadStore(t *testing.T) {
	t.Parallel()

	nestedDir := "nested"
	tests := []struct {
		name      string
		key       string
		env       map[string]string
		local     map[string]string
		global    map[string]string
		custom    map[string]string
		wantValue string
		wantKind  SourceKind
	}{
		{
			name:      "no result",
			key:       "A2ACLI_MISSING",
			wantValue: "",
		},
		{
			name:      "env precedence",
			key:       "A2ACLI_AGENT_CARD",
			env:       map[string]string{"A2ACLI_AGENT_CARD": "env-card"},
			local:     map[string]string{"A2ACLI_AGENT_CARD": "local-card"},
			global:    map[string]string{"A2ACLI_AGENT_CARD": "global-card"},
			wantValue: "env-card",
			wantKind:  SourceEnv,
		},
		{
			name:      "local file precedence",
			key:       "A2ACLI_AGENT_CARD",
			local:     map[string]string{"A2ACLI_AGENT_CARD": "local-card"},
			global:    map[string]string{"A2ACLI_AGENT_CARD": "global-card"},
			wantValue: "local-card",
			wantKind:  SourceLocalFile,
		},
		{
			name:      "global file precedence",
			key:       "A2ACLI_AGENT_CARD",
			env:       map[string]string{},
			local:     map[string]string{},
			global:    map[string]string{"A2ACLI_AGENT_CARD": "global-card"},
			wantValue: "global-card",
			wantKind:  SourceGlobalFile,
		},
		{
			name:      "custom config path",
			key:       "A2ACLI_AGENT_CARD",
			local:     map[string]string{"A2ACLI_AGENT_CARD": "local-card"},
			custom:    map[string]string{"A2ACLI_AGENT_CARD": "custom-card"},
			wantValue: "custom-card",
			wantKind:  SourceLocalFile,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			nestedDirPath := filepath.Join(dir, nestedDir)
			if err := os.Mkdir(nestedDirPath, os.ModePerm); err != nil {
				t.Fatalf("os.Mkdir(%q) error = %v", nestedDirPath, err)
			}

			opts := LoadOpts{WorkingDir: dir, LookupEnv: makeEnv(tc.env)}
			if tc.custom != nil {
				opts.ConfigPath = filepath.Join(dir, ".env.custom")
				writeDotenv(t, opts.ConfigPath, tc.custom)
			}
			if tc.global != nil {
				opts.GlobalPath = filepath.Join(dir, ".env.global")
				writeDotenv(t, filepath.Join(dir, ".env.global"), tc.global)
			}
			if tc.local != nil {
				writeDotenv(t, filepath.Join(dir, ".env"), tc.local)
			}
			store, err := Load(opts)
			if err != nil {
				t.Fatalf("Load() error = %v, want nil", err)
			}

			value, source, ok := store.Lookup(tc.key)
			if ok != (tc.wantValue != "") {
				t.Fatalf("Lookup(%q) ok = %v, want %v", tc.key, ok, !ok)
			}
			if tc.wantValue == "" {
				return
			}
			if value != tc.wantValue {
				t.Errorf("Lookup(%q) value = %q, want %q", tc.key, value, tc.wantValue)
			}
			if source.Kind != tc.wantKind {
				t.Errorf("Lookup(%q) source.Kind = %v, want %v", tc.key, source.Kind, tc.wantKind)
			}
		})
	}
}

func TestLoadStoreErrors(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	emptyFile := filepath.Join(dir, ".env.empty")
	writeDotenv(t, emptyFile, nil)
	emptyJSON := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(emptyJSON, []byte(""), 0o600); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", emptyJSON, err)
	}
	emptyYAML := filepath.Join(dir, "empty.yaml")
	if err := os.WriteFile(emptyYAML, []byte(""), 0o600); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", emptyYAML, err)
	}
	invalidJSON := filepath.Join(dir, "invalid.json")
	if err := os.WriteFile(invalidJSON, []byte("{invalid json"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", invalidJSON, err)
	}
	invalidYAML := filepath.Join(dir, "invalid.yaml")
	if err := os.WriteFile(invalidYAML, []byte("key: [unclosed list"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", invalidYAML, err)
	}

	tests := []struct {
		name            string
		key             string
		opts            LoadOpts
		wantErrContains string
	}{
		{
			name:            "error if no custom config",
			key:             "A2ACLI_AGENT_CARD",
			opts:            LoadOpts{ConfigPath: "non-existent"},
			wantErrContains: "no such file or directory",
		},
		{
			name: "no error if empty config",
			key:  "A2ACLI_AGENT_CARD",
			opts: LoadOpts{ConfigPath: emptyFile},
		},
		{
			name: "no error if empty json config",
			key:  "A2ACLI_AGENT_CARD",
			opts: LoadOpts{ConfigPath: emptyJSON},
		},
		{
			name: "no error if empty yaml config",
			key:  "A2ACLI_AGENT_CARD",
			opts: LoadOpts{ConfigPath: emptyYAML},
		},
		{
			name:            "error if invalid json config",
			key:             "A2ACLI_AGENT_CARD",
			opts:            LoadOpts{ConfigPath: invalidJSON},
			wantErrContains: "parsing JSON config",
		},
		{
			name:            "error if invalid yaml config",
			key:             "A2ACLI_AGENT_CARD",
			opts:            LoadOpts{ConfigPath: invalidYAML},
			wantErrContains: "parsing YAML config",
		},
		{
			name: "no error if no local file",
			key:  "A2ACLI_AGENT_CARD",
			opts: LoadOpts{WorkingDir: dir},
		},
		{
			name: "no error if no global file",
			key:  "A2ACLI_AGENT_CARD",
			opts: LoadOpts{GlobalPath: "non-existent"},
		},
		{
			name: "no error if empty global file",
			key:  "A2ACLI_AGENT_CARD",
			opts: LoadOpts{GlobalPath: emptyFile},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := Load(tc.opts)
			if err != nil && tc.wantErrContains == "" {
				t.Fatalf("Load() error = %v, want nil", err)
			}
			if err == nil && tc.wantErrContains != "" {
				t.Fatalf("Load() error = nul, want erro containing %v", tc.wantErrContains)
			}
			if err != nil && !strings.Contains(err.Error(), tc.wantErrContains) {
				t.Fatalf("Load() error = %v, want to contain %q", err, tc.wantErrContains)
			}
		})
	}
}

func TestLoadWalksUpForLocalEnv(t *testing.T) {
	t.Parallel()

	key, wantVal := "A2ACLI_TENANT", "from-ancestor"
	root := t.TempDir()
	writeDotenv(t, filepath.Join(root, ".env"), map[string]string{key: wantVal})

	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	store, err := Load(LoadOpts{
		WorkingDir: nested,
		GlobalPath: filepath.Join(root, "missing-global.env"),
		LookupEnv:  makeEnv(nil),
	})
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	gotVal, _, ok := store.Lookup(key)
	if !ok {
		t.Fatalf("Lookup(%q) ok = false, want true", key)
	}
	if gotVal != wantVal {
		t.Errorf("Lookup(%q) value = %q, want %q", key, gotVal, wantVal)
	}
}

func TestLoadStoreJSONAndYAML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		filename  string
		content   string
		key       string
		wantValue string
		wantKind  SourceKind
	}{
		{
			name:      "load from json file",
			filename:  "config.json",
			content:   `{"agent-card": "https://json.example.com", "timeout": "10s"}`,
			key:       "agent-card",
			wantValue: "https://json.example.com",
			wantKind:  SourceLocalFile,
		},
		{
			name:      "load from yaml file",
			filename:  "config.yaml",
			content:   "agent-card: https://yaml.example.com\ntimeout: 15s\n",
			key:       "agent-card",
			wantValue: "https://yaml.example.com",
			wantKind:  SourceLocalFile,
		},
		{
			name:      "load from json specifying only agent-card",
			filename:  "only-agent-card.json",
			content:   `{"agent-card": "https://json-only.example.com"}`,
			key:       "agent-card",
			wantValue: "https://json-only.example.com",
			wantKind:  SourceLocalFile,
		},
		{
			name:      "load from yaml specifying only agent-card",
			filename:  "only-agent-card.yaml",
			content:   "agent-card: https://yaml-only.example.com\n",
			key:       "agent-card",
			wantValue: "https://yaml-only.example.com",
			wantKind:  SourceLocalFile,
		},
		{
			name:      "load from yml file",
			filename:  "config.yml",
			content:   "agent-card: https://yml.example.com\n",
			key:       "agent-card",
			wantValue: "https://yml.example.com",
			wantKind:  SourceLocalFile,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			configPath := filepath.Join(dir, tc.filename)
			if err := os.WriteFile(configPath, []byte(tc.content), 0o600); err != nil {
				t.Fatalf("os.WriteFile(%q) error = %v", configPath, err)
			}

			store, err := Load(LoadOpts{
				ConfigPath: configPath,
				WorkingDir: dir,
				LookupEnv:  makeEnv(nil),
			})
			if err != nil {
				t.Fatalf("Load() error = %v, want nil", err)
			}

			value, source, ok := store.Lookup(tc.key)
			if !ok {
				t.Fatalf("store.Lookup(%q) = false, want true", tc.key)
			}
			if value != tc.wantValue {
				t.Fatalf("store.Lookup(%q) = %v, want %v", tc.key, value, tc.wantValue)
			}
			if source.Kind != tc.wantKind {
				t.Fatalf("store.Lookup(%q) = %v, want %v", tc.key, source.Kind, tc.wantKind)
			}
		})
	}
}

func TestLoadStoreSpecifyingOnlyAgentCard(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		filename string
		content  string
	}{
		{
			name:     "json specifying only agent-card",
			filename: "config.json",
			content:  `{"agent-card": "https://json-only.example.com"}`,
		},
		{
			name:     "yaml specifying only agent-card",
			filename: "config.yaml",
			content:  "agent-card: https://yaml-only.example.com\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			configPath := filepath.Join(dir, tc.filename)
			if err := os.WriteFile(configPath, []byte(tc.content), 0o600); err != nil {
				t.Fatalf("os.WriteFile(%q) error = %v", configPath, err)
			}

			store, err := Load(LoadOpts{
				ConfigPath: configPath,
				WorkingDir: dir,
				LookupEnv:  makeEnv(nil),
			})
			if err != nil {
				t.Fatalf("Load() error = %v, want nil", err)
			}

			if _, _, ok := store.Lookup("agent-card"); !ok {
				t.Fatalf("store.Lookup(%q) = false, want true", "agent-card")
			}
			if _, _, ok := store.Lookup("A2ACLI_AGENT_CARD"); !ok {
				t.Fatalf("store.Lookup(%q) = false, want true", "A2ACLI_AGENT_CARD")
			}
			if val, _, ok := store.Lookup("timeout"); ok {
				t.Fatalf("store.Lookup(%q) = %v, want not ok", "timeout", val)
			}
			if val, _, ok := store.Lookup("A2ACLI_TIMEOUT"); ok {
				t.Fatalf("store.Lookup(%q) = %v, want not ok", "A2ACLI_TIMEOUT", val)
			}
			if val, _, ok := store.Lookup("transport"); ok {
				t.Fatalf("store.Lookup(%q) = %v, want not ok", "transport", val)
			}
		})
	}
}

func writeDotenv(t *testing.T, path string, content map[string]string) {
	t.Helper()
	var lines [][]byte
	for k, v := range content {
		kv := bytes.Join([][]byte{[]byte(k), []byte(v)}, []byte("="))
		lines = append(lines, kv)
	}
	if err := os.WriteFile(path, bytes.Join(lines, []byte("\n")), 0o600); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", path, err)
	}
}

func makeEnv(env map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := env[key]
		return v, ok
	}
}
