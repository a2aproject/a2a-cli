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

// Package harness describes how a coding agent harness is started as an ACP
// agent (Profile) and where its process runs (Runner).
package harness

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
)

// Workspace is what a harness works on.
type Workspace struct {
	// Dir is the workspace directory. It has the same path on the host and
	// in a sandbox, because ACP sessions are created with it as cwd.
	Dir string
	// Home is the host directory holding the harness home: sessions,
	// caches and config. Runners which isolate the harness mount it as its home.
	Home string
}

// Command describes a harness process to start.
type Command struct {
	Path string
	Args []string
	// Dir is the working directory.
	Dir string
	// Env holds NAME=value pairs added to the environment of the harness.
	Env []string
}

// Profile describes how to start one harness as an ACP agent.
type Profile interface {
	// Name is the harness name, such as opencode.
	Name() string
	// Command returns the command which starts the harness in ACP mode.
	Command(ws Workspace) Command
	// Image is the container image with the harness installed.
	Image() string
	// Bootstrap writes harness config into ws.Home before the first start,
	// so that every tool permission is asked and the server's rules decide.
	Bootstrap(ws Workspace) error
}

// ProfileNames lists the known harness profiles.
func ProfileNames() []string { return slices.Sorted(maps.Keys(profiles)) }

var profiles = map[string]func() Profile{
	"opencode": func() Profile { return opencode{} },
	"claude":   func() Profile { return claude{} },
}

// Lookup returns the profile of a harness. A non-empty command replaces the
// profile's command line, which also allows harnesses without a profile.
func Lookup(name string, command []string) (Profile, error) {
	if len(command) > 0 {
		var base Profile
		if mk, ok := profiles[name]; ok {
			base = mk()
		}
		return custom{name: name, argv: command, base: base}, nil
	}
	mk, ok := profiles[name]
	if !ok {
		return nil, fmt.Errorf("unknown harness %q: known harnesses are %v, or pass --command for any ACP agent", name, ProfileNames())
	}
	return mk(), nil
}

// ImageRepo is where harness images are published.
const ImageRepo = "ghcr.io/a2aproject/harness-"

// ImageTag is the tag of the harness images this build uses.
const ImageTag = "latest"

// opencodePermission makes opencode ask before tools that change things,
// instead of its default of approving them.
const opencodePermission = `{"bash":"ask","edit":"ask","webfetch":"ask"}`

type opencode struct{}

func (opencode) Name() string  { return "opencode" }
func (opencode) Image() string { return ImageRepo + "opencode:" + ImageTag }

func (opencode) Command(ws Workspace) Command {
	return Command{
		Path: "opencode",
		Args: []string{"acp"},
		Dir:  ws.Dir,
		Env:  []string{"OPENCODE_PERMISSION=" + opencodePermission, "CLOUDCODE_PERMISSION=" + opencodePermission},
	}
}

func (opencode) Bootstrap(ws Workspace) error {
	var permission map[string]any
	if err := json.Unmarshal([]byte(opencodePermission), &permission); err != nil {
		return err
	}
	return writeJSONIfMissing(filepath.Join(ws.Home, ".config", "opencode", "opencode.json"), map[string]any{
		"$schema":    "https://opencode.ai/config.json",
		"permission": permission,
		"autoupdate": false,
	})
}

type claude struct{}

func (claude) Name() string  { return "claude" }
func (claude) Image() string { return ImageRepo + "claude:" + ImageTag }

func (claude) Command(ws Workspace) Command {
	return Command{Path: "claude-agent-acp", Dir: ws.Dir}
}

func (claude) Bootstrap(ws Workspace) error {
	return writeJSONIfMissing(filepath.Join(ws.Home, ".claude", "settings.json"), map[string]any{
		"permissions": map[string]any{"defaultMode": "default"},
	})
}

// custom is a harness started with an explicit command line. It keeps the
// image and bootstrap of the profile it replaces, if any.
type custom struct {
	name string
	argv []string
	base Profile
}

func (c custom) Name() string { return c.name }

func (c custom) Image() string {
	if c.base != nil {
		return c.base.Image()
	}
	return ""
}

func (c custom) Command(ws Workspace) Command {
	cmd := Command{Path: c.argv[0], Args: slices.Clone(c.argv[1:]), Dir: ws.Dir}
	if c.base != nil {
		cmd.Env = c.base.Command(ws).Env
	}
	return cmd
}

func (c custom) Bootstrap(ws Workspace) error {
	if c.base != nil {
		return c.base.Bootstrap(ws)
	}
	return nil
}

// writeJSONIfMissing keeps config the user changed after the first start.
func writeJSONIfMissing(path string, v any) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}
