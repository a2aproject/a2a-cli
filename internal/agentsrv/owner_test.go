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
	"path/filepath"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"

	"github.com/a2aproject/a2a-cli/internal/auth"
	"github.com/a2aproject/a2a-cli/internal/controlapi"
	"github.com/a2aproject/a2a-cli/internal/credstore"
)

func lookupOwnerToken(t *testing.T, creds *credstore.Store, url string) string {
	t.Helper()
	c, ok, err := creds.Lookup(url)
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if !ok {
		t.Fatalf("Lookup(%q) found no token, want the owner token of this machine", url)
	}
	if c.Principal != auth.OwnerID {
		t.Fatalf("Lookup(%q) principal = %q, want %q", url, c.Principal, auth.OwnerID)
	}
	return c.Token
}

func TestOwnerTokenSavedForThisMachine(t *testing.T) {
	t.Parallel()
	creds := &credstore.Store{Path: filepath.Join(t.TempDir(), "credentials.json")}
	ts := startServer(t, serverOpts{name: "assistant", credentials: creds})
	token := lookupOwnerToken(t, creds, ts.localURL())

	task := send(t, ts.client(token), userText("hello", "", ""))
	wantResult(t, task, a2a.TaskStateCompleted, "Hello from fakeacp.")

	dir := ts.dir.Root
	ts.stop()
	ts = startServer(t, serverOpts{dir: dir, credentials: creds})
	if got := lookupOwnerToken(t, creds, ts.localURL()); got != token {
		t.Fatalf("owner token after a restart = %q, want the saved %q", got, token)
	}

	if _, err := controlapi.NewClient(ts.dir).Revoke(context.Background(), localOwnerName); err != nil {
		t.Fatalf("Revoke(%q) error = %v", localOwnerName, err)
	}
	ts.stop()
	ts = startServer(t, serverOpts{dir: dir, credentials: creds})
	renewed := lookupOwnerToken(t, creds, ts.localURL())
	if renewed == token {
		t.Fatalf("owner token after a revoke = the revoked token, want a new one")
	}
	task = send(t, ts.client(renewed), userText("hello", "", ""))
	wantResult(t, task, a2a.TaskStateCompleted, "Hello from fakeacp.")
}

func TestPairRejectsReservedName(t *testing.T) {
	t.Parallel()
	ts := startServer(t, serverOpts{name: "assistant"})
	if _, err := controlapi.NewClient(ts.dir).Pair(context.Background(), controlapi.PairCodeRequest{Name: localOwnerName}); err == nil {
		t.Fatalf("Pair(%q) error = nil, want error", localOwnerName)
	}
}
