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

package discover

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/a2aproject/a2a-cli/internal/atomicfile"
)

const (
	sessionLockStale = 5 * time.Second
	sessionLockWait  = 500 * time.Millisecond
	sessionMaxAge    = 7 * 24 * time.Hour
)

// sessionState is what a harness session has already been told, so the hook
// adds each piece of context once.
type sessionState struct {
	// ContactsDigest identifies the contacts block last injected.
	ContactsDigest string `json:"contactsDigest,omitempty"`
	// Injected maps origins of discovered agents already injected to when.
	Injected map[string]time.Time `json:"injected,omitempty"`
	// Pending maps origins awaiting a background probe to when it was started.
	Pending map[string]time.Time `json:"pending,omitempty"`
}

// sessionStore keeps one state file per session id under dir.
type sessionStore struct {
	dir string
}

type openSession struct {
	path  string
	lock  *lockFile
	state sessionState
}

// open loads the state of session, holding its lock until close. When the lock
// can't be taken in time it proceeds without it: a duplicate injection beats
// stalling the harness.
func (s sessionStore) open(session string) *openSession {
	base := filepath.Join(s.dir, hashName(session))
	o := &openSession{path: base + ".json"}
	if l, ok := lockWait(base+".lock", sessionLockStale, sessionLockWait); ok {
		o.lock = l
	}
	if data, err := os.ReadFile(o.path); err == nil {
		if err := json.Unmarshal(data, &o.state); err != nil {
			o.state = sessionState{}
		}
	}
	if o.state.Injected == nil {
		o.state.Injected = map[string]time.Time{}
	}
	if o.state.Pending == nil {
		o.state.Pending = map[string]time.Time{}
	}
	return o
}

func (o *openSession) close() error {
	var errs []error
	data, err := json.Marshal(o.state)
	if err != nil {
		errs = append(errs, err)
	} else if err := atomicfile.Write(o.path, data, 0o600); err != nil {
		errs = append(errs, err)
	}
	if o.lock != nil {
		if err := o.lock.unlock(); err != nil {
			errs = append(errs, err)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("saving session state: %w", err)
	}
	return nil
}

// prune deletes state files of sessions idle for longer than sessionMaxAge.
func (s sessionStore) prune(now time.Time) error {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var errs []error
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil || now.Sub(info.ModTime()) < sessionMaxAge {
			continue
		}
		if err := os.Remove(filepath.Join(s.dir, e.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
