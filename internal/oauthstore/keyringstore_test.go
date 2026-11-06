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
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zalando/go-keyring"

	"github.com/a2aproject/a2a-go/v2/a2aclient/oauth"
)

type fakeKeyring struct {
	mu      sync.Mutex
	secrets map[string]string
	err     error
	// failSuffix makes Set fail with err only for users ending in it.
	failSuffix string
	reads      int
}

func (k *fakeKeyring) Get(service, user string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.reads++
	s, ok := k.secrets[service+"/"+user]
	if !ok {
		return "", ErrSecretNotFound
	}
	return s, nil
}

func (k *fakeKeyring) Set(service, user, secret string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.err != nil && strings.HasSuffix(user, k.failSuffix) {
		return k.err
	}
	if k.secrets == nil {
		k.secrets = map[string]string{}
	}
	k.secrets[service+"/"+user] = secret
	return nil
}

func (k *fakeKeyring) Delete(service, user string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if _, ok := k.secrets[service+"/"+user]; !ok {
		return ErrSecretNotFound
	}
	delete(k.secrets, service+"/"+user)
	return nil
}

func TestKeyringStore(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	kr := &fakeKeyring{}
	s := &KeyringStore{Index: &FileStore{Dir: dir}, Keyring: kr}
	key := oauth.NewGrantKey("https://login.example.com", "c", "https://a.example.com", nil)
	g := oauth.Grant{Key: key, AccessToken: "access-secret", RefreshToken: "refresh-secret", Endpoints: oauth.Endpoints{Issuer: "https://login.example.com", Token: "https://login.example.com/token"}}

	if err := s.Put(t.Context(), g); err != nil {
		t.Fatalf("KeyringStore.Put() error = %v", err)
	}
	index, err := os.ReadFile(filepath.Join(dir, grantsFileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(index), "secret") {
		t.Fatalf("the index file contains tokens: %s", index)
	}
	if len(kr.secrets) != 2 {
		t.Fatalf("keyring entries = %v, want one per token", kr.secrets)
	}
	want := g
	want.Refreshable = true
	got, err := s.Get(t.Context(), key)
	if err != nil {
		t.Fatalf("KeyringStore.Get() error = %v", err)
	}
	if diff := cmp.Diff(&want, got); diff != "" {
		t.Fatalf("KeyringStore.Get() wrong result (-want +got) diff = %s", diff)
	}

	// List reads only the index: the grant, without its tokens.
	kr.reads = 0
	all, err := s.List(t.Context())
	if err != nil {
		t.Fatalf("KeyringStore.List() error = %v", err)
	}
	listed := want
	listed.AccessToken, listed.RefreshToken = "", ""
	if diff := cmp.Diff([]oauth.Grant{listed}, all); diff != "" {
		t.Fatalf("KeyringStore.List() wrong result (-want +got) diff = %s", diff)
	}
	if kr.reads != 0 {
		t.Fatalf("KeyringStore.List() read %d keychain entries, want none", kr.reads)
	}

	if err := s.Delete(t.Context(), key); err != nil {
		t.Fatalf("KeyringStore.Delete() error = %v", err)
	}
	if got, err := s.Get(t.Context(), key); !errors.Is(err, oauth.ErrGrantNotFound) {
		t.Fatalf("KeyringStore.Get() after Delete = %v, %v, want oauth.ErrGrantNotFound", got, err)
	}
	if len(kr.secrets) != 0 {
		t.Fatalf("keyring entries after Delete = %v, want none", kr.secrets)
	}

	kr.err = errors.New("no secret service")
	if err := s.Put(t.Context(), g); err == nil || !strings.Contains(err.Error(), "--oauth-store file") {
		t.Fatalf("KeyringStore.Put() without a keychain error = %v, want a hint to use the file store", err)
	}
	kr.err = keyring.ErrSetDataTooBig
	if err := s.Put(t.Context(), g); err == nil || !strings.Contains(err.Error(), "too large for the OS keychain") {
		t.Fatalf("KeyringStore.Put() with an oversized token error = %v, want it named", err)
	}
}

func TestKeyringStoreMissingSecret(t *testing.T) {
	t.Parallel()
	kr := &fakeKeyring{}
	s := &KeyringStore{Index: &FileStore{Dir: t.TempDir()}, Keyring: kr}
	key := oauth.NewGrantKey("https://login.example.com", "c", "https://a.example.com", nil)
	if err := s.Put(t.Context(), oauth.Grant{Key: key, AccessToken: "a", RefreshToken: "r"}); err != nil {
		t.Fatal(err)
	}
	kr.secrets = nil
	g, err := s.Get(t.Context(), key)
	if err != nil || g == nil || g.AccessToken != "" || g.RefreshToken != "" || g.Refreshable {
		t.Fatalf("KeyringStore.Get() with the keychain entries gone = %+v, %v, want the grant without tokens", g, err)
	}
}

// TestKeyringStoreRotatedRefreshToken checks that a refresh token the server
// rotated is stored even when the access token can't be written.
func TestKeyringStoreRotatedRefreshToken(t *testing.T) {
	t.Parallel()
	kr := &fakeKeyring{}
	s := &KeyringStore{Index: &FileStore{Dir: t.TempDir()}, Keyring: kr}
	key := oauth.NewGrantKey("https://login.example.com", "c", "https://a.example.com", nil)
	if err := s.Put(t.Context(), oauth.Grant{Key: key, AccessToken: "a1", RefreshToken: "r1"}); err != nil {
		t.Fatal(err)
	}
	kr.err, kr.failSuffix = keyring.ErrSetDataTooBig, "/access"
	if err := s.Put(t.Context(), oauth.Grant{Key: key, AccessToken: "a2", RefreshToken: "r2"}); err == nil {
		t.Fatal("KeyringStore.Put() with an oversized access token error = nil, want one")
	}
	g, err := s.Get(t.Context(), key)
	if err != nil || g.RefreshToken != "r2" {
		t.Fatalf("KeyringStore.Get() after the failed Put = %+v, %v, want the rotated refresh token r2", g, err)
	}
}
