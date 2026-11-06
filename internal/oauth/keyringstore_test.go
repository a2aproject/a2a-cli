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
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zalando/go-keyring"
)

type fakeKeyring struct {
	mu      sync.Mutex
	secrets map[string]string
	err     error
}

func (k *fakeKeyring) Get(service, user string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	s, ok := k.secrets[service+"/"+user]
	if !ok {
		return "", ErrSecretNotFound
	}
	return s, nil
}

func (k *fakeKeyring) Set(service, user, secret string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.err != nil {
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
	key := NewGrantKey("https://login.example.com", "c", "https://a.example.com", nil)
	g := Grant{Key: key, AccessToken: "access-secret", RefreshToken: "refresh-secret", Endpoints: Endpoints{Issuer: "https://login.example.com", Token: "https://login.example.com/token"}}

	if err := s.Put(g); err != nil {
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
	got, err := s.Get(key)
	if err != nil {
		t.Fatalf("KeyringStore.Get() error = %v", err)
	}
	if diff := cmp.Diff(&g, got); diff != "" {
		t.Fatalf("KeyringStore.Get() wrong result (-want +got) diff = %s", diff)
	}
	all, err := s.List()
	if err != nil || len(all) != 1 || all[0].RefreshToken != "refresh-secret" {
		t.Fatalf("KeyringStore.List() = %v, %v, want the grant with its tokens", all, err)
	}
	if err := s.Delete(key); err != nil {
		t.Fatalf("KeyringStore.Delete() error = %v", err)
	}
	if got, err := s.Get(key); err != nil || got != nil {
		t.Fatalf("KeyringStore.Get() after Delete = %v, %v, want nil, nil", got, err)
	}
	if len(kr.secrets) != 0 {
		t.Fatalf("keyring entries after Delete = %v, want none", kr.secrets)
	}

	kr.err = errors.New("no secret service")
	if err := s.Put(g); err == nil || !strings.Contains(err.Error(), "--oauth-store file") {
		t.Fatalf("KeyringStore.Put() without a keychain error = %v, want a hint to use the file store", err)
	}
	kr.err = keyring.ErrSetDataTooBig
	if err := s.Put(g); err == nil || !strings.Contains(err.Error(), "too large for the OS keychain") {
		t.Fatalf("KeyringStore.Put() with an oversized token error = %v, want it named", err)
	}
}

func TestKeyringStoreMissingSecret(t *testing.T) {
	t.Parallel()
	kr := &fakeKeyring{}
	s := &KeyringStore{Index: &FileStore{Dir: t.TempDir()}, Keyring: kr}
	key := NewGrantKey("https://login.example.com", "c", "https://a.example.com", nil)
	if err := s.Put(Grant{Key: key, AccessToken: "a", RefreshToken: "r"}); err != nil {
		t.Fatal(err)
	}
	kr.secrets = nil
	g, err := s.Get(key)
	if err != nil || g == nil || g.AccessToken != "" || g.RefreshToken != "" {
		t.Fatalf("KeyringStore.Get() with the keychain entries gone = %+v, %v, want the grant without tokens", g, err)
	}
}
