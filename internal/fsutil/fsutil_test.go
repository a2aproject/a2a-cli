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

//go:build unix

package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
)

func TestWriteAtomicSetsMode(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "secrets")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := WriteAtomic(path, []byte("new"), 0o600); err != nil {
		t.Fatalf("WriteAtomic() error = %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Fatalf("WriteAtomic() mode = %v, want %v", got, os.FileMode(0o600))
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("WriteAtomic() left %d files, want 1", len(entries))
	}
}

func TestMkdirPrivate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		setup   func(t *testing.T, dir string)
		wantErr bool
	}{
		{name: "new", setup: func(*testing.T, string) {}},
		{name: "open to others", setup: func(t *testing.T, dir string) {
			if err := os.Mkdir(dir, 0o755); err != nil {
				t.Fatalf("Mkdir() error = %v", err)
			}
			if err := os.Chmod(dir, 0o755); err != nil {
				t.Fatalf("Chmod() error = %v", err)
			}
		}, wantErr: true},
		{name: "symlink", setup: func(t *testing.T, dir string) {
			if err := os.Symlink(t.TempDir(), dir); err != nil {
				t.Fatalf("Symlink() error = %v", err)
			}
		}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(t.TempDir(), "run")
			tc.setup(t, dir)
			if err := MkdirPrivate(dir); (err != nil) != tc.wantErr {
				t.Fatalf("MkdirPrivate() error = %v, want error %v", err, tc.wantErr)
			}
		})
	}
}

func TestWithLockSerializesUpdates(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "counter")
	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- WithLock(path, func() error {
				data, err := os.ReadFile(path)
				if err != nil && !errors.Is(err, os.ErrNotExist) {
					return err
				}
				v := 0
				if len(data) > 0 {
					if v, err = strconv.Atoi(string(data)); err != nil {
						return err
					}
				}
				return WriteAtomic(path, []byte(strconv.Itoa(v+1)), 0o600)
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("WithLock() error = %v", err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got := string(data); got != strconv.Itoa(n) {
		t.Fatalf("counter = %s, want %d", got, n)
	}
}

func TestTryLock(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "server.lock")
	release, err := TryLock(path)
	if err != nil {
		t.Fatalf("TryLock() error = %v", err)
	}
	if _, err := TryLock(path); !errors.Is(err, ErrLocked) {
		t.Fatalf("TryLock() error = %v, want %v", err, ErrLocked)
	}
	if err := release(); err != nil {
		t.Fatalf("release() error = %v", err)
	}
	release, err = TryLock(path)
	if err != nil {
		t.Fatalf("TryLock() after release error = %v", err)
	}
	if err := release(); err != nil {
		t.Fatalf("release() error = %v", err)
	}
}
