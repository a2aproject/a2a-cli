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

// Package harness holds the pieces shared by harness executors: how a harness
// process is started (Runner), where the context-to-session mapping is kept
// (SessionStore) and who answers permission requests and questions (Questions).
//
// Executors for individual harnesses live in subpackages, such as a2aopencode
// and a2aclaude. They are meant to move to a2a-go/harness once the interfaces
// settle.
package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// Command describes a harness process to start.
type Command struct {
	Path string
	Args []string
	// Dir is the working directory of the harness, its workspace.
	Dir string
	// Env is appended to the environment of the runner.
	Env []string
}

// Runner decides where a harness process runs. A container runner can wrap
// the command in `podman run` with the workspace mounted.
type Runner interface {
	// Command returns the process to start for c. It must be stopped when
	// ctx is done.
	Command(ctx context.Context, c Command) (*exec.Cmd, error)
}

// LocalRunner runs the harness directly on the host.
type LocalRunner struct{}

var _ Runner = LocalRunner{}

// Command implements [Runner].
func (LocalRunner) Command(ctx context.Context, c Command) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, c.Path, c.Args...)
	cmd.Dir = c.Dir
	cmd.Env = append(os.Environ(), c.Env...)
	setProcessGroup(cmd)
	cmd.WaitDelay = 5 * time.Second
	return cmd, nil
}

// SessionStore maps A2A contexts to harness sessions, so that every task in a
// context continues the same harness conversation.
type SessionStore interface {
	// Get returns the harness session of an A2A context, if there is one.
	Get(ctx context.Context, contextID string) (sessionID string, ok bool, err error)
	// Put records the harness session of an A2A context.
	Put(ctx context.Context, contextID, sessionID string) error
}

// MemorySessions is a [SessionStore] which forgets everything on restart.
type MemorySessions struct {
	mu sync.Mutex
	m  map[string]string
}

var _ SessionStore = (*MemorySessions)(nil)

// NewMemorySessions creates an empty [MemorySessions].
func NewMemorySessions() *MemorySessions {
	return &MemorySessions{m: map[string]string{}}
}

// Get implements [SessionStore].
func (s *MemorySessions) Get(_ context.Context, contextID string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.m[contextID]
	return id, ok, nil
}

// Put implements [SessionStore].
func (s *MemorySessions) Put(_ context.Context, contextID, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[contextID] = sessionID
	return nil
}

// FileSessions is a [SessionStore] persisted to a JSON file, so that contexts
// survive a restart as long as the harness keeps its own session state.
type FileSessions struct {
	path string
	mu   sync.Mutex
	m    map[string]string
}

var _ SessionStore = (*FileSessions)(nil)

// NewFileSessions loads the mapping from path, or starts empty if it doesn't exist.
func NewFileSessions(path string) (*FileSessions, error) {
	s := &FileSessions{path: path, m: map[string]string{}}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &s.m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return s, nil
}

// Get implements [SessionStore].
func (s *FileSessions) Get(_ context.Context, contextID string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.m[contextID]
	return id, ok, nil
}

// Put implements [SessionStore].
func (s *FileSessions) Put(_ context.Context, contextID, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[contextID] = sessionID
	data, err := json.MarshalIndent(s.m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
