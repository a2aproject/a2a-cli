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
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestNewGrantKey(t *testing.T) {
	t.Parallel()
	got := NewGrantKey("https://login.example.com/", "c", "https://Agent.example.com/", []string{"b", "a", "b"})
	want := GrantKey{Issuer: "https://login.example.com", ClientID: "c", Resource: "https://agent.example.com", Scopes: []string{"a", "b"}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("NewGrantKey() wrong result (-want +got) diff = %s", diff)
	}
	if got.ID() != NewGrantKey("https://login.example.com", "c", "https://agent.example.com", []string{"a", "b"}).ID() {
		t.Fatalf("GrantKey.ID() differs for equal keys")
	}
}

func TestFileStore(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "oauth")
	s := &FileStore{Dir: dir}
	k1 := NewGrantKey("https://login.example.com", "c", "https://a.example.com", []string{"x"})
	k2 := NewGrantKey("https://login.example.com", "c", "https://b.example.com", nil)
	expires := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)

	if got, err := s.List(); err != nil || got != nil {
		t.Fatalf("FileStore.List() on a missing file = %v, %v, want nil, nil", got, err)
	}
	for _, g := range []Grant{
		{Key: k1, AccessToken: "a1", RefreshToken: "r1", ExpiresAt: expires},
		{Key: k2, AccessToken: "b1"},
		{Key: k1, AccessToken: "a2", RefreshToken: "r2", ExpiresAt: expires},
	} {
		if err := s.Put(g); err != nil {
			t.Fatalf("FileStore.Put() error = %v", err)
		}
	}
	got, err := s.Get(k1)
	if err != nil {
		t.Fatalf("FileStore.Get() error = %v", err)
	}
	if diff := cmp.Diff(&Grant{Key: k1, AccessToken: "a2", RefreshToken: "r2", ExpiresAt: expires}, got); diff != "" {
		t.Fatalf("FileStore.Get() wrong result (-want +got) diff = %s", diff)
	}
	if err := s.Delete(k1); err != nil {
		t.Fatalf("FileStore.Delete() error = %v", err)
	}
	all, err := s.List()
	if err != nil {
		t.Fatalf("FileStore.List() error = %v", err)
	}
	if diff := cmp.Diff([]Grant{{Key: k2, AccessToken: "b1"}}, all); diff != "" {
		t.Fatalf("FileStore.List() wrong result (-want +got) diff = %s", diff)
	}

	if runtime.GOOS == "windows" {
		return
	}
	for path, want := range map[string]os.FileMode{dir: 0o700, filepath.Join(dir, grantsFileName): 0o600} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("os.Stat(%q) error = %v", path, err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Fatalf("mode of %s = %v, want %v", path, got, want)
		}
	}
}

func TestFileStoreLock(t *testing.T) {
	t.Parallel()
	s := &FileStore{Dir: t.TempDir()}
	key := NewGrantKey("https://login.example.com", "c", "https://a.example.com", nil)
	if err := s.Put(Grant{Key: key, AccessToken: "0"}); err != nil {
		t.Fatal(err)
	}

	const workers = 8
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			unlock, err := s.Lock(ctx)
			if err != nil {
				t.Errorf("FileStore.Lock() error = %v", err)
				return
			}
			defer unlock()
			g, err := s.Get(key)
			if err != nil {
				t.Errorf("FileStore.Get() error = %v", err)
				return
			}
			g.AccessToken += "+"
			if err := s.Put(*g); err != nil {
				t.Errorf("FileStore.Put() error = %v", err)
			}
		})
	}
	wg.Wait()
	g, err := s.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	if want := "0++++++++"; g.AccessToken != want {
		t.Fatalf("access token after %d locked updates = %q, want %q", workers, g.AccessToken, want)
	}
}

func TestFileStoreLockTimesOut(t *testing.T) {
	t.Parallel()
	s := &FileStore{Dir: t.TempDir()}
	unlock, err := s.Lock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := s.Lock(ctx); err == nil {
		t.Fatalf("FileStore.Lock() while held error = nil, want timeout")
	}
}

func TestFileStoreLockIsNotStolen(t *testing.T) {
	t.Parallel()
	s := &FileStore{Dir: t.TempDir()}
	unlock, err := s.Lock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// A lock held through a slow refresh stays held, however old the file is.
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(s.Dir, lockFileName), old, old); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := s.Lock(ctx); err == nil {
		t.Fatal("FileStore.Lock() took a held lock")
	}
	unlock()

	// The file left behind after release does not block the next locker.
	unlock, err = s.Lock(context.Background())
	if err != nil {
		t.Fatalf("FileStore.Lock() after release error = %v", err)
	}
	unlock()
}
