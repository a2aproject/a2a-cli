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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/google/go-cmp/cmp"

	"github.com/a2aproject/a2a-cli/internal/auth"
	"github.com/a2aproject/a2a-cli/internal/controlapi"
	"github.com/a2aproject/a2a-cli/internal/credstore"
	"github.com/a2aproject/a2a-cli/internal/pairapi"
	"github.com/a2aproject/a2a-cli/internal/relayapi"
)

// fakeNode is a webhook node which answers every task after delay.
type fakeNode struct {
	t      *testing.T
	delay  time.Duration
	answer string

	mu     sync.Mutex
	client *relayapi.Client
	secret func() string
	wakes  []relayapi.Envelope
	badSig bool
}

func (n *fakeNode) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		n.t.Errorf("reading wake: %v", err)
		return
	}
	var env relayapi.Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		n.t.Errorf("decoding wake: %v", err)
		return
	}
	n.mu.Lock()
	n.wakes = append(n.wakes, env)
	if relayapi.Verify(n.secret(), r.Header.Get(relayapi.SignatureHeader), body, time.Now(), time.Minute) != nil {
		n.badSig = true
	}
	client := n.client
	n.mu.Unlock()
	w.WriteHeader(http.StatusAccepted)
	if env.Kind != relayapi.KindTask {
		return
	}
	go func() {
		time.Sleep(n.delay)
		ev := relayapi.Event{State: "completed", Text: n.answer + ": " + env.Text}
		ev.IdempotencyKey = relayapi.ContentKey(env.TaskID, ev)
		if _, err := client.PostEvent(context.Background(), env.TaskID, ev); err != nil {
			n.t.Errorf("PostEvent() error = %v", err)
		}
	}()
}

// addNode hosts a fake node on ts and pairs it.
func addNode(t *testing.T, ts *testServer, id string, delay time.Duration) *fakeNode {
	t.Helper()
	node := &fakeNode{t: t, delay: delay, answer: "grok says"}
	hs := httptest.NewServer(node)
	t.Cleanup(hs.Close)
	ctx := context.Background()
	resp, err := controlapi.NewClient(ts.dir).Pair(ctx, controlapi.PairCodeRequest{Name: id, Webhook: hs.URL, Headers: map[string]string{"Authorization": "Bearer hook"}})
	if err != nil {
		t.Fatalf("Pair() error = %v", err)
	}
	if resp.Node == nil || resp.PairCommand != "a2a pair add "+ts.localURL()+" "+resp.Code {
		t.Fatalf("Pair() = %+v, want a node code and its pair command", resp)
	}
	wantInstructions := fmt.Sprintf("You are the webhook node %q of the agent at %s.\nRead %s/relay/skill.md and follow it.\nYour pairing code is %s.", id, ts.localURL(), ts.localURL(), resp.Code)
	if diff := cmp.Diff(wantInstructions, resp.Instructions); diff != "" {
		t.Fatalf("Pair() wrong instructions (-want +got) diff = %s", diff)
	}
	paired, err := credstore.Pair(ctx, http.DefaultClient, ts.localURL(), resp.Code)
	if err != nil {
		t.Fatalf("Pair() error = %v", err)
	}
	if paired.Kind != credstore.KindNode || paired.URL != ts.localURL()+"/n/"+id {
		t.Fatalf("Pair() = %+v, want a node token for %s/n/%s", paired, ts.localURL(), id)
	}
	client, err := relayapi.NodeClient(paired.Token)
	if err != nil {
		t.Fatalf("NodeClient() error = %v", err)
	}
	if base, nodeID, _ := relayapi.ParseNodeToken(paired.Token); nodeID != id || base != relayapi.APIBase(ts.localURL()) {
		t.Fatalf("ParseNodeToken() = %q, %q, want %q, %q", base, nodeID, relayapi.APIBase(ts.localURL()), id)
	}
	node.mu.Lock()
	node.client = client
	node.secret = func() string {
		n, err := ts.srv.store.Node(context.Background(), id)
		if err != nil {
			t.Errorf("Node() error = %v", err)
		}
		return n.Secret
	}
	node.mu.Unlock()
	return node
}

func TestWebhookNodeAnswersAgent(t *testing.T) {
	t.Parallel()
	ts := startServer(t, serverOpts{name: "assistant"})
	node := addNode(t, ts, "grok", 50*time.Millisecond)

	c := ts.client(ts.pair("me", auth.RoleOwner))
	task := send(t, c, userText("call grok what's new?", "", ""))
	wantResult(t, task, a2a.TaskStateCompleted, "grok says: what's new?")

	node.mu.Lock()
	defer node.mu.Unlock()
	if len(node.wakes) == 0 || node.wakes[0].Sender.Kind != relayapi.SenderAgent || node.wakes[0].Sender.OnBehalfOf != auth.OwnerID {
		t.Fatalf("wakes = %+v, want a wake from the agent on behalf of the owner", node.wakes)
	}
	if node.badSig {
		t.Fatalf("a wake had an invalid %s", relayapi.SignatureHeader)
	}
}

func TestNodeTokenCallsAgent(t *testing.T) {
	t.Parallel()
	ts := startServer(t, serverOpts{name: "assistant"})
	node := addNode(t, ts, "grok", 0)
	node.mu.Lock()
	token := node.client.Token
	node.mu.Unlock()

	c := ts.client(token)
	task := send(t, c, userText("hello", "", ""))
	wantResult(t, task, a2a.TaskStateCompleted, "Hello from fakeacp.")

	if _, err := controlapi.NewClient(ts.dir).Revoke(context.Background(), "grok"); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if _, err := c.SendMessage(context.Background(), &a2a.SendMessageRequest{Message: userText("hello", "", "")}); err == nil {
		t.Fatalf("SendMessage() with the token of a removed node error = nil, want error")
	}
}

func TestContactRejectsNodeCode(t *testing.T) {
	t.Parallel()
	coder := startServer(t, serverOpts{name: "coder"})
	assistant := startServer(t, serverOpts{name: "assistant"})
	ctx := context.Background()
	code, err := controlapi.NewClient(coder.dir).Pair(ctx, controlapi.PairCodeRequest{Name: "grok", Webhook: "https://example.com/hook"})
	if err != nil {
		t.Fatalf("Pair() error = %v", err)
	}
	if _, err := controlapi.NewClient(assistant.dir).AddContact(ctx, controlapi.ContactRequest{Name: "coder", URL: coder.localURL(), Code: code.Code}); err == nil {
		t.Fatalf("AddContact() with a node code error = nil, want error")
	}
}

func TestPairNodeOrCaller(t *testing.T) {
	t.Parallel()
	ts := startServer(t, serverOpts{name: "assistant"})
	addNode(t, ts, "grok", 0)
	ts.pair("anna", auth.RoleMember)
	tests := []struct {
		name     string
		req      controlapi.PairCodeRequest
		wantNode bool
		wantErr  bool
	}{
		{name: "new caller", req: controlapi.PairCodeRequest{Name: "bob"}},
		{name: "existing node gets a node code", req: controlapi.PairCodeRequest{Name: "grok"}, wantNode: true},
		{name: "existing node with a new webhook", req: controlapi.PairCodeRequest{Name: "grok", Webhook: "https://example.com/new"}, wantNode: true},
		{name: "role for a node", req: controlapi.PairCodeRequest{Name: "grok", Role: auth.RoleOwner}, wantErr: true},
		{name: "webhook for a caller's name", req: controlapi.PairCodeRequest{Name: "anna", Webhook: "https://example.com/hook"}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := controlapi.NewClient(ts.dir).Pair(context.Background(), tc.req)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Pair() error = %v, want error %v", err, tc.wantErr)
			}
			if err == nil && (resp.Node != nil) != tc.wantNode {
				t.Fatalf("Pair() node = %+v, want node %v", resp.Node, tc.wantNode)
			}
		})
	}
}

func TestPairRmRemovesNodeOrCaller(t *testing.T) {
	t.Parallel()
	ts := startServer(t, serverOpts{name: "assistant"})
	addNode(t, ts, "grok", 0)
	ts.pair("anna", auth.RoleMember)
	ctx := context.Background()
	for name, want := range map[string]string{"grok": pairapi.KindNode, "anna": pairapi.KindCaller} {
		resp, err := controlapi.NewClient(ts.dir).Revoke(ctx, name)
		if err != nil {
			t.Fatalf("Revoke(%q) error = %v", name, err)
		}
		if resp.Kind != want {
			t.Fatalf("Revoke(%q) = %q, want %q", name, resp.Kind, want)
		}
	}
	if _, err := ts.srv.store.Node(ctx, "grok"); err == nil {
		t.Fatalf("Node() after Revoke() error = nil, want error")
	}
	if _, err := controlapi.NewClient(ts.dir).Revoke(ctx, "nobody"); err == nil {
		t.Fatalf("Revoke() unknown name error = nil, want error")
	}
}

func TestWebhookNodeRejectsMembers(t *testing.T) {
	t.Parallel()
	ts := startServer(t, serverOpts{name: "assistant"})
	addNode(t, ts, "grok", 0)
	tok := ts.pair("grok-caller", auth.RoleMember)
	req, err := http.NewRequest(http.MethodPost, ts.localURL()+"/n/grok/", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"SendMessage","params":{"message":{"messageId":"m","role":"ROLE_USER","parts":[{"text":"hi"}]}}}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("A2A-Version", "1.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	if err := errors.Join(err, resp.Body.Close()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "error") {
		t.Fatalf("member call to a node = %s, want an error", body)
	}
}

func TestLongCallIsWatchedAndPostedBack(t *testing.T) {
	t.Parallel()
	ts := startServer(t, serverOpts{name: "assistant", channel: newFakeChannel("fake")})
	addNode(t, ts, "grok", 3*time.Second)
	linkOwner(t, ts, "tg:1", "dm1")

	ts.ch.write("tg:1", "dm1", "acall grok what's new?")
	ts.ch.waitFor(t, "dm1", hasText("tool result: task"))
	ts.ch.waitFor(t, "dm1", hasText("Hello from fakeacp."))

	contextID, err := ts.srv.store.ContextOf(context.Background(), "fake", "dm1")
	if err != nil {
		t.Fatal(err)
	}
	last, err := ts.srv.tasks.LatestInContext(context.Background(), contextID)
	if err != nil {
		t.Fatal(err)
	}
	if len(last.History) == 0 || !strings.Contains(messageText(last.History[0]), "grok says: what's new?") {
		t.Fatalf("injected turn history = %v, want the node's result", last.History)
	}
}
