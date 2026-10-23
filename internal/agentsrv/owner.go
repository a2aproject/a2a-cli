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

package agentsrv

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/a2aproject/a2a-cli/internal/auth"
	"github.com/a2aproject/a2a-cli/internal/credstore"
	"github.com/a2aproject/a2a-cli/internal/store"
)

// localOwnerName names the owner token the server keeps in the credentials
// of the user running it.
const localOwnerName = "local"

// saveLocalOwnerToken makes sure the user running the server has an owner
// token for its local URL, so `a2a send` from this machine works without
// pairing. Loopback connections aren't trusted instead, because tunnels
// such as ngrok reach the server from loopback too.
func (s *Server) saveLocalOwnerToken(ctx context.Context) error {
	local, _, _ := s.URLs()
	saved, ok, err := s.credentials.Lookup(local)
	if err != nil {
		return err
	}
	if ok && saved.Kind != credstore.KindNode {
		t, err := s.store.LookupToken(ctx, saved.Token)
		if err == nil && slices.Contains(t.Roles, auth.RoleOwner) {
			return nil
		}
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}
	}
	if err := s.store.DeleteTokens(ctx, localOwnerName); err != nil {
		return err
	}
	token, p, err := auth.IssueToken(ctx, s.store, store.Code{Name: localOwnerName, Role: auth.RoleOwner}, local)
	if err != nil {
		return err
	}
	return s.credentials.Put(credstore.Credential{URL: local, Token: token, Agent: s.config().Name, Principal: p.ID, Created: time.Now().UTC()})
}
