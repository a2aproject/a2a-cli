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
	"testing"

	"github.com/a2aproject/a2a-cli/internal/auth"
	"github.com/a2aproject/a2a-cli/internal/channel"
)

// linkOwner links identity as the owner in conversation conv.
func linkOwner(t *testing.T, ts *testServer, identity, conv string) {
	t.Helper()
	code, err := NewClient(ts.dir).Pair(context.Background(), PairCodeRequest{Name: "owner", Role: auth.RoleOwner})
	if err != nil {
		t.Fatal(err)
	}
	ts.ch.write(identity, conv, "link "+code.Code)
	ts.ch.waitFor(t, conv, hasText("Linked as owner"))
}

func TestRouterUnknownSenderGetsPairingHint(t *testing.T) {
	t.Parallel()
	ts := startServer(t, serverOpts{channel: newFakeChannel("fake")})
	ts.ch.write("tg:9", "dm9", "hello")
	ts.ch.waitFor(t, "dm9", hasText("I don't know you yet"))
	ts.ch.write("tg:9", "dm9", "link AAAA-BBBB")
	ts.ch.waitFor(t, "dm9", hasText("invalid or has expired"))
}

func TestRouterTurnAndApprovalInChat(t *testing.T) {
	t.Parallel()
	ts := startServer(t, serverOpts{channel: newFakeChannel("fake")})
	linkOwner(t, ts, "tg:1", "dm1")

	ts.ch.write("tg:1", "dm1", "hello")
	ts.ch.waitFor(t, "dm1", hasText("Hello from fakeacp."))

	ts.ch.write("tg:1", "dm1", "ask")
	ref, q := ts.ch.waitFor(t, "dm1", hasQuestion)
	if len(q.Question.Options) != 3 {
		t.Fatalf("question options = %v, want 3", q.Question.Options)
	}
	ts.ch.click("tg:1", "dm1", ref, q.Question, 1)
	ts.ch.waitFor(t, "dm1", hasText("Pushed (always)."))
}

func TestRouterTypedAnswer(t *testing.T) {
	t.Parallel()
	ch := newFakeChannel("fake")
	ch.buttons = false
	ts := startServer(t, serverOpts{channel: ch})
	linkOwner(t, ts, "mail:me@example.com", "thread1")

	ts.ch.write("mail:me@example.com", "thread1", "ask")
	ts.ch.waitFor(t, "thread1", hasText("Reply approve, always, deny"))
	ts.ch.write("mail:me@example.com", "thread1", "deny")
	ts.ch.waitFor(t, "thread1", hasText("Push denied."))
}

func TestRouterFollowUpAndNewConversation(t *testing.T) {
	t.Parallel()
	ts := startServer(t, serverOpts{channel: newFakeChannel("fake")})
	linkOwner(t, ts, "tg:1", "dm1")

	ts.ch.write("tg:1", "dm1", "remember red")
	ts.ch.waitFor(t, "dm1", hasText("OK."))
	ts.ch.write("tg:1", "dm1", "recall")
	ts.ch.waitFor(t, "dm1", hasText("You said: red"))
	ts.ch.write("tg:1", "dm1", "/new")
	ts.ch.waitFor(t, "dm1", hasText("Started a new conversation."))
	ts.ch.write("tg:1", "dm1", "recall")
	ts.ch.waitFor(t, "dm1", func(m channel.Outbound) bool { return m.Text == "You said: " })
}

func TestRouterStop(t *testing.T) {
	t.Parallel()
	ts := startServer(t, serverOpts{channel: newFakeChannel("fake")})
	linkOwner(t, ts, "tg:1", "dm1")
	go ts.ch.write("tg:1", "dm1", "slow")
	ts.ch.waitFor(t, "dm1", hasText("..."))
	ts.ch.write("tg:1", "dm1", "/stop")
	ts.ch.waitFor(t, "dm1", hasText("Stopped."))
}
