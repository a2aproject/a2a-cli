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
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/a2aproject/a2a-go/v2/a2aclient/oauth"
)

func TestFileStore(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "oauth")
	s := &FileStore{Dir: dir}
	k1 := oauth.Grant{Issuer: "https://login.example.com", ClientID: "c", Resource: "https://a.example.com", Scopes: []string{"x"}}
	k2 := oauth.Grant{Issuer: "https://login.example.com", ClientID: "c", Resource: "https://b.example.com"}
	expires := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)

	if got, err := s.List(t.Context()); err != nil || got != nil {
		t.Fatalf("FileStore.List() on a missing file = %v, %v, want nil, nil", got, err)
	}
	for _, g := range []oauth.Grant{
		withTokens(k1, "a1", "r1", expires),
		withTokens(k2, "b1", "", time.Time{}),
		withTokens(k1, "a2", "r2", expires),
	} {
		if err := s.Put(t.Context(), g); err != nil {
			t.Fatalf("FileStore.Put() error = %v", err)
		}
	}
	got, err := s.Get(t.Context(), k1.ID())
	if err != nil {
		t.Fatalf("FileStore.Get() error = %v", err)
	}
	want := withTokens(k1, "a2", "r2", expires)
	want.Refreshable = true
	if diff := cmp.Diff(&want, got); diff != "" {
		t.Fatalf("FileStore.Get() wrong result (-want +got) diff = %s", diff)
	}
	if err := s.Delete(t.Context(), k1.ID()); err != nil {
		t.Fatalf("FileStore.Delete() error = %v", err)
	}
	all, err := s.List(t.Context())
	if err != nil {
		t.Fatalf("FileStore.List() error = %v", err)
	}
	if diff := cmp.Diff([]oauth.Grant{withTokens(k2, "b1", "", time.Time{})}, all); diff != "" {
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

func TestFileStoreContract(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := &FileStore{Dir: dir}
	key := oauth.Grant{Issuer: "https://login.example.com", ClientID: "c", Resource: "https://a.example.com", Scopes: []string{"x"}}
	other := oauth.Grant{Issuer: "https://login.example.com", ClientID: "c", Resource: "https://b.example.com", AccessToken: "b"}

	if g, err := s.Get(t.Context(), key.ID()); !errors.Is(err, oauth.ErrGrantNotFound) {
		t.Fatalf("FileStore.Get() on a missing file = %v, %v, want oauth.ErrGrantNotFound", g, err)
	}
	if err := s.Put(t.Context(), other); err != nil {
		t.Fatal(err)
	}
	if g, err := s.Get(t.Context(), key.ID()); !errors.Is(err, oauth.ErrGrantNotFound) {
		t.Fatalf("FileStore.Get() of a missing key = %v, %v, want oauth.ErrGrantNotFound", g, err)
	}

	// Put keeps a copy: later changes to the caller's grant are not stored.
	g := oauth.Grant{Issuer: key.Issuer, ClientID: key.ClientID, Resource: key.Resource, Scopes: []string{"x"}, GrantedScopes: []string{"x"}, AccessToken: "a"}
	if err := s.Put(t.Context(), g); err != nil {
		t.Fatal(err)
	}
	g.Scopes[0], g.GrantedScopes[0] = "changed", "changed"
	got, err := s.Get(t.Context(), key.ID())
	if err != nil {
		t.Fatal(err)
	}
	if got.GrantedScopes[0] != "x" {
		t.Fatalf("FileStore.Get() GrantedScopes = %v, want the scopes as stored", got.GrantedScopes)
	}
	// Get returns a copy: changing it doesn't change the next result.
	got.GrantedScopes[0] = "changed"
	if again, err := s.Get(t.Context(), key.ID()); err != nil || again.GrantedScopes[0] != "x" {
		t.Fatalf("FileStore.Get() after changing an earlier result = %v, %v, want the stored grant", again, err)
	}
}

func TestFileStoreRefreshableFromOlderFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Files written before Grant.Refreshable existed only have the refresh token.
	data := `{"version":1,"grants":[{"issuer":"https://login.example.com","client_id":"c","resource":"https://a.example.com","access_token":"a","refresh_token":"r","endpoints":{"issuer":"https://login.example.com"}}]}`
	if err := os.WriteFile(filepath.Join(dir, grantsFileName), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &FileStore{Dir: dir}
	all, err := s.List(t.Context())
	if err != nil || len(all) != 1 || !all[0].Refreshable {
		t.Fatalf("FileStore.List() = %+v, %v, want the grant marked refreshable", all, err)
	}
}

func TestFileStoreLock(t *testing.T) {
	t.Parallel()
	s := &FileStore{Dir: t.TempDir()}
	key := oauth.Grant{Issuer: "https://login.example.com", ClientID: "c", Resource: "https://a.example.com"}
	if err := s.Put(t.Context(), withTokens(key, "0", "", time.Time{})); err != nil {
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
			g, err := s.Get(t.Context(), key.ID())
			if err != nil {
				t.Errorf("FileStore.Get() error = %v", err)
				return
			}
			g.AccessToken += "+"
			if err := s.Put(t.Context(), *g); err != nil {
				t.Errorf("FileStore.Put() error = %v", err)
			}
		})
	}
	wg.Wait()
	g, err := s.Get(t.Context(), key.ID())
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

// withTokens returns g with the given tokens and expiry.
func withTokens(g oauth.Grant, access, refresh string, expires time.Time) oauth.Grant {
	g.AccessToken, g.RefreshToken, g.ExpiresAt = access, refresh, expires
	return g
}
