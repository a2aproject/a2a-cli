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
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"

	"github.com/a2aproject/a2a-cli/internal/agentdir"
)

// Sandboxes a harness can run in.
const (
	SandboxDocker = "docker"
	SandboxLocal  = "local"
)

// EnvUnixSocket tells the a2a CLI to send requests for one host through a
// Unix socket: <host>=<path>.
const EnvUnixSocket = "A2ACLI_UNIX_SOCKET"

// Spec describes a harness to run for an agent.
type Spec struct {
	Profile Profile
	// Sandbox is SandboxDocker (the default) or SandboxLocal.
	Sandbox string
	// Mounts are the directories the harness works on. The first is the
	// workspace and must keep its path in the sandbox.
	Mounts []agentdir.Mount
	// Home is the host directory holding the harness home.
	Home string
	// Hidden are host paths a sandbox hides even where a mount exposes them.
	Hidden []string
	// Env holds NAME=value pairs for the harness, such as API keys.
	Env []string

	// Name, Image, Limits and Labels configure the container. Image
	// defaults to the profile's.
	Name   string
	Image  string
	Limits Limits
	Labels map[string]string

	// Socket is the host path of the Unix socket through which the harness
	// reaches the agent server, and SocketHost the host name the a2a CLI
	// sends through it. Empty when the harness reaches the server over TCP.
	Socket     string
	SocketHost string
	// CLI is the host path of the a2a binary made available to the harness.
	CLI string

	// Progress receives the output of pulling or building the image.
	Progress io.Writer
}

// Prepared is a harness ready to start.
type Prepared struct {
	// Args, Env and Dir start the harness as an ACP agent.
	Args []string
	Env  []string
	Dir  string
	// Summary describes the sandbox for the user.
	Summary string
	// Stop cleans up after the harness exited, such as removing its container.
	Stop func(context.Context) error
}

// Prepare bootstraps the harness home and returns the command which runs
// the harness in its sandbox.
func Prepare(ctx context.Context, spec Spec) (*Prepared, error) {
	if len(spec.Mounts) == 0 {
		return nil, fmt.Errorf("no workspace to mount")
	}
	// ACP sessions get the workspace as cwd, and the executor starts the
	// agent there, so the workspace must have the same path on both sides.
	if ws := spec.Mounts[0]; ws.Dst != ws.Src {
		return nil, fmt.Errorf("the first --mount is the workspace and must keep its path (%s), got %s", ws.Src, ws.Dst)
	}
	ws := Workspace{Dir: spec.Mounts[0].Src, Home: spec.Home}
	if err := spec.Profile.Bootstrap(ws); err != nil {
		return nil, fmt.Errorf("bootstrapping %s: %w", spec.Profile.Name(), err)
	}
	switch spec.Sandbox {
	case "", SandboxDocker:
		return prepareDocker(ctx, spec, ws)
	case SandboxLocal:
		return prepareLocal(ctx, spec, ws)
	}
	return nil, fmt.Errorf("unknown sandbox %q: want docker or local", spec.Sandbox)
}

func prepareDocker(ctx context.Context, spec Spec, ws Workspace) (*Prepared, error) {
	rootless, err := CheckDocker(ctx)
	if err != nil {
		return nil, err
	}
	image := spec.Image
	if image == "" {
		image = spec.Profile.Image()
	}
	image, what, err := EnsureImage(ctx, image, spec.Profile.Name(), spec.Progress)
	if err != nil {
		return nil, err
	}
	dr := &DockerRunner{
		Name:     spec.Name,
		Image:    image,
		Mounts:   slices.Clone(spec.Mounts),
		Hidden:   spec.Hidden,
		Home:     spec.Home,
		Env:      slices.Clone(spec.Env),
		Limits:   spec.Limits,
		Labels:   spec.Labels,
		Rootless: rootless,
	}
	if spec.Socket != "" {
		dr.Mounts = append(dr.Mounts, agentdir.Mount{Src: filepath.Dir(spec.Socket), Dst: SandboxDir})
		dr.Env = append(dr.Env, EnvUnixSocket+"="+spec.SocketHost+"="+filepath.Join(SandboxDir, filepath.Base(spec.Socket)))
	}
	dr.Mounts = append(dr.Mounts, dockerCLIMount(spec.CLI, runtime.GOOS)...)
	cmd, err := dr.Command(ctx, spec.Profile.Command(ws))
	if err != nil {
		return nil, err
	}
	return &Prepared{Args: cmd.Args, Env: cmd.Env, Dir: cmd.Dir, Summary: "docker: " + what, Stop: dr.Stop}, nil
}

// dockerCLIMount mounts the server's binary over the image's a2a CLI. Only
// a Linux host's binary runs in the container; elsewhere the image's own
// build, made for the server's commit, is used.
func dockerCLIMount(cli, goos string) []agentdir.Mount {
	if cli == "" || goos != "linux" {
		return nil
	}
	return []agentdir.Mount{{Src: cli, Dst: SandboxCLI, ReadOnly: true}}
}

func prepareLocal(ctx context.Context, spec Spec, ws Workspace) (*Prepared, error) {
	c := spec.Profile.Command(ws)
	c.Env = append(c.Env, spec.Env...)
	if spec.CLI != "" {
		c.Env = append(c.Env, "PATH="+filepath.Dir(spec.CLI)+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	if spec.Socket != "" {
		c.Env = append(c.Env, EnvUnixSocket+"="+spec.SocketHost+"="+spec.Socket)
	}
	cmd, err := LocalRunner{}.Command(ctx, c)
	if err != nil {
		return nil, err
	}
	return &Prepared{Args: cmd.Args, Env: cmd.Env, Dir: cmd.Dir, Summary: "no sandbox", Stop: func(context.Context) error { return nil }}, nil
}

// TCPBridge reports whether a harness in sandbox must reach the agent
// server over TCP rather than a Unix socket, and where the server listens
// (bind) and the harness connects (dial). Docker Desktop can't mount a
// socket but forwards host.docker.internal to the host's loopback.
func TCPBridge(sandbox string) (bind, dial string, ok bool) {
	if sandbox == SandboxLocal || runtime.GOOS == "linux" {
		return "", "", false
	}
	return "127.0.0.1", "host.docker.internal", true
}
