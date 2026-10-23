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
	"net/http"
	"strings"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"

	"github.com/a2aproject/a2a-cli/internal/agentdir"
	"github.com/a2aproject/a2a-cli/internal/agentsrv/approvals"
	"github.com/a2aproject/a2a-cli/internal/auth"
	"github.com/a2aproject/a2a-cli/internal/controlapi"
)

// The conformance suite runs every scenario a harness profile must pass
// through the whole server: auth, the ACP executor, the task store and the
// harness process.

func TestConformanceTextTurn(t *testing.T) {
	t.Parallel()
	ts := startServer(t, serverOpts{})
	c := ts.client(ts.pair("me", auth.RoleOwner))
	wantResult(t, send(t, c, userText("hello", "", "")), a2a.TaskStateCompleted, "Hello from fakeacp.")
}

func TestConformanceToolProgress(t *testing.T) {
	t.Parallel()
	ts := startServer(t, serverOpts{})
	c := ts.client(ts.pair("me", auth.RoleOwner))
	var sawTool bool
	var final a2a.TaskState
	for ev, err := range c.SendStreamingMessage(context.Background(), &a2a.SendMessageRequest{Message: userText("tool", "", "")}) {
		if err != nil {
			t.Fatalf("SendStreamingMessage() error = %v", err)
		}
		if su, ok := ev.(*a2a.TaskStatusUpdateEvent); ok {
			final = su.Status.State
			if su.Status.Message != nil {
				for _, p := range su.Status.Message.Parts {
					if m, ok := p.Data().(map[string]any); ok && m["toolCall"] != nil {
						sawTool = true
					}
				}
			}
		}
	}
	if !sawTool {
		t.Fatalf("SendStreamingMessage() had no toolCall progress update")
	}
	if final != a2a.TaskStateCompleted {
		t.Fatalf("final state = %s, want completed", final)
	}
}

func TestConformanceApprovals(t *testing.T) {
	t.Parallel()
	tests := []struct {
		reply string
		want  string
	}{
		{reply: "approve", want: "Pushed (allow)."},
		{reply: "always", want: "Pushed (always)."},
		{reply: "deny", want: "Push denied."},
	}
	ts := startServer(t, serverOpts{})
	c := ts.client(ts.pair("me", auth.RoleOwner))
	for _, tc := range tests {
		t.Run(tc.reply, func(t *testing.T) {
			task := send(t, c, userText("ask", "", ""))
			if task.Status.State != a2a.TaskStateInputRequired {
				t.Fatalf("task state = %s, want input-required", task.Status.State)
			}
			q := approvals.FromStatus(task.Status)
			if q == nil || q.Type != approvals.TypePermission || len(q.Options) != 3 {
				t.Fatalf("question = %+v, want a permission question with 3 options", q)
			}
			wantResult(t, send(t, c, userText(tc.reply, string(task.ID), task.ContextID)), a2a.TaskStateCompleted, tc.want)
		})
	}
}

func TestConformanceCancelMidTool(t *testing.T) {
	t.Parallel()
	ts := startServer(t, serverOpts{})
	c := ts.client(ts.pair("me", auth.RoleOwner))
	task := sendAsync(t, c, userText("slow", "", ""))
	waitState(t, c, task.ID, a2a.TaskStateWorking)
	if _, err := c.CancelTask(context.Background(), &a2a.CancelTaskRequest{ID: task.ID}); err != nil {
		t.Fatalf("CancelTask() error = %v", err)
	}
	waitState(t, c, task.ID, a2a.TaskStateCanceled)
	wantResult(t, send(t, c, userText("hello", "", task.ContextID)), a2a.TaskStateCompleted, "Hello from fakeacp.")
}

func TestConformanceFollowUpInContext(t *testing.T) {
	t.Parallel()
	ts := startServer(t, serverOpts{})
	c := ts.client(ts.pair("me", auth.RoleOwner))
	first := send(t, c, userText("remember blue", "", ""))
	wantResult(t, send(t, c, userText("recall", "", first.ContextID)), a2a.TaskStateCompleted, "You said: blue")
	wantResult(t, send(t, c, userText("recall", "", "")), a2a.TaskStateCompleted, "You said: ")
}

func TestConformanceRestartAndResume(t *testing.T) {
	t.Parallel()
	ts := startServer(t, serverOpts{})
	token := ts.pair("me", auth.RoleOwner)
	first := send(t, ts.client(token), userText("remember green", "", ""))
	root := ts.dir.Root
	ts.stop()

	ts2 := startServer(t, serverOpts{dir: root})
	wantResult(t, send(t, ts2.client(token), userText("recall", "", first.ContextID)), a2a.TaskStateCompleted, "You said: green")
}

func TestSecondServerInSameDirFailsWithoutSideEffects(t *testing.T) {
	t.Parallel()
	ts := startServer(t, serverOpts{})
	c := ts.client(ts.pair("me", auth.RoleOwner))
	task := send(t, c, userText("ask", "", ""))
	if task.Status.State != a2a.TaskStateInputRequired {
		t.Fatalf("task state = %s, want input-required", task.Status.State)
	}

	cfg, err := ts.dir.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	err = Run(context.Background(), Options{Dir: ts.dir, Config: cfg, Secrets: agentdir.Secrets{}})
	if err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("Run() error = %v, want an already running error", err)
	}

	got := waitState(t, c, task.ID, a2a.TaskStateInputRequired)
	wantResult(t, send(t, c, userText("approve", string(got.ID), got.ContextID)), a2a.TaskStateCompleted, "Pushed (allow).")
}

func TestConformanceCrashRestartsAgent(t *testing.T) {
	t.Parallel()
	ts := startServer(t, serverOpts{})
	c := ts.client(ts.pair("me", auth.RoleOwner))
	task := send(t, c, userText("crash", "", ""))
	if task.Status.State != a2a.TaskStateFailed {
		t.Fatalf("task state = %s, want failed", task.Status.State)
	}
	wantResult(t, send(t, c, userText("hello", "", "")), a2a.TaskStateCompleted, "Hello from fakeacp.")
}

func TestConformanceQueuedConcurrentTask(t *testing.T) {
	t.Parallel()
	ts := startServer(t, serverOpts{})
	c := ts.client(ts.pair("me", auth.RoleOwner))
	slow := sendAsync(t, c, userText("slow", "", ""))
	waitState(t, c, slow.ID, a2a.TaskStateWorking)
	queued := sendAsync(t, c, userText("hello", "", slow.ContextID))
	if got := waitState(t, c, queued.ID, a2a.TaskStateSubmitted); got.Status.State != a2a.TaskStateSubmitted {
		t.Fatalf("queued task state = %s, want submitted", got.Status.State)
	}
	if _, err := c.CancelTask(context.Background(), &a2a.CancelTaskRequest{ID: slow.ID}); err != nil {
		t.Fatalf("CancelTask() error = %v", err)
	}
	wantResult(t, waitState(t, c, queued.ID, a2a.TaskStateCompleted), a2a.TaskStateCompleted, "Hello from fakeacp.")
}

func TestUnauthenticatedCallerIsRejected(t *testing.T) {
	t.Parallel()
	ts := startServer(t, serverOpts{})
	resp, err := http.Post(ts.localURL()+"/", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("POST / status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
	card, err := http.Get(ts.localURL() + "/.well-known/agent-card.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := card.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if card.StatusCode != http.StatusOK {
		t.Fatalf("GET card status = %d, want %d", card.StatusCode, http.StatusOK)
	}
}

func TestMemberCannotApproveAndOwnerIsAsked(t *testing.T) {
	t.Parallel()
	ch := newFakeChannel("fake")
	ts := startServer(t, serverOpts{channel: ch})
	ctx := context.Background()
	code, err := controlapi.NewClient(ts.dir).Pair(ctx, controlapi.PairCodeRequest{Name: "owner", Role: auth.RoleOwner})
	if err != nil {
		t.Fatal(err)
	}
	ch.write("tg:1", "dm1", "/start "+code.Code)
	ch.waitFor(t, "dm1", hasText("Linked as owner"))

	member := ts.client(ts.pair("grok", auth.RoleMember))
	task := send(t, member, userText("ask", "", ""))
	if task.Status.State != a2a.TaskStateInputRequired {
		t.Fatalf("task state = %s, want input-required", task.Status.State)
	}
	_, err = member.SendMessage(ctx, &a2a.SendMessageRequest{Message: userText("approve", string(task.ID), task.ContextID)})
	if !errors.Is(err, a2a.ErrUnauthorized) {
		t.Fatalf("SendMessage() by member error = %v, want %v", err, a2a.ErrUnauthorized)
	}

	ref, q := ch.waitFor(t, "dm1", hasQuestion)
	ch.click("tg:1", "dm1", ref, q.Question, 0)
	ch.waitFor(t, "dm1", hasText("✓ Allow"))
	wantResult(t, waitState(t, member, task.ID, a2a.TaskStateCompleted), a2a.TaskStateCompleted, "Pushed (allow).")
}
