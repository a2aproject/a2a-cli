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

package agentdir

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestFind(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	d, err := Open(root)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := d.SaveConfig(&Config{Name: "coder", Harness: "opencode"}); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := Find(nested)
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if got.Root != root {
		t.Fatalf("Find() = %v, want %v", got.Root, root)
	}

	if _, err := Find(t.TempDir()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Find() error = %v, want %v", err, ErrNotFound)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	t.Parallel()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	want := &Config{
		Name:     "coder",
		Harness:  "opencode",
		Mounts:   []string{"/src/app", "/home/u/docs:/docs:ro"},
		Network:  NetworkConfig{Private: "tailscale"},
		Channels: ChannelsConfig{Telegram: &TelegramConfig{Token: "secret:BOT"}},
	}
	if err := d.SaveConfig(want); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}
	got, err := d.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("LoadConfig() wrong result (-want +got) diff = %s", diff)
	}
	gi, err := os.ReadFile(filepath.Join(d.Path(), ".gitignore"))
	if err != nil {
		t.Fatalf("reading .gitignore error = %v", err)
	}
	if string(gi) != "state/\n" {
		t.Fatalf(".gitignore = %q, want %q", gi, "state/\n")
	}
}

func TestParseMount(t *testing.T) {
	t.Parallel()
	tests := []struct {
		spec    string
		want    Mount
		wantErr bool
	}{
		{spec: "/src/app", want: Mount{Src: "/src/app", Dst: "/src/app"}},
		{spec: "/src/app:ro", want: Mount{Src: "/src/app", Dst: "/src/app", ReadOnly: true}},
		{spec: "/home/u/docs:/docs:ro", want: Mount{Src: "/home/u/docs", Dst: "/docs", ReadOnly: true}},
		{spec: "/a:/b", want: Mount{Src: "/a", Dst: "/b"}},
		{spec: "/a:rel", wantErr: true},
		{spec: ":/b", wantErr: true},
		{spec: "/a:/b:/c", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.spec, func(t *testing.T) {
			t.Parallel()
			got, err := ParseMount(tc.spec)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ParseMount() error = %v, want error %v", err, tc.wantErr)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("ParseMount() wrong result (-want +got) diff = %s", diff)
			}
		})
	}
}

func TestSanitizeName(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"My App":     "my-app",
		"coder":      "coder",
		"__x__y__":   "x-y",
		"":           "agent",
		"Ünïcode 42": "n-code-42",
	}
	for in, want := range tests {
		if got := SanitizeName(in); got != want {
			t.Errorf("SanitizeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSecrets(t *testing.T) {
	t.Setenv("A2A_TEST_TOKEN", "from-env")
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	ref, s, err := ResolveInput("env:A2A_TEST_TOKEN", "TELEGRAM_BOT_TOKEN")
	if err != nil {
		t.Fatalf("ResolveInput() error = %v", err)
	}
	if ref != "secret:A2A_TEST_TOKEN" {
		t.Fatalf("ResolveInput() ref = %q, want %q", ref, "secret:A2A_TEST_TOKEN")
	}
	if err := d.SaveSecrets(s); err != nil {
		t.Fatalf("SaveSecrets() error = %v", err)
	}
	ref2, s2, err := ResolveInput("literal", "OTHER")
	if err != nil {
		t.Fatalf("ResolveInput() error = %v", err)
	}
	if err := d.SaveSecrets(s2); err != nil {
		t.Fatalf("SaveSecrets() error = %v", err)
	}

	loaded, err := d.LoadSecrets()
	if err != nil {
		t.Fatalf("LoadSecrets() error = %v", err)
	}
	for ref, want := range map[string]string{ref: "from-env", ref2: "literal", "plain": "plain"} {
		got, err := loaded.Resolve(ref)
		if err != nil {
			t.Fatalf("Resolve(%q) error = %v", ref, err)
		}
		if got != want {
			t.Fatalf("Resolve(%q) = %q, want %q", ref, got, want)
		}
	}
	if _, err := loaded.Resolve("secret:MISSING"); err == nil {
		t.Fatalf("Resolve(missing) error = nil, want error")
	}
	info, err := os.Stat(d.SecretsPath())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("secrets mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestSettingsApply(t *testing.T) {
	t.Setenv("A2A_TEST_KEY", "sk-1")
	t.Setenv("A2A_TEST_BOT", "123:abc")
	cfg := &Config{Network: NetworkConfig{Public: "tailscale"}}
	secrets, err := Settings{
		Harness:  "opencode",
		Secrets:  []string{"A2A_TEST_KEY"},
		Private:  "tailscale",
		Public:   "none",
		Telegram: "env:A2A_TEST_BOT",
		Mounts:   []string{"/src/app", "/docs:/docs:ro"},
	}.Apply(cfg, "My App")
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	want := &Config{
		Name:     "my-app",
		Harness:  "opencode",
		Secrets:  []string{"A2A_TEST_KEY"},
		Mounts:   []string{"/src/app", "/docs:/docs:ro"},
		Network:  NetworkConfig{Private: "tailscale"},
		Channels: ChannelsConfig{Telegram: &TelegramConfig{Token: "secret:A2A_TEST_BOT"}},
	}
	if diff := cmp.Diff(want, cfg); diff != "" {
		t.Fatalf("Apply() wrong result (-want +got) diff = %s", diff)
	}
	if diff := cmp.Diff(Secrets{"A2A_TEST_KEY": "sk-1", "A2A_TEST_BOT": "123:abc"}, secrets); diff != "" {
		t.Fatalf("Apply() wrong secrets (-want +got) diff = %s", diff)
	}

	if _, err := (Settings{Secrets: []string{"A2A_TEST_UNSET"}}).Apply(cfg, ""); err == nil {
		t.Fatalf("Apply() with an unset secret error = nil, want error")
	}
	if _, err := (Settings{}).Apply(&Config{}, "x"); err == nil {
		t.Fatalf("Apply() without a harness error = nil, want error")
	}
}

func TestSettingsApplyNetworkSecrets(t *testing.T) {
	t.Setenv("A2A_TEST_TS", "tskey-1")
	tests := []struct {
		name        string
		authKey     string
		wantSecrets Secrets
		wantErr     bool
	}{
		{name: "value", authKey: "tskey-2", wantSecrets: Secrets{SecretTailscaleKey: "tskey-2"}},
		{name: "env", authKey: "env:A2A_TEST_TS", wantSecrets: Secrets{SecretTailscaleKey: "tskey-1"}},
		{name: "stored under its name", authKey: "secret:" + SecretTailscaleKey, wantSecrets: Secrets{}},
		{name: "other secret", authKey: "secret:OTHER", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Settings{Harness: "opencode", TailscaleAuthKey: tc.authKey}.Apply(&Config{}, "x")
			if (err != nil) != tc.wantErr {
				t.Fatalf("Apply() error = %v, want error %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if diff := cmp.Diff(tc.wantSecrets, got); diff != "" {
				t.Fatalf("Apply() wrong secrets (-want +got) diff = %s", diff)
			}
		})
	}
}
