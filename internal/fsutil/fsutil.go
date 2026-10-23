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

// Package fsutil writes files that several processes share: atomically,
// with exact permissions, under an advisory lock.
package fsutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// WriteAtomic replaces the file at path with data, readable as perm. Readers
// see the old or the new content, never a partial write.
func WriteAtomic(path string, data []byte, perm os.FileMode) (err error) {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, removeIfExists(f.Name()))
		}
	}()
	if _, err := f.Write(data); err != nil {
		return errors.Join(err, f.Close())
	}
	if err := f.Chmod(perm); err != nil {
		return errors.Join(err, f.Close())
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// WithLock runs fn while holding an exclusive advisory lock on path+".lock",
// so that read-modify-write cycles of several processes don't lose updates.
func WithLock(path string, fn func() error) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	if err := lock(f); err != nil {
		return fmt.Errorf("locking %s: %w", path, err)
	}
	defer func() { err = errors.Join(err, unlock(f)) }()
	return fn()
}

// ErrLocked is returned by TryLock when another holder has the lock.
var ErrLocked = errors.New("locked by another process")

// TryLock takes an exclusive advisory lock on path without waiting, and
// returns the function that releases it.
func TryLock(path string) (release func() error, err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := tryLock(f); err != nil {
		return nil, errors.Join(err, f.Close())
	}
	return func() error { return errors.Join(unlock(f), f.Close()) }, nil
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
