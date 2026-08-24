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
	"path/filepath"
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
