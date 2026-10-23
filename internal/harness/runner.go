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
	"bytes"
	"context"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/a2aproject/a2a-cli/internal/agentdir"
)

// Runner decides where a harness process runs.
type Runner interface {
	// Command returns the process which runs c.
	Command(ctx context.Context, c Command) (*exec.Cmd, error)
}

// LocalRunner runs the harness directly on the host, for trusted setups or
// when Docker isn't available.
type LocalRunner struct{}

var _ Runner = LocalRunner{}

// Command implements [Runner].
func (LocalRunner) Command(ctx context.Context, c Command) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, c.Path, c.Args...)
	cmd.Dir = c.Dir
	cmd.Env = append(os.Environ(), c.Env...)
	return cmd, nil
}

// Limits are container resource limits.
type Limits struct {
	Memory string
	CPUs   string
	PIDs   int
}

// DockerRunner runs the harness in a container with the workspace mounted.
type DockerRunner struct {
	// Name is the container name.
	Name   string
	Image  string
	Mounts []agentdir.Mount
	// Hidden are host paths which appear as empty read-only directories
	// wherever a mount exposes them, such as the agent's .a2a directory.
	Hidden []string
	// Home is the host directory mounted at /home/agent.
	Home string
	// Env holds NAME=value pairs passed into the container. Values are passed
	// through the environment of the docker CLI, not its arguments, so they
	// don't show up in process listings.
	Env    []string
	Limits Limits
	Labels map[string]string
	// Rootless is set for rootless Docker, where container root is the host
	// user, so files the agent writes stay owned by you.
	Rootless bool
}

var _ Runner = (*DockerRunner)(nil)

// ContainerHome is the home directory of the harness in the container.
const ContainerHome = "/home/agent"

// Paths in the container where the directory of the socket the harness
// reaches the server through, and the a2a CLI, are mounted.
const (
	SandboxDir = "/run/a2a"
	SandboxCLI = "/usr/local/bin/a2a"
)

// startScript removes a container left behind by a crash, which would keep
// the name taken, and replaces itself with docker run. Signals sent to the
// process group reach the docker CLI, which forwards them to the container.
const startScript = `docker rm -f "$0" >/dev/null 2>&1; exec docker run --name "$0" "$@"`

// Command implements [Runner].
func (r *DockerRunner) Command(ctx context.Context, c Command) (*exec.Cmd, error) {
	args, env := r.runArgs(c)
	cmd := exec.CommandContext(ctx, "sh", append([]string{"-c", startScript, r.Name}, args...)...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Dir = os.TempDir()
	return cmd, nil
}

// runArgs returns the arguments of docker run after --name, and the
// environment of the docker CLI.
func (r *DockerRunner) runArgs(c Command) ([]string, []string) {
	limits := r.Limits
	if limits.Memory == "" {
		limits.Memory = "4g"
	}
	if limits.CPUs == "" {
		limits.CPUs = "2"
	}
	if limits.PIDs == 0 {
		limits.PIDs = 512
	}
	user := fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
	if r.Rootless {
		user = "0:0"
	}
	args := []string{
		"--rm", "-i", "--init",
		"--user", user,
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"--memory", limits.Memory, "--cpus", limits.CPUs, "--pids-limit", strconv.Itoa(limits.PIDs),
		"--add-host", "host.docker.internal:host-gateway",
	}
	for _, k := range slices.Sorted(maps.Keys(r.Labels)) {
		args = append(args, "--label", k+"="+r.Labels[k])
	}
	for _, m := range r.Mounts {
		spec := m.Src + ":" + m.Dst
		if m.ReadOnly {
			spec += ":ro"
		}
		args = append(args, "-v", spec)
	}
	for _, dst := range hiddenDsts(r.Mounts, r.Hidden) {
		args = append(args, "--mount", "type=tmpfs,dst="+dst+",readonly")
	}
	if r.Home != "" {
		args = append(args, "-v", r.Home+":"+ContainerHome)
	}
	args = append(args, "-e", "HOME="+ContainerHome)
	if c.Dir != "" {
		args = append(args, "-w", c.Dir)
	}
	var env []string
	for _, kv := range append(append([]string{}, r.Env...), c.Env...) {
		name, _, ok := strings.Cut(kv, "=")
		if !ok || name == "" {
			continue
		}
		args = append(args, "-e", name)
		env = append(env, kv)
	}
	args = append(args, r.Image, c.Path)
	return append(args, c.Args...), env
}

// hiddenDsts returns the container paths at which mounts expose the hidden
// host paths.
func hiddenDsts(mounts []agentdir.Mount, hidden []string) []string {
	var out []string
	for _, h := range hidden {
		for _, m := range mounts {
			rel, err := filepath.Rel(m.Src, h)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				continue
			}
			dst := filepath.Join(m.Dst, rel)
			if !slices.Contains(out, dst) {
				out = append(out, dst)
			}
		}
	}
	return out
}

// Stop removes the container if it is still running.
func (r *DockerRunner) Stop(ctx context.Context) error {
	out, err := exec.CommandContext(ctx, "docker", "rm", "-f", r.Name).CombinedOutput()
	if err != nil && !bytes.Contains(out, []byte("No such container")) {
		return fmt.Errorf("docker rm -f %s: %w: %s", r.Name, err, bytes.TrimSpace(out))
	}
	return nil
}

// CheckDocker reports whether the docker daemon is reachable and whether it
// runs rootless.
func CheckDocker(ctx context.Context) (rootless bool, err error) {
	if _, err := exec.LookPath("docker"); err != nil {
		return false, fmt.Errorf("docker is not installed; install Docker or run with --sandbox local: %w", err)
	}
	out, err := exec.CommandContext(ctx, "docker", "info", "--format", "{{.SecurityOptions}}").Output()
	if err != nil {
		return false, fmt.Errorf("docker is not running; start Docker or run with --sandbox local: %w", exitErr(err))
	}
	return bytes.Contains(out, []byte("rootless")), nil
}

func exitErr(err error) error {
	if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
		return fmt.Errorf("%w: %s", err, bytes.TrimSpace(ee.Stderr))
	}
	return err
}
