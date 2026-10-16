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

package relay

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
)

// hook is a fake node webhook.
type hook struct {
	srv   *httptest.Server
	wakes chan Envelope
}

func newHook(t *testing.T) *hook {
	h := &hook{wakes: make(chan Envelope, 32)}
	h.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Secret") != "s3cret" {
			http.Error(w, "bad secret", http.StatusUnauthorized)
			return
		}
		var env Envelope
		if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
			t.Errorf("decoding wake: %v", err)
		}
		h.wakes <- env
	}))
	t.Cleanup(h.srv.Close)
	return h
}

func (h *hook) next(t *testing.T, kind string) Envelope {
	t.Helper()
	select {
	case env := <-h.wakes:
		if env.Kind != kind {
			t.Fatalf("wake kind = %q, want %q (%+v)", env.Kind, kind, env)
		}
		return env
	case <-time.After(5 * time.Second):
		t.Fatalf("no %s wake", kind)
	}
	return Envelope{}
}

type fixture struct {
	relay *httptest.Server
	admin *Client
}

func newFixture(t *testing.T, cfg Config) *fixture {
	cfg.AdminToken = "admin"
	cfg.WebhookRetries = []time.Duration{0}
	cfg.StateDir = t.TempDir()
	cfg.PollInterval = 50 * time.Millisecond
	srv, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return &fixture{relay: ts, admin: &Client{BaseURL: ts.URL, Token: "admin"}}
}

// attach creates and pairs a node and returns the node's client.
func (f *fixture) attach(t *testing.T, h *hook, name string) (*Client, NodeInfo) {
	t.Helper()
	ctx := context.Background()
	res, err := f.admin.CreateNode(ctx, CreateNodeRequest{Name: name, Webhook: h.srv.URL, WebhookHeaders: map[string]string{"X-Secret": "s3cret"}})
	if err != nil {
		t.Fatal(err)
	}
	paired, err := (&Client{BaseURL: f.relay.URL}).Pair(ctx, strings.ToLower(res.PairingCode))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (&Client{BaseURL: f.relay.URL}).Pair(ctx, res.PairingCode); err == nil {
		t.Fatal("pairing code worked twice")
	}
	u, id, ok := ParseNodeToken(paired.Token)
	if !ok || u != f.relay.URL || id != res.Node.ID {
		t.Fatalf("ParseNodeToken(%q) = %q, %q, %v", paired.Token, u, id, ok)
	}
	return &Client{BaseURL: f.relay.URL, Token: paired.Token}, paired.Node
}

func caller(t *testing.T, url, token string) *a2aclient.Client {
	t.Helper()
	ctx := context.Background()
	hc := &http.Client{Transport: &BearerTransport{Token: token}}
	card, err := agentcard.NewResolver(hc).Resolve(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	c, err := a2aclient.NewFromCard(ctx, card, a2aclient.WithJSONRPCTransport(hc))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func sendAsync(t *testing.T, c *a2aclient.Client, msg *a2a.Message) *a2a.Task {
	t.Helper()
	res, err := c.SendMessage(context.Background(), &a2a.SendMessageRequest{Message: msg, Config: &a2a.SendMessageConfig{ReturnImmediately: true}})
	if err != nil {
		t.Fatal(err)
	}
	task, ok := res.(*a2a.Task)
	if !ok {
		t.Fatalf("SendMessage returned %T, want a task", res)
	}
	return task
}

func waitState(t *testing.T, c *a2aclient.Client, id a2a.TaskID, want a2a.TaskState) *a2a.Task {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		task, err := c.GetTask(context.Background(), &a2a.GetTaskRequest{ID: id})
		if err != nil {
			t.Fatal(err)
		}
		if task.Status.State == want {
			return task
		}
		if time.Now().After(deadline) {
			t.Fatalf("task state = %s, want %s", task.Status.State, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestTaskLifecycle(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, Config{})
	h := newHook(t)
	node, info := f.attach(t, h, "grok")

	// The node is private: anonymous callers are rejected.
	_, err := caller(t, info.URL, "").SendMessage(ctx, &a2a.SendMessageRequest{Message: a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("hi"))})
	if err == nil {
		t.Fatal("anonymous call to a private node succeeded")
	}

	g, err := node.Grant(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	alice := caller(t, info.URL, g.Token)

	task := sendAsync(t, alice, a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("summarize the news")))
	if task.Status.State != a2a.TaskStateSubmitted {
		t.Fatalf("state = %s, want submitted", task.Status.State)
	}
	wake := h.next(t, KindTask)
	if wake.TaskID != string(task.ID) || wake.Text != "summarize the news" || wake.Sender != (Sender{Kind: SenderGrant, Name: "alice"}) {
		t.Fatalf("unexpected wake %+v", wake)
	}
	if wake.ReplyURL != f.relay.URL+"/api/tasks/"+string(task.ID)+"/events" || wake.Instructions == "" {
		t.Fatalf("wake lacks reply info: %+v", wake)
	}

	inbox, err := node.Inbox(ctx, false)
	if err != nil || len(inbox.Entries) != 1 || !inbox.Entries[0].Open {
		t.Fatalf("Inbox() = %+v, %v; want one open entry", inbox, err)
	}

	if _, err := node.PostEvent(ctx, string(task.ID), Event{State: "working", Text: "reading"}); err != nil {
		t.Fatal(err)
	}
	waitState(t, alice, task.ID, a2a.TaskStateWorking)

	if _, err := node.PostEvent(ctx, string(task.ID), Event{State: "input-required", Text: "which topic?", IdempotencyKey: "q1"}); err != nil {
		t.Fatal(err)
	}
	got := waitState(t, alice, task.ID, a2a.TaskStateInputRequired)
	if TextOf(got.Status.Message) != "which topic?" {
		t.Fatalf("status message = %q", TextOf(got.Status.Message))
	}

	// The caller answers under the same task, which wakes the node again.
	answer := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("AI"))
	answer.TaskID, answer.ContextID = task.ID, task.ContextID
	sendAsync(t, alice, answer)
	wake = h.next(t, KindMessage)
	if wake.TaskID != string(task.ID) || wake.Text != "AI" {
		t.Fatalf("unexpected follow-up wake %+v", wake)
	}

	final := Event{
		State: "completed", Text: "Here is the summary.", IdempotencyKey: "final",
		Artifacts: []*a2a.Artifact{{Name: "notes.txt", Parts: []*a2a.Part{a2a.NewTextPart("notes")}}},
	}
	res, err := node.PostEvent(ctx, string(task.ID), final)
	if err != nil || res.Duplicate || res.State != "completed" {
		t.Fatalf("PostEvent(final) = %+v, %v", res, err)
	}
	// A retry with the same key is acknowledged, not applied.
	res, err = node.PostEvent(ctx, string(task.ID), final)
	if err != nil || !res.Duplicate {
		t.Fatalf("retried PostEvent = %+v, %v; want duplicate", res, err)
	}
	// A new event for a finished task is rejected.
	if _, err := node.PostEvent(ctx, string(task.ID), Event{State: "working"}); err == nil {
		t.Fatal("event for a completed task was accepted")
	}

	got = waitState(t, alice, task.ID, a2a.TaskStateCompleted)
	var names []string
	for _, a := range got.Artifacts {
		names = append(names, a.Name)
	}
	if strings.Join(names, ",") != "notes.txt,response" {
		t.Fatalf("artifacts = %v", names)
	}

	viaNode, err := node.Task(ctx, string(task.ID))
	if err != nil || viaNode.Status.State != a2a.TaskStateCompleted {
		t.Fatalf("node Task() = %v, %v", viaNode, err)
	}
	pending, err := node.Inbox(ctx, false)
	if err != nil {
		t.Fatalf("node.Inbox() error = %v", err)
	}
	if len(pending.Entries) != 0 {
		t.Fatalf("inbox not empty after completion: %+v", pending.Entries)
	}
}

func TestStreamingCaller(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, Config{})
	h := newHook(t)
	node, info := f.attach(t, h, "grok")
	if _, err := node.UpdateNode(ctx, UpdateNodeRequest{Public: new(true)}); err != nil {
		t.Fatal(err)
	}

	go func() {
		wake := h.next(t, KindTask)
		if wake.Sender.Kind != SenderAnonymous {
			t.Errorf("sender = %v, want anonymous", wake.Sender)
		}
		for _, ev := range []Event{{State: "working", Text: "on it"}, {State: "completed", Text: "42"}} {
			if _, err := node.PostEvent(ctx, wake.TaskID, ev); err != nil {
				t.Errorf("node.PostEvent() error = %v", err)
			}
		}
	}()

	var states []string
	var answer strings.Builder
	for ev, err := range caller(t, info.URL, "").SendStreamingMessage(ctx, &a2a.SendMessageRequest{
		Message: a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("question")),
	}) {
		if err != nil {
			t.Fatal(err)
		}
		switch e := ev.(type) {
		case *a2a.Task:
			states = append(states, StateName(e.Status.State))
		case *a2a.TaskStatusUpdateEvent:
			states = append(states, StateName(e.Status.State))
		case *a2a.TaskArtifactUpdateEvent:
			answer.WriteString(textOfParts(e.Artifact.Parts))
		}
	}
	if strings.Join(states, ",") != "submitted,working,completed" || answer.String() != "42" {
		t.Fatalf("states = %v, answer = %q", states, answer.String())
	}
}

func TestTimeout(t *testing.T) {
	f := newFixture(t, Config{TaskTimeout: 200 * time.Millisecond})
	h := newHook(t)
	node, info := f.attach(t, h, "sleepy")
	g := mustGrant(t, node, "bob")
	bob := caller(t, info.URL, g.Token)

	task := sendAsync(t, bob, a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("hello?")))
	h.next(t, KindTask)
	got := waitState(t, bob, task.ID, a2a.TaskStateFailed)
	if !strings.Contains(TextOf(got.Status.Message), "did not respond") {
		t.Fatalf("status message = %q", TextOf(got.Status.Message))
	}
}

func TestCancel(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, Config{})
	h := newHook(t)
	node, info := f.attach(t, h, "grok")
	g := mustGrant(t, node, "bob")
	bob := caller(t, info.URL, g.Token)

	task := sendAsync(t, bob, a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("long job")))
	h.next(t, KindTask)
	if _, err := bob.CancelTask(ctx, &a2a.CancelTaskRequest{ID: task.ID}); err != nil {
		t.Fatal(err)
	}
	h.next(t, KindCanceled)
	if _, err := node.PostEvent(ctx, string(task.ID), Event{State: "completed", Text: "late"}); err == nil {
		t.Fatal("event for a canceled task was accepted")
	}
}

func TestNodeToNodeSend(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, Config{})
	hookA, hookB := newHook(t), newHook(t)
	a, infoA := f.attach(t, hookA, "a")
	b, infoB := f.attach(t, hookB, "b")

	// b doesn't allow a yet.
	if _, err := a.Send(ctx, SendRequest{Peer: infoB.ID, Text: "hi"}); err == nil {
		t.Fatal("send to a node that doesn't allow the sender succeeded")
	}
	if _, err := b.Grant(ctx, "node:"+infoA.ID); err != nil {
		t.Fatal(err)
	}
	sent, err := a.Send(ctx, SendRequest{Peer: infoB.ID, Text: "what is 6*7?"})
	if err != nil {
		t.Fatal(err)
	}
	if sent.TaskID == "" || sent.Peer != infoB.URL {
		t.Fatalf("Send() = %+v", sent)
	}

	wake := hookB.next(t, KindTask)
	if wake.Sender != (Sender{Kind: SenderNode, Name: infoA.ID}) || wake.TaskID != sent.TaskID {
		t.Fatalf("unexpected wake for b: %+v", wake)
	}
	if _, err := b.PostEvent(ctx, wake.TaskID, Event{State: "completed", Text: "42"}); err != nil {
		t.Fatal(err)
	}

	result := hookA.next(t, KindResult)
	if result.TaskID != sent.TaskID || result.State != "completed" || result.Text != "42" || result.Task == nil {
		t.Fatalf("unexpected result wake %+v", result)
	}
}

func TestPersistedNodes(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	srv, err := New(Config{AdminToken: "admin", StateDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv)
	admin := &Client{BaseURL: ts.URL, Token: "admin"}
	res, err := admin.CreateNode(ctx, CreateNodeRequest{Webhook: "https://example.com/hook"})
	if err != nil {
		t.Fatal(err)
	}
	paired, err := (&Client{BaseURL: ts.URL}).Pair(ctx, res.PairingCode)
	if err != nil {
		t.Fatal(err)
	}
	ts.Close()

	srv2, err := New(Config{AdminToken: "admin", StateDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	ts2 := httptest.NewServer(srv2)
	defer ts2.Close()
	info, err := (&Client{BaseURL: ts2.URL, Token: paired.Token}).Node(ctx)
	if err != nil || info.ID != res.Node.ID {
		t.Fatalf("Node() after restart = %+v, %v", info, err)
	}
	if _, err := (&Client{BaseURL: ts2.URL, Token: "a2rn.bogus"}).Node(ctx); err == nil {
		t.Fatal("bogus token accepted")
	}
}

func TestParseState(t *testing.T) {
	for _, s := range []string{"working", "input-required", "INPUT_REQUIRED", "TASK_STATE_COMPLETED"} {
		if _, ok := ParseState(s); !ok {
			t.Errorf("ParseState(%q) failed", s)
		}
	}
	if _, ok := ParseState("submitted"); ok {
		t.Error("nodes must not post submitted")
	}
	if got := StateName(a2a.TaskStateInputRequired); got != "input-required" {
		t.Errorf("StateName = %q", got)
	}
}

func mustGrant(t *testing.T, node *Client, name string) *GrantResponse {
	t.Helper()
	g, err := node.Grant(context.Background(), name)
	if err != nil {
		t.Fatalf("node.Grant(%q) error = %v", name, err)
	}
	return g
}
