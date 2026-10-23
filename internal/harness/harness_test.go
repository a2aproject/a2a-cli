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
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/a2aproject/a2a-cli/internal/agentdir"
)

func TestDockerRunnerCommand(t *testing.T) {
	t.Parallel()
	r := &DockerRunner{
		Name:  "a2a-coder-123",
		Image: "img:1",
		Mounts: []agentdir.Mount{
			{Src: "/src/app", Dst: "/src/app"},
			{Src: "/home/u/docs", Dst: "/docs", ReadOnly: true},
		},
		Home: "/src/app/.a2a/state/home",
		Env:  []string{"OPENAI_API_KEY=sk-secret"},
	}
	cmd, err := r.Command(context.Background(), Command{Path: "opencode", Args: []string{"acp"}, Dir: "/src/app", Env: []string{"X=1"}})
	if err != nil {
		t.Fatalf("Command() error = %v", err)
	}
	want := []string{
		"sh", "-c", startScript, "a2a-coder-123",
		"--rm", "-i", "--init",
		"--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"--memory", "4g", "--cpus", "2", "--pids-limit", "512",
		"--add-host", "host.docker.internal:host-gateway",
		"-v", "/src/app:/src/app",
		"-v", "/home/u/docs:/docs:ro",
		"-v", "/src/app/.a2a/state/home:/home/agent",
		"-e", "HOME=/home/agent",
		"-w", "/src/app",
		"-e", "OPENAI_API_KEY",
		"-e", "X",
		"img:1", "opencode", "acp",
	}
	if diff := cmp.Diff(want, cmd.Args); diff != "" {
		t.Fatalf("Command() wrong result (-want +got) diff = %s", diff)
	}
	if strings.Contains(strings.Join(cmd.Args, " "), "sk-secret") {
		t.Fatalf("Command() args contain a secret value: %v", cmd.Args)
	}
	if !slices.Contains(cmd.Env, "OPENAI_API_KEY=sk-secret") {
		t.Fatalf("Command() env does not pass the secret to the docker CLI")
	}
}

func TestDockerRunnerHidden(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mounts []agentdir.Mount
		hidden []string
		want   []string
	}{
		{
			name:   "inside the workspace",
			mounts: []agentdir.Mount{{Src: "/src/app", Dst: "/src/app"}},
			hidden: []string{"/src/app/.a2a"},
			want:   []string{"type=tmpfs,dst=/src/app/.a2a,readonly"},
		},
		{
			name:   "under a mount with another destination",
			mounts: []agentdir.Mount{{Src: "/src/app", Dst: "/src/app"}, {Src: "/home/u", Dst: "/u", ReadOnly: true}},
			hidden: []string{"/home/u/proj/.a2a"},
			want:   []string{"type=tmpfs,dst=/u/proj/.a2a,readonly"},
		},
		{
			name:   "the mount itself",
			mounts: []agentdir.Mount{{Src: "/src/app/.a2a", Dst: "/cfg"}},
			hidden: []string{"/src/app/.a2a"},
			want:   []string{"type=tmpfs,dst=/cfg,readonly"},
		},
		{
			name:   "outside every mount",
			mounts: []agentdir.Mount{{Src: "/src/app", Dst: "/src/app"}},
			hidden: []string{"/src/application/.a2a", "/other/.a2a"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := &DockerRunner{Name: "n", Image: "img", Mounts: tc.mounts, Hidden: tc.hidden}
			cmd, err := r.Command(context.Background(), Command{Path: "opencode"})
			if err != nil {
				t.Fatalf("Command() error = %v", err)
			}
			var got []string
			for i, a := range cmd.Args {
				if a == "--mount" {
					got = append(got, cmd.Args[i+1])
				}
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("Command() wrong result (-want +got) diff = %s", diff)
			}
		})
	}
}

func TestLookup(t *testing.T) {
	t.Parallel()
	ws := Workspace{Dir: "/w", Home: "/h"}
	tests := []struct {
		name    string
		command []string
		want    Command
		image   string
		wantErr bool
	}{
		{name: "opencode", want: Command{Path: "opencode", Args: []string{"acp"}, Dir: "/w",
			Env: []string{"OPENCODE_PERMISSION=" + opencodePermission, "CLOUDCODE_PERMISSION=" + opencodePermission}},
			image: ImageRepo + "opencode:" + ImageTag},
		{name: "claude", want: Command{Path: "claude-agent-acp", Dir: "/w"}, image: ImageRepo + "claude:" + ImageTag},
		{name: "opencode", command: []string{"cloudcode", "acp"}, want: Command{Path: "cloudcode", Args: []string{"acp"}, Dir: "/w",
			Env: []string{"OPENCODE_PERMISSION=" + opencodePermission, "CLOUDCODE_PERMISSION=" + opencodePermission}},
			image: ImageRepo + "opencode:" + ImageTag},
		{name: "gemini", command: []string{"gemini", "--experimental-acp"}, want: Command{Path: "gemini", Args: []string{"--experimental-acp"}, Dir: "/w"}},
		{name: "unknown", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name+strings.Join(tc.command, "_"), func(t *testing.T) {
			t.Parallel()
			p, err := Lookup(tc.name, tc.command)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Lookup() error = %v, want error %v", err, tc.wantErr)
			}
			if err != nil {
				return
			}
			if diff := cmp.Diff(tc.want, p.Command(ws)); diff != "" {
				t.Fatalf("Command() wrong result (-want +got) diff = %s", diff)
			}
			if p.Image() != tc.image {
				t.Fatalf("Image() = %q, want %q", p.Image(), tc.image)
			}
		})
	}
}

func TestBootstrap(t *testing.T) {
	t.Parallel()
	tests := []struct {
		harness string
		file    string
		want    string
	}{
		{harness: "opencode", file: ".config/opencode/opencode.json", want: `"bash": "ask"`},
		{harness: "claude", file: ".claude/settings.json", want: `"defaultMode": "default"`},
	}
	for _, tc := range tests {
		t.Run(tc.harness, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			p, err := Lookup(tc.harness, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := p.Bootstrap(Workspace{Dir: t.TempDir(), Home: home}); err != nil {
				t.Fatalf("Bootstrap() error = %v", err)
			}
			data, err := os.ReadFile(filepath.Join(home, tc.file))
			if err != nil {
				t.Fatalf("reading %s error = %v", tc.file, err)
			}
			if !strings.Contains(string(data), tc.want) {
				t.Fatalf("Bootstrap() wrote %s, want it to contain %s", data, tc.want)
			}

			if err := os.WriteFile(filepath.Join(home, tc.file), []byte("{}"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := p.Bootstrap(Workspace{Home: home}); err != nil {
				t.Fatalf("Bootstrap() error = %v", err)
			}
			if data, _ := os.ReadFile(filepath.Join(home, tc.file)); string(data) != "{}" {
				t.Fatalf("Bootstrap() overwrote user config: %s", data)
			}
		})
	}
}

func TestDockerfiles(t *testing.T) {
	t.Parallel()
	for _, name := range ProfileNames() {
		if _, ok := Dockerfile(name); !ok {
			t.Errorf("Dockerfile(%q) missing", name)
		}
	}
}

func TestHarnessImageCLIVersion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		bi       debug.BuildInfo
		version  string
		localTag string
	}{
		{
			name: "built from a checkout",
			bi: debug.BuildInfo{
				Main:     debug.Module{Version: "v0.3.2-0.20261022190000-609d0b5dd28f+dirty"},
				Settings: []debug.BuildSetting{{Key: "vcs.revision", Value: "609d0b5dd28fe8870a51eadca75eb8f33a15c640"}},
			},
			version:  "609d0b5dd28fe8870a51eadca75eb8f33a15c640",
			localTag: "a2a-harness-opencode:609d0b5dd28fe8870a51eadca75eb8f33a15c640",
		},
		{
			name:     "go install of a release",
			bi:       debug.BuildInfo{Main: debug.Module{Version: "v0.3.1"}},
			version:  "v0.3.1",
			localTag: "a2a-harness-opencode:v0.3.1",
		},
		{
			name:     "dirty pseudo-version",
			bi:       debug.BuildInfo{Main: debug.Module{Version: "v0.3.2-0.20261022190000-609d0b5dd28f+dirty"}},
			version:  "v0.3.2-0.20261022190000-609d0b5dd28f+dirty",
			localTag: "a2a-harness-opencode:v0.3.2-0.20261022190000-609d0b5dd28f-dirty",
		},
		{
			name:     "unknown",
			bi:       debug.BuildInfo{Main: debug.Module{Version: "(devel)"}},
			version:  "latest",
			localTag: "a2a-harness-opencode:local",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := cliVersion(&tc.bi); got != tc.version {
				t.Fatalf("cliVersion() = %q, want %q", got, tc.version)
			}
			if got := LocalImage("opencode", tc.version); got != tc.localTag {
				t.Fatalf("LocalImage() = %q, want %q", got, tc.localTag)
			}
		})
	}
}
