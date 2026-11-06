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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// lockFile is a cross-process lock held by exclusively creating a file. A lock
// older than stale is treated as abandoned by a crashed process and broken.
type lockFile struct {
	path string
}

func tryLock(path string, stale time.Duration) (*lockFile, bool) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, false
	}
	for attempt := 0; attempt < 2; attempt++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			if err := f.Close(); err != nil {
				return nil, false
			}
			return &lockFile{path: path}, true
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, false
		}
		info, err := os.Stat(path)
		if err != nil || time.Since(info.ModTime()) < stale {
			return nil, false
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, false
		}
	}
	return nil, false
}

// lockWait retries tryLock until it succeeds or wait elapses.
func lockWait(path string, stale, wait time.Duration) (*lockFile, bool) {
	deadline := time.Now().Add(wait)
	for {
		if l, ok := tryLock(path, stale); ok {
			return l, true
		}
		if time.Now().After(deadline) {
			return nil, false
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (l *lockFile) unlock() error {
	if err := os.Remove(l.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func hashName(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:16])
}
