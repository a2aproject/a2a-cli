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

package oauthstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2aclient/oauth"
)

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
// the user (0600, in a 0700 directory). It follows the [oauth.Store] contract:
// every call reads or writes the file, so results never share memory with the
// store, List returns the tokens too, and Get returns an error wrapping
// [oauth.ErrGrantNotFound] when there is no grant for the key.
type FileStore struct {
	Dir string
}

var (
	_ oauth.Store  = (*FileStore)(nil)
	_ oauth.Locker = (*FileStore)(nil)
)

type grantsFile struct {
	Version int           `json:"version"`
	Grants  []oauth.Grant `json:"grants"`
}

// List implements [oauth.Store].
func (s *FileStore) List(context.Context) ([]oauth.Grant, error) {
	return s.read()
}

func (s *FileStore) read() ([]oauth.Grant, error) {
	data, err := os.ReadFile(filepath.Join(s.Dir, grantsFileName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading grants: %w", err)
	}
	var f grantsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", filepath.Join(s.Dir, grantsFileName), err)
	}
	if f.Version > storeVersion {
		return nil, fmt.Errorf("%s was written by a newer a2a (version %d)", filepath.Join(s.Dir, grantsFileName), f.Version)
	}
	for i := range f.Grants {
		// Files written before Grant.Refreshable existed only have the token.
		f.Grants[i].Refreshable = f.Grants[i].Refreshable || f.Grants[i].RefreshToken != ""
	}
	return f.Grants, nil
}

// Get implements [oauth.Store].
func (s *FileStore) Get(_ context.Context, key oauth.GrantKey) (*oauth.Grant, error) {
	grants, err := s.read()
	if err != nil {
		return nil, err
	}
	for _, g := range grants {
		if g.Key.Equal(key) {
			return &g, nil
		}
	}
	return nil, oauth.ErrGrantNotFound
}

// Put implements [oauth.Store].
func (s *FileStore) Put(_ context.Context, g oauth.Grant) error {
	grants, err := s.read()
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

// Delete implements [oauth.Store].
func (s *FileStore) Delete(_ context.Context, key oauth.GrantKey) error {
	grants, err := s.read()
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

func (s *FileStore) write(grants []oauth.Grant) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", s.Dir, err)
	}
	if grants == nil {
		grants = []oauth.Grant{}
	}
	data, err := json.MarshalIndent(grantsFile{Version: storeVersion, Grants: grants}, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.Dir, grantsFileName+".*.tmp")
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
	if err := renameFile(tmpName, filepath.Join(s.Dir, grantsFileName)); err != nil {
		return fmt.Errorf("writing grants: %w", err)
	}
	return nil
}

// Lock implements [oauth.Locker] with an advisory lock on Dir/grants.lock. The OS
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
