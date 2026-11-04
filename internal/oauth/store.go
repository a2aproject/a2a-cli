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

package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Store keeps grants. Reads may happen at any time; callers hold Lock around a
// read-modify-write so concurrent CLI processes don't lose rotated tokens.
type Store interface {
	// List returns every grant.
	List() ([]Grant, error)
	// Get returns the grant for key, or nil when there is none.
	Get(key GrantKey) (*Grant, error)
	// Put adds or replaces the grant with the same key.
	Put(g Grant) error
	// Delete removes the grant for key, if any.
	Delete(key GrantKey) error
	// Lock takes the store's cross-process lock and returns its release function.
	Lock(ctx context.Context) (func(), error)
}

// DefaultDir returns ~/.config/a2a-cli/oauth.
func DefaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".config", "a2a-cli", "oauth"), nil
}

const (
	grantsFileName = "grants.json"
	lockFileName   = "grants.lock"
	storeVersion   = 1
	lockRetry      = 25 * time.Millisecond
)

// FileStore keeps grants, tokens included, in Dir/grants.json, readable only by
// the user (0600, in a 0700 directory).
type FileStore struct {
	Dir string
	// Name replaces grants.json as the file name, so stores with different
	// formats in one directory don't read each other's grants.
	Name string
}

func (s *FileStore) path() string {
	name := s.Name
	if name == "" {
		name = grantsFileName
	}
	return filepath.Join(s.Dir, name)
}

var _ Store = (*FileStore)(nil)

type grantsFile struct {
	Version int     `json:"version"`
	Grants  []Grant `json:"grants"`
}

// List implements [Store].
func (s *FileStore) List() ([]Grant, error) {
	data, err := os.ReadFile(s.path())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading grants: %w", err)
	}
	var f grantsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", s.path(), err)
	}
	if f.Version > storeVersion {
		return nil, fmt.Errorf("%s was written by a newer a2a (version %d)", s.path(), f.Version)
	}
	return f.Grants, nil
}

// Get implements [Store].
func (s *FileStore) Get(key GrantKey) (*Grant, error) {
	grants, err := s.List()
	if err != nil {
		return nil, err
	}
	for _, g := range grants {
		if g.Key.Equal(key) {
			return &g, nil
		}
	}
	return nil, nil
}

// Put implements [Store].
func (s *FileStore) Put(g Grant) error {
	grants, err := s.List()
	if err != nil {
		return err
	}
	replaced := false
	for i := range grants {
		if grants[i].Key.Equal(g.Key) {
			grants[i] = g
			replaced = true
		}
	}
	if !replaced {
		grants = append(grants, g)
	}
	return s.write(grants)
}

// Delete implements [Store].
func (s *FileStore) Delete(key GrantKey) error {
	grants, err := s.List()
	if err != nil {
		return err
	}
	kept := grants[:0]
	for _, g := range grants {
		if !g.Key.Equal(key) {
			kept = append(kept, g)
		}
	}
	return s.write(kept)
}

func (s *FileStore) write(grants []Grant) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", s.Dir, err)
	}
	if grants == nil {
		grants = []Grant{}
	}
	data, err := json.MarshalIndent(grantsFile{Version: storeVersion, Grants: grants}, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.Dir, filepath.Base(s.path())+".*.tmp")
	if err != nil {
		return fmt.Errorf("writing grants: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if _, err := os.Stat(tmpName); err == nil {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil && !errors.Is(err, errors.ErrUnsupported) {
		_ = tmp.Close()
		return fmt.Errorf("writing grants: %w", err)
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing grants: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing grants: %w", err)
	}
	if err := os.Rename(tmpName, s.path()); err != nil {
		return fmt.Errorf("writing grants: %w", err)
	}
	return nil
}

// Lock implements [Store] with an advisory lock on Dir/grants.lock. The OS
// releases it when the process exits, so a crashed or interrupted process
// never leaves the store locked.
func (s *FileStore) Lock(ctx context.Context) (func(), error) {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating %s: %w", s.Dir, err)
	}
	f, err := os.OpenFile(filepath.Join(s.Dir, lockFileName), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("locking grants: %w", err)
	}
	for {
		locked, err := tryLockFile(f)
		if err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("locking grants: %w", err)
		}
		if locked {
			return func() {
				_ = unlockFile(f)
				_ = f.Close()
			}, nil
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, fmt.Errorf("locking grants: %w", ctx.Err())
		case <-time.After(lockRetry):
		}
	}
}
