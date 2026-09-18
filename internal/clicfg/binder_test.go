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
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/pflag"
)

func TestEnvVarName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		flag string
		want string
	}{
		{flag: "agent-card", want: "A2ACLI_AGENT_CARD"},
		{flag: "context-id", want: "A2ACLI_CONTEXT_ID"},
		{flag: "a2a-version", want: "A2ACLI_A2A_VERSION"},
		{flag: "output", want: "A2ACLI_OUTPUT"},
	}

	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			t.Parallel()
			if got := flagToEnvVar(tt.flag); got != tt.want {
				t.Fatalf("envVarName(%q) = %q, want %q", tt.flag, got, tt.want)
			}
		})
	}
}

type testRepeatableValue struct {
	vals *[]string
}

func (v *testRepeatableValue) Set(s string) error {
	*v.vals = append(*v.vals, s)
	return nil
}

func (v *testRepeatableValue) String() string {
	if v.vals == nil {
		return ""
	}
	return strings.Join(*v.vals, ",")
}

func (v *testRepeatableValue) Type() string {
	return "string"
}

func TestApplyConfig(t *testing.T) {
	t.Parallel()

	newFlags := func() (*pflag.FlagSet, map[string]*string) {
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		vals := map[string]*string{}
		vals["agent-card"] = fs.String("agent-card", "", "")
		vals["tenant"] = fs.String("tenant", "", "")
		fs.StringArray("transport", nil, "")
		fs.Bool("insecure", false, "")
		fs.Bool("stream", false, "")
		fs.String("config", "", "")
		return fs, vals
	}

	store := func(t *testing.T, env map[string]string) *Store {
		t.Helper()
		dir := t.TempDir()
		s, err := Load(LoadOpts{
			WorkingDir: dir,
			GlobalPath: filepath.Join(dir, "none.env"),
			LookupEnv:  func(k string) (string, bool) { v, ok := env[k]; return v, ok },
		})
		if err != nil {
			t.Fatalf("config.Load() error = %v", err)
		}
		return s
	}

	t.Run("fills unset flags from the environment", func(t *testing.T) {
		t.Parallel()
		fs, vals := newFlags()
		s := store(t, map[string]string{"A2ACLI_AGENT_CARD": "https://example.com"})

		if _, err := Bind(fs, s); err != nil {
			t.Fatalf("applyConfig() error = %v", err)
		}
		if *vals["agent-card"] != "https://example.com" {
			t.Errorf("agent-card = %q, want %q", *vals["agent-card"], "https://example.com")
		}
		if !fs.Changed("agent-card") {
			t.Error("agent-card Changed = false, want true after applying config")
		}
	})

	t.Run("does not override a flag set on the command line", func(t *testing.T) {
		t.Parallel()
		fs, vals := newFlags()
		if err := fs.Set("agent-card", "https://from-flag.com"); err != nil {
			t.Fatalf("fs.Set() error = %v", err)
		}
		s := store(t, map[string]string{"A2ACLI_AGENT_CARD": "https://from-env.com"})

		if _, err := Bind(fs, s); err != nil {
			t.Fatalf("applyConfig() error = %v", err)
		}
		if *vals["agent-card"] != "https://from-flag.com" {
			t.Errorf("agent-card = %q, want the command-line value %q", *vals["agent-card"], "https://from-flag.com")
		}
	})

	t.Run("splits a repeatable flag on commas", func(t *testing.T) {
		t.Parallel()
		fs, _ := newFlags()
		s := store(t, map[string]string{"A2ACLI_TRANSPORT": "rest,jsonrpc"})

		if _, err := Bind(fs, s); err != nil {
			t.Fatalf("applyConfig() error = %v", err)
		}
		got, err := fs.GetStringArray("transport")
		if err != nil {
			t.Fatalf("fs.GetStringArray() error = %v", err)
		}
		if diff := cmp.Diff([]string{"rest", "jsonrpc"}, got); diff != "" {
			t.Fatalf("transport wrong result (-want +got) diff = %s", diff)
		}
	})

	t.Run("excluded flags are never read from config", func(t *testing.T) {
		t.Parallel()
		fs, _ := newFlags()
		s := store(t, map[string]string{"A2ACLI_STREAM": "true", "A2ACLI_CONFIG": "/somewhere.env"})

		resolutions, err := Bind(fs, s)
		if err != nil {
			t.Fatalf("applyConfig() error = %v", err)
		}
		if stream, _ := fs.GetBool("stream"); stream {
			t.Error("stream = true, want false: --stream must not be read from config")
		}
		for _, r := range resolutions {
			if r.Name == "stream" || r.Name == "config" {
				t.Errorf("resolutions include excluded flag %q", r.Name)
			}
		}
	})

	t.Run("binds flags from json file", func(t *testing.T) {
		t.Parallel()
		fs, vals := newFlags()
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "config.json")
		if err := os.WriteFile(cfgPath, []byte(`{
			"agent-card": "https://json-agent.example.com",
			"transport": ["rest", "jsonrpc"],
			"insecure": true
		}`), 0o600); err != nil {
			t.Fatalf("os.WriteFile(%q) error = %v", cfgPath, err)
		}

		s, err := Load(LoadOpts{
			ConfigPath: cfgPath,
			WorkingDir: dir,
			LookupEnv:  makeEnv(nil),
		})
		if err != nil {
			t.Fatalf("Load() error = %v, want nil", err)
		}

		resolutions, err := Bind(fs, s)
		if err != nil {
			t.Fatalf("Bind() error = %v, want nil", err)
		}

		if *vals["agent-card"] != "https://json-agent.example.com" {
			t.Fatalf("agent-card = %v, want %v", *vals["agent-card"], "https://json-agent.example.com")
		}
		transports, err := fs.GetStringArray("transport")
		if err != nil {
			t.Fatalf("fs.GetStringArray() error = %v", err)
		}
		if diff := cmp.Diff([]string{"rest", "jsonrpc"}, transports); diff != "" {
			t.Fatalf("fs.GetStringArray() wrong result (-want +got) diff = %s", diff)
		}
		insecure, err := fs.GetBool("insecure")
		if err != nil {
			t.Fatalf("fs.GetBool() error = %v", err)
		}
		if !insecure {
			t.Fatalf("fs.GetBool(\"insecure\") = %v, want %v", insecure, true)
		}

		sources := map[string]string{}
		paths := map[string]string{}
		for _, r := range resolutions {
			sources[r.Name] = r.Source
			paths[r.Name] = r.Path
		}
		if sources["agent-card"] != "local-file" {
			t.Fatalf("sources[\"agent-card\"] = %v, want %v", sources["agent-card"], "local-file")
		}
		if paths["agent-card"] != cfgPath {
			t.Fatalf("paths[\"agent-card\"] = %v, want %v", paths["agent-card"], cfgPath)
		}
	})

	t.Run("binds flags from json file specifying only agent-card", func(t *testing.T) {
		t.Parallel()
		fs, vals := newFlags()
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "only-card.json")
		if err := os.WriteFile(cfgPath, []byte(`{"agent-card": "https://json-only-card.example.com"}`), 0o600); err != nil {
			t.Fatalf("os.WriteFile(%q) error = %v", cfgPath, err)
		}

		s, err := Load(LoadOpts{
			ConfigPath: cfgPath,
			WorkingDir: dir,
			LookupEnv:  makeEnv(nil),
		})
		if err != nil {
			t.Fatalf("Load() error = %v, want nil", err)
		}

		resolutions, err := Bind(fs, s)
		if err != nil {
			t.Fatalf("Bind() error = %v, want nil", err)
		}

		if *vals["agent-card"] != "https://json-only-card.example.com" {
			t.Fatalf("agent-card = %v, want %v", *vals["agent-card"], "https://json-only-card.example.com")
		}
		if *vals["tenant"] != "" {
			t.Fatalf("tenant = %v, want empty", *vals["tenant"])
		}
		transports, err := fs.GetStringArray("transport")
		if err != nil {
			t.Fatalf("fs.GetStringArray() error = %v", err)
		}
		if len(transports) != 0 {
			t.Fatalf("transport = %v, want empty", transports)
		}
		insecure, err := fs.GetBool("insecure")
		if err != nil {
			t.Fatalf("fs.GetBool() error = %v", err)
		}
		if insecure {
			t.Fatalf("fs.GetBool(\"insecure\") = %v, want false", insecure)
		}

		sources := map[string]string{}
		for _, r := range resolutions {
			sources[r.Name] = r.Source
		}
		if sources["agent-card"] != "local-file" {
			t.Fatalf("sources[\"agent-card\"] = %v, want %v", sources["agent-card"], "local-file")
		}
		if sources["tenant"] != "default" {
			t.Fatalf("sources[\"tenant\"] = %v, want default", sources["tenant"])
		}
		if sources["transport"] != "default" {
			t.Fatalf("sources[\"transport\"] = %v, want default", sources["transport"])
		}
	})

	t.Run("binds flags from yaml file specifying only agent-card", func(t *testing.T) {
		t.Parallel()
		fs, vals := newFlags()
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "only-card.yaml")
		if err := os.WriteFile(cfgPath, []byte("agent-card: https://yaml-only-card.example.com\n"), 0o600); err != nil {
			t.Fatalf("os.WriteFile(%q) error = %v", cfgPath, err)
		}

		s, err := Load(LoadOpts{
			ConfigPath: cfgPath,
			WorkingDir: dir,
			LookupEnv:  makeEnv(nil),
		})
		if err != nil {
			t.Fatalf("Load() error = %v, want nil", err)
		}

		resolutions, err := Bind(fs, s)
		if err != nil {
			t.Fatalf("Bind() error = %v, want nil", err)
		}

		if *vals["agent-card"] != "https://yaml-only-card.example.com" {
			t.Fatalf("agent-card = %v, want %v", *vals["agent-card"], "https://yaml-only-card.example.com")
		}
		if *vals["tenant"] != "" {
			t.Fatalf("tenant = %v, want empty", *vals["tenant"])
		}
		transports, err := fs.GetStringArray("transport")
		if err != nil {
			t.Fatalf("fs.GetStringArray() error = %v", err)
		}
		if len(transports) != 0 {
			t.Fatalf("transport = %v, want empty", transports)
		}
		insecure, err := fs.GetBool("insecure")
		if err != nil {
			t.Fatalf("fs.GetBool() error = %v", err)
		}
		if insecure {
			t.Fatalf("fs.GetBool(\"insecure\") = %v, want false", insecure)
		}

		sources := map[string]string{}
		for _, r := range resolutions {
			sources[r.Name] = r.Source
		}
		if sources["agent-card"] != "local-file" {
			t.Fatalf("sources[\"agent-card\"] = %v, want %v", sources["agent-card"], "local-file")
		}
		if sources["tenant"] != "default" {
			t.Fatalf("sources[\"tenant\"] = %v, want default", sources["tenant"])
		}
		if sources["transport"] != "default" {
			t.Fatalf("sources[\"transport\"] = %v, want default", sources["transport"])
		}
	})

	t.Run("ignores additional properties in json and yaml files", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name     string
			filename string
			content  string
		}{
			{
				name:     "json with additional properties",
				filename: "extra.json",
				content: `{
					"agent-card": "https://json-extra.example.com",
					"unknown-flag": "foo",
					"extra_metadata": {"key": "value"},
					"custom-number": 42
				}`,
			},
			{
				name:     "yaml with additional properties",
				filename: "extra.yaml",
				content: `agent-card: https://yaml-extra.example.com
unknown-flag: foo
extra_metadata:
  key: value
custom-number: 42
`,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				fs, vals := newFlags()
				dir := t.TempDir()
				cfgPath := filepath.Join(dir, tc.filename)
				if err := os.WriteFile(cfgPath, []byte(tc.content), 0o600); err != nil {
					t.Fatalf("os.WriteFile(%q) error = %v", cfgPath, err)
				}

				s, err := Load(LoadOpts{
					ConfigPath: cfgPath,
					WorkingDir: dir,
					LookupEnv:  makeEnv(nil),
				})
				if err != nil {
					t.Fatalf("Load() error = %v, want nil", err)
				}

				resolutions, err := Bind(fs, s)
				if err != nil {
					t.Fatalf("Bind() error = %v, want nil", err)
				}

				if !strings.Contains(*vals["agent-card"], "extra.example.com") {
					t.Fatalf("agent-card = %v, want to contain extra.example.com", *vals["agent-card"])
				}

				for _, r := range resolutions {
					if r.Name == "unknown-flag" || r.Name == "extra_metadata" || r.Name == "custom-number" {
						t.Fatalf("resolutions contain unexpected property %q", r.Name)
					}
				}
			})
		}
	})

	t.Run("binds flags from yaml file", func(t *testing.T) {
		t.Parallel()
		fs, vals := newFlags()
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "config.yaml")
		yamlContent := `agent-card: https://yaml-agent.example.com
transport:
  - rest
  - jsonrpc
insecure: true
tenant: yaml-tenant
`
		if err := os.WriteFile(cfgPath, []byte(yamlContent), 0o600); err != nil {
			t.Fatalf("os.WriteFile(%q) error = %v", cfgPath, err)
		}

		s, err := Load(LoadOpts{
			ConfigPath: cfgPath,
			WorkingDir: dir,
			LookupEnv:  makeEnv(nil),
		})
		if err != nil {
			t.Fatalf("Load() error = %v, want nil", err)
		}

		if _, err := Bind(fs, s); err != nil {
			t.Fatalf("Bind() error = %v, want nil", err)
		}

		if *vals["agent-card"] != "https://yaml-agent.example.com" {
			t.Fatalf("agent-card = %v, want %v", *vals["agent-card"], "https://yaml-agent.example.com")
		}
		if *vals["tenant"] != "yaml-tenant" {
			t.Fatalf("tenant = %v, want %v", *vals["tenant"], "yaml-tenant")
		}
		transports, err := fs.GetStringArray("transport")
		if err != nil {
			t.Fatalf("fs.GetStringArray() error = %v", err)
		}
		if diff := cmp.Diff([]string{"rest", "jsonrpc"}, transports); diff != "" {
			t.Fatalf("fs.GetStringArray() wrong result (-want +got) diff = %s", diff)
		}
		insecure, err := fs.GetBool("insecure")
		if err != nil {
			t.Fatalf("fs.GetBool() error = %v", err)
		}
		if !insecure {
			t.Fatalf("fs.GetBool(\"insecure\") = %v, want %v", insecure, true)
		}
	})

	t.Run("binds repeatable flag from yaml list of strings", func(t *testing.T) {
		t.Parallel()
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		var collected []string
		fs.Var(&testRepeatableValue{vals: &collected}, "svc-param", "")

		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "config.yaml")
		yamlContent := `svc-param:
  - "key1=val1"
  - "key2=val2"
`
		if err := os.WriteFile(cfgPath, []byte(yamlContent), 0o600); err != nil {
			t.Fatalf("os.WriteFile(%q) error = %v", cfgPath, err)
		}

		s, err := Load(LoadOpts{
			ConfigPath: cfgPath,
			WorkingDir: dir,
			LookupEnv:  makeEnv(nil),
		})
		if err != nil {
			t.Fatalf("Load() error = %v, want nil", err)
		}

		if _, err := Bind(fs, s); err != nil {
			t.Fatalf("Bind() error = %v, want nil", err)
		}

		want := []string{"key1=val1", "key2=val2"}
		if diff := cmp.Diff(want, collected); diff != "" {
			t.Fatalf("svc-param wrong result (-want +got) diff = %s", diff)
		}
	})

	t.Run("env overrides yaml config", func(t *testing.T) {
		t.Parallel()
		fs, vals := newFlags()
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "config.yaml")
		if err := os.WriteFile(cfgPath, []byte("agent-card: https://yaml.example.com\n"), 0o600); err != nil {
			t.Fatalf("os.WriteFile(%q) error = %v", cfgPath, err)
		}

		s, err := Load(LoadOpts{
			ConfigPath: cfgPath,
			WorkingDir: dir,
			LookupEnv:  makeEnv(map[string]string{"A2ACLI_AGENT_CARD": "https://env.example.com"}),
		})
		if err != nil {
			t.Fatalf("Load() error = %v, want nil", err)
		}

		resolutions, err := Bind(fs, s)
		if err != nil {
			t.Fatalf("Bind() error = %v, want nil", err)
		}
		if *vals["agent-card"] != "https://env.example.com" {
			t.Fatalf("agent-card = %v, want %v", *vals["agent-card"], "https://env.example.com")
		}
		for _, r := range resolutions {
			if r.Name == "agent-card" && r.Source != "env" {
				t.Fatalf("r.Source = %v, want %v", r.Source, "env")
			}
		}
	})

	t.Run("cli flag overrides yaml config", func(t *testing.T) {
		t.Parallel()
		fs, vals := newFlags()
		if err := fs.Set("agent-card", "https://flag.example.com"); err != nil {
			t.Fatalf("fs.Set() error = %v", err)
		}
		dir := t.TempDir()
		cfgPath := filepath.Join(dir, "config.yaml")
		if err := os.WriteFile(cfgPath, []byte("agent-card: https://yaml.example.com\n"), 0o600); err != nil {
			t.Fatalf("os.WriteFile(%q) error = %v", cfgPath, err)
		}

		s, err := Load(LoadOpts{
			ConfigPath: cfgPath,
			WorkingDir: dir,
			LookupEnv:  makeEnv(nil),
		})
		if err != nil {
			t.Fatalf("Load() error = %v, want nil", err)
		}

		resolutions, err := Bind(fs, s)
		if err != nil {
			t.Fatalf("Bind() error = %v, want nil", err)
		}
		if *vals["agent-card"] != "https://flag.example.com" {
			t.Fatalf("agent-card = %v, want %v", *vals["agent-card"], "https://flag.example.com")
		}
		for _, r := range resolutions {
			if r.Name == "agent-card" && r.Source != "flag" {
				t.Fatalf("r.Source = %v, want %v", r.Source, "flag")
			}
		}
	})

	t.Run("records the source of each setting", func(t *testing.T) {
		t.Parallel()
		fs, _ := newFlags()
		if err := fs.Set("tenant", "flag-tenant"); err != nil {
			t.Fatalf("fs.Set() error = %v", err)
		}
		s := store(t, map[string]string{"A2ACLI_AGENT_CARD": "https://example.com"})

		resolutions, err := Bind(fs, s)
		if err != nil {
			t.Fatalf("applyConfig() error = %v", err)
		}
		got := map[string]string{}
		for _, r := range resolutions {
			got[r.Name] = r.Source
		}
		want := map[string]string{
			"agent-card": "env",
			"tenant":     "flag",
			"transport":  "default",
			"insecure":   "default",
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("resolution sources wrong result (-want +got) diff = %s", diff)
		}
	})
}

