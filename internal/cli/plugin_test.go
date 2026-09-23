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

package cli

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/clicfg"
	"github.com/a2aproject/a2a-cli/internal/flagparse"
	"github.com/a2aproject/a2a-cli/internal/output"
)

func TestPluginListStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		deps         deps
		args         []string
		wantContains []string
		wantAbsent   []string
	}{
		{
			name:         "disabled by default",
			deps:         deps{cfgLoader: clicfg.LoadEmpty},
			args:         []string{"plugin", "list"},
			wantContains: []string{"Command plugins: disabled", "set-enabled true"},
		},
		{
			name:         "enabled via config",
			deps:         deps{cfgLoader: pluginsEnabledLoader()},
			args:         []string{"plugin", "list"},
			wantContains: []string{"Command plugins: enabled"},
			wantAbsent:   []string{"set-enabled true"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, err := runCMDWithConfig(t, tc.deps, tc.args...)
			if err != nil {
				t.Fatalf("runCMDWithConfig(%v) error = %v", tc.args, err)
			}
			for _, want := range tc.wantContains {
				if !strings.Contains(out, want) {
					t.Fatalf("plugin list = %q, want it to contain %q", out, want)
				}
			}
			for _, absent := range tc.wantAbsent {
				if strings.Contains(out, absent) {
					t.Fatalf("plugin list = %q, want it to not contain %q", out, absent)
				}
			}
		})
	}
}

func TestPluginSetEnabled(t *testing.T) {
	tests := []struct {
		name             string
		seed             string
		arg              string
		wantErrContains  string
		wantFileContains string
	}{
		{
			name:             "enable writes true",
			arg:              "true",
			wantFileContains: "plugins-enabled: true",
		},
		{
			name:             "disable overwrites existing",
			seed:             "plugins-enabled: true\n",
			arg:              "false",
			wantFileContains: "plugins-enabled: false",
		},
		{
			name:            "invalid argument",
			arg:             "notabool",
			wantErrContains: "invalid boolean",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			path := filepath.Join(home, ".config", "a2a-cli", "config.yaml")

			if tc.seed != "" {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatalf("os.MkdirAll(%q) error = %v", filepath.Dir(path), err)
				}
				if err := os.WriteFile(path, []byte(tc.seed), 0o600); err != nil {
					t.Fatalf("os.WriteFile(%q) error = %v", path, err)
				}
			}

			_, err := runCMD(t, "plugin", "set-enabled", tc.arg)

			if tc.wantErrContains != "" {
				if err == nil {
					t.Fatalf("runCMD(plugin set-enabled %s) error = nil, want a usage error", tc.arg)
				}
				if !strings.Contains(err.Error(), tc.wantErrContains) {
					t.Fatalf("runCMD(plugin set-enabled %s) error = %v, want it to contain %q", tc.arg, err, tc.wantErrContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("runCMD(plugin set-enabled %s) error = %v", tc.arg, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("os.ReadFile(%q) error = %v", path, err)
			}
			got := string(data)
			if !strings.Contains(got, tc.wantFileContains) {
				t.Fatalf("config.yaml = %q, want it to contain %q", got, tc.wantFileContains)
			}
		})
	}
}

func TestCommandPluginGating(t *testing.T) {
	dir := t.TempDir()
	cmdName := "foo"
	writeTestExecutable(t, filepath.Join(dir, "a2a-"+cmdName), "#!/bin/sh\n")
	t.Setenv("PATH", dir)

	tests := []struct {
		name           string
		deps           deps
		wantRegistered bool
	}{
		{
			name:           "not registered while disabled",
			deps:           deps{cfgLoader: clicfg.LoadEmpty},
			wantRegistered: false,
		},
		{
			name:           "registered while enabled",
			deps:           deps{cfgLoader: pluginsEnabledLoader()},
			wantRegistered: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &globalConfig{
				Printer:   output.NewPrinter(io.Discard, output.ModeText),
				svcParams: &flagparse.ServiceParams{},
			}
			root := mustNewRoot(t, cfg, tc.deps)

			gotRegistered := slices.ContainsFunc(root.Commands(), func(cmd *cobra.Command) bool {
				return cmd.Name() == cmdName
			})
			if gotRegistered != tc.wantRegistered {
				t.Fatalf("hasSubcommand(root, %q) = %v, want %v", cmdName, gotRegistered, tc.wantRegistered)
			}
		})
	}
}

func pluginsEnabledLoader() cfgLoaderFunc {
	return func(clicfg.LoadOpts) (*clicfg.Store, error) {
		return clicfg.Load(clicfg.LoadOpts{
			LookupEnv: func(k string) (string, bool) {
				if k == "A2ACLI_PLUGINS_ENABLED" {
					return "true", true
				}
				return "", false
			},
		})
	}
}

func writeTestExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", path, err)
	}
}
