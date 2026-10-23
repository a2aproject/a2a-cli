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

// Package agentdir manages the .a2a directory of an agent server: the agent
// definition in config.yaml, which can be committed, and the state directory
// with the database, secrets, network identity and harness home, which never is.
package agentdir

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Name is the name of the directory which marks an agent.
const Name = ".a2a"

// ErrNotFound is returned by [Find] when no agent directory is found.
var ErrNotFound = errors.New("no .a2a directory found here or in any parent directory; start an agent with `a2a server <harness>` or pass --dir")

// Dir is the .a2a directory of one agent.
type Dir struct {
	// Root is the directory which holds .a2a, the default workspace.
	Root string
}

// Open returns the agent directory rooted at root without checking that it exists.
func Open(root string) (Dir, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return Dir{}, err
	}
	return Dir{Root: abs}, nil
}

// Find walks up from start to the first directory holding .a2a/config.yaml,
// the way git finds a repository.
func Find(start string) (Dir, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return Dir{}, err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, Name, configFile)); err == nil {
			return Dir{Root: dir}, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return Dir{}, ErrNotFound
		}
		dir = parent
	}
}

const configFile = "config.yaml"

// Path returns the .a2a directory.
func (d Dir) Path() string { return filepath.Join(d.Root, Name) }

// ConfigPath returns the path of config.yaml.
func (d Dir) ConfigPath() string { return filepath.Join(d.Path(), configFile) }

// StateDir returns the directory which is never committed.
func (d Dir) StateDir() string { return filepath.Join(d.Path(), "state") }

// DBPath returns the path of the SQLite database.
func (d Dir) DBPath() string { return filepath.Join(d.StateDir(), "server.db") }

// SecretsPath returns the path of the secrets file.
func (d Dir) SecretsPath() string { return filepath.Join(d.StateDir(), "secrets") }

// TSNetDir returns the directory of the Tailscale node identity.
func (d Dir) TSNetDir() string { return filepath.Join(d.StateDir(), "tsnet") }

// HomeDir returns the harness home, mounted at /home/agent in a container.
func (d Dir) HomeDir() string { return filepath.Join(d.StateDir(), "home") }

// LogPath returns the path of the server log.
func (d Dir) LogPath() string { return filepath.Join(d.StateDir(), "server.log") }

// PIDPath returns the path of the file holding the PID of the running server.
func (d Dir) PIDPath() string { return filepath.Join(d.StateDir(), "server.pid") }

// maxSocketPath stays under the sun_path limit of Linux (108) and macOS (104).
const maxSocketPath = 100

// SocketPath returns the path of the control socket. It is state/server.sock
// unless that path is too long for a Unix socket, in which case it is a path
// in the runtime directory derived from the agent directory.
func (d Dir) SocketPath() string {
	p := filepath.Join(d.StateDir(), "server.sock")
	if len(p) <= maxSocketPath {
		return p
	}
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		base = filepath.Join(os.TempDir(), fmt.Sprintf("a2a-%d", os.Getuid()))
	}
	return filepath.Join(base, "a2a-"+d.ID()+".sock")
}

// HarnessSocketDir returns the directory of the socket through which a
// sandboxed harness reaches the server. Like the control socket, it moves to
// the runtime directory when the path is too long for a Unix socket.
func (d Dir) HarnessSocketDir() string {
	p := filepath.Join(d.StateDir(), "harness")
	if len(p)+len("/a2a.sock") <= maxSocketPath {
		return p
	}
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		base = filepath.Join(os.TempDir(), fmt.Sprintf("a2a-%d", os.Getuid()))
	}
	return filepath.Join(base, "a2a-"+d.ID()+"-harness")
}

// ID is a short stable identifier of the agent directory, used in container
// and service names.
func (d Dir) ID() string {
	sum := sha256.Sum256([]byte(d.Root))
	return hex.EncodeToString(sum[:])[:10]
}

// Init creates the .a2a and state directories and the .gitignore which keeps
// state out of version control.
func (d Dir) Init() error {
	for _, p := range []string{d.Path(), d.StateDir(), d.HomeDir()} {
		if err := os.MkdirAll(p, 0o700); err != nil {
			return err
		}
	}
	gi := filepath.Join(d.Path(), ".gitignore")
	if _, err := os.Stat(gi); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(gi, []byte("state/\n"), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Exists reports whether the agent has a config.yaml.
func (d Dir) Exists() bool {
	_, err := os.Stat(d.ConfigPath())
	return err == nil
}

// DefaultName derives an agent name from the root directory name.
func (d Dir) DefaultName() string {
	return SanitizeName(filepath.Base(d.Root))
}

// SanitizeName turns s into a lowercase DNS label, usable as a tailnet
// hostname and in container names.
func SanitizeName(s string) string {
	var sb strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			sb.WriteRune(r)
			dash = false
		case !dash && sb.Len() > 0:
			sb.WriteByte('-')
			dash = true
		}
	}
	out := strings.TrimSuffix(sb.String(), "-")
	if len(out) > 63 {
		out = strings.TrimSuffix(out[:63], "-")
	}
	if out == "" {
		return "agent"
	}
	return out
}
