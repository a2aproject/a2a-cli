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
	"fmt"

	"github.com/zalando/go-keyring"

	"github.com/a2aproject/a2a-go/v2/a2aclient/oauth"
)

// keyringService is the service name grants are stored under in the OS keychain.
const keyringService = "a2a-cli-oauth"

// Keyring stores secrets in the OS keychain.
type Keyring interface {
	// Get returns the secret for user, or ErrSecretNotFound.
	Get(service, user string) (string, error)
	// Set stores the secret for user.
	Set(service, user, secret string) error
	// Delete removes the secret for user.
	Delete(service, user string) error
}

// ErrSecretNotFound is returned by a [Keyring] that has no secret for a user.
var ErrSecretNotFound = keyring.ErrNotFound

// SystemKeyring is the OS keychain: macOS Keychain, Windows Credential Manager,
// or the Secret Service on Linux.
type SystemKeyring struct{}

// Get implements [Keyring].
func (SystemKeyring) Get(service, user string) (string, error) { return keyring.Get(service, user) }

// Set implements [Keyring].
func (SystemKeyring) Set(service, user, secret string) error {
	return keyring.Set(service, user, secret)
}

// Delete implements [Keyring].
func (SystemKeyring) Delete(service, user string) error { return keyring.Delete(service, user) }

// KeyringIndexName is the file name of the [KeyringStore] index, kept apart from
// grants.json so a [FileStore] never mistakes a keychain grant for one without
// tokens.
const KeyringIndexName = "keychain-grants.json"

// KeyringStore keeps tokens in the OS keychain and everything else in a
// [FileStore] index. Each token is a separate keychain entry, which keeps
// entries within the size limits of the macOS Keychain and the Windows
// Credential Manager.
//
// It follows the [oauth.Store] contract: List returns the index entries
// without their tokens and reads no secrets, with Refreshable telling whether a
// grant has a refresh token; Get reads the tokens of one grant and returns an
// error wrapping [oauth.ErrGrantNotFound] when there is none. Results never
// share memory with the store.
type KeyringStore struct {
	Index   *FileStore
	Keyring Keyring
}

var (
	_ oauth.Store  = (*KeyringStore)(nil)
	_ oauth.Locker = (*KeyringStore)(nil)
)

// keyringUser is the keychain user of a grant's access or refresh token.
func keyringUser(id, token string) string {
	return id + "/" + token
}

// List implements [oauth.Store]. It reads only the index, so the grants have
// no tokens.
func (s *KeyringStore) List(ctx context.Context) ([]oauth.Grant, error) {
	return s.Index.List(ctx)
}

// Get implements [oauth.Store].
func (s *KeyringStore) Get(ctx context.Context, id string) (*oauth.Grant, error) {
	g, err := s.Index.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.fill(g); err != nil {
		return nil, err
	}
	g.Refreshable = g.RefreshToken != ""
	return g, nil
}

// fill reads the grant's tokens from the keychain. A missing entry leaves the
// token empty, and a grant with no tokens left needs a new sign-in.
func (s *KeyringStore) fill(g *oauth.Grant) error {
	for _, t := range []struct {
		name string
		dst  *string
	}{{"access", &g.AccessToken}, {"refresh", &g.RefreshToken}} {
		v, err := s.Keyring.Get(keyringService, keyringUser(g.ID(), t.name))
		if err != nil && !errors.Is(err, ErrSecretNotFound) {
			return fmt.Errorf("reading the keychain: %w", err)
		}
		*t.dst = v
	}
	return nil
}

// Put implements [oauth.Store]. It writes the refresh token first: after a
// refresh the server may have rotated it, and a failure to write the access
// token then still leaves the new refresh token stored, without the previous
// access token, so the grant is refreshed when next used. A grant the index
// doesn't hold yet leaves no keychain entries behind when it can't be stored,
// since nothing would find them to remove them.
func (s *KeyringStore) Put(ctx context.Context, g oauth.Grant) error {
	id := g.ID()
	_, err := s.Index.Get(ctx, id)
	indexed := err == nil
	if err != nil && !errors.Is(err, oauth.ErrGrantNotFound) {
		return err
	}
	var written []string
	undo := func() {
		if indexed {
			return
		}
		for _, user := range written {
			_ = s.Keyring.Delete(keyringService, user)
		}
	}
	for _, t := range []struct{ name, value string }{{"refresh", g.RefreshToken}, {"access", g.AccessToken}} {
		user := keyringUser(id, t.name)
		if t.value == "" {
			if err := s.Keyring.Delete(keyringService, user); err != nil && !errors.Is(err, ErrSecretNotFound) {
				undo()
				return fmt.Errorf("deleting from the keychain: %w", err)
			}
			continue
		}
		if err := s.Keyring.Set(keyringService, user, t.value); err != nil {
			if t.name == "access" && indexed {
				// The previous access token doesn't go with the refresh token
				// just written.
				_ = s.Keyring.Delete(keyringService, user)
			}
			undo()
			if errors.Is(err, keyring.ErrSetDataTooBig) {
				return fmt.Errorf("the %s token from %s is too large for the OS keychain; use --oauth-store file: %w", t.name, g.Issuer, err)
			}
			return fmt.Errorf("writing to the keychain (use --oauth-store file where none is available): %w", err)
		}
		written = append(written, user)
	}
	g.Refreshable = g.RefreshToken != ""
	g.AccessToken, g.RefreshToken = "", ""
	if err := s.Index.Put(ctx, g); err != nil {
		undo()
		return err
	}
	return nil
}

// Delete implements [oauth.Store].
func (s *KeyringStore) Delete(ctx context.Context, id string) error {
	for _, name := range []string{"access", "refresh"} {
		if err := s.Keyring.Delete(keyringService, keyringUser(id, name)); err != nil && !errors.Is(err, ErrSecretNotFound) {
			return fmt.Errorf("deleting from the keychain: %w", err)
		}
	}
	return s.Index.Delete(ctx, id)
}

// Lock implements [oauth.Locker] with the index's lock.
func (s *KeyringStore) Lock(ctx context.Context) (func(), error) {
	return s.Index.Lock(ctx)
}
