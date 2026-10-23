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

package harness

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/a2aproject/a2a-cli/internal/agentdir"
	"github.com/google/go-cmp/cmp"
)

func TestPrepareLocal(t *testing.T) {
	t.Parallel()
	ws, home := t.TempDir(), t.TempDir()
	profile, err := Lookup("opencode", []string{"my-acp", "--flag"})
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	p, err := Prepare(context.Background(), Spec{
		Profile:    profile,
		Sandbox:    SandboxLocal,
		Mounts:     []agentdir.Mount{{Src: ws, Dst: ws}},
		Home:       home,
		Env:        []string{"API_KEY=k"},
		Socket:     "/run/agent/a2a.sock",
		SocketHost: "a2a",
		CLI:        "/opt/a2a/bin/a2a",
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if !slices.Equal(p.Args, []string{"my-acp", "--flag"}) {
		t.Fatalf("Prepare() args = %v, want the custom command", p.Args)
	}
	for _, want := range []string{"API_KEY=k", EnvUnixSocket + "=a2a=/run/agent/a2a.sock", "PATH=/opt/a2a/bin" + string(os.PathListSeparator)} {
		if !slices.ContainsFunc(p.Env, func(kv string) bool { return strings.HasPrefix(kv, want) }) {
			t.Fatalf("Prepare() env is missing %q", want)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "opencode", "opencode.json")); err != nil {
		t.Fatalf("Prepare() did not bootstrap the harness home: %v", err)
	}
	if err := p.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestDockerCLIMount(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		cli  string
		goos string
		want []agentdir.Mount
	}{
		{name: "linux host", cli: "/opt/a2a", goos: "linux", want: []agentdir.Mount{{Src: "/opt/a2a", Dst: SandboxCLI, ReadOnly: true}}},
		{name: "macOS host keeps the image's binary", cli: "/opt/a2a", goos: "darwin"},
		{name: "windows host keeps the image's binary", cli: `C:\a2a.exe`, goos: "windows"},
		{name: "no binary", goos: "linux"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := dockerCLIMount(tc.cli, tc.goos)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("dockerCLIMount() wrong result (-want +got) diff = %s", diff)
			}
		})
	}
}

func TestPrepareRejectsInvalidSpecs(t *testing.T) {
	t.Parallel()
	ws := t.TempDir()
	tests := []struct {
		name    string
		spec    Spec
		wantErr string
	}{
		{name: "no workspace", spec: Spec{Sandbox: SandboxLocal}, wantErr: "no workspace"},
		{name: "moved workspace", spec: Spec{Sandbox: SandboxLocal, Mounts: []agentdir.Mount{{Src: ws, Dst: "/work"}}}, wantErr: "must keep its path"},
		{name: "unknown sandbox", spec: Spec{Sandbox: "vm", Mounts: []agentdir.Mount{{Src: ws, Dst: ws}}}, wantErr: "unknown sandbox"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.spec.Profile = claude{}
			tc.spec.Home = t.TempDir()
			if _, err := Prepare(context.Background(), tc.spec); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Prepare() error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}
