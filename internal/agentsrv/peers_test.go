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
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"

	"github.com/a2aproject/a2a-cli/internal/auth"
	"github.com/a2aproject/a2a-cli/internal/controlapi"
	"github.com/a2aproject/a2a-cli/internal/credstore"
	"github.com/a2aproject/a2a-cli/internal/store"
)

// addContact pairs caller with callee as a contact named name.
func addContact(t *testing.T, caller, callee *testServer, name, role string) {
	t.Helper()
	ctx := context.Background()
	code, err := controlapi.NewClient(callee.dir).Pair(ctx, controlapi.PairCodeRequest{Name: caller.srv.config().Name, Role: role})
	if err != nil {
		t.Fatal(err)
	}
	c, err := controlapi.NewClient(caller.dir).AddContact(ctx, controlapi.ContactRequest{Name: name, URL: callee.localURL(), Code: code.Code})
	if err != nil {
		t.Fatalf("AddContact() error = %v", err)
	}
	if !c.HasToken || c.Via != ViaHTTPS {
		t.Fatalf("AddContact() = %+v, want an https contact with a token", c)
	}
}

func TestAgentCallsContact(t *testing.T) {
	t.Parallel()
	coder := startServer(t, serverOpts{name: "coder"})
	assistant := startServer(t, serverOpts{name: "assistant"})
	addContact(t, assistant, coder, "coder", auth.RoleMember)

	c := assistant.client(assistant.pair("me", auth.RoleOwner))
	task := send(t, c, userText("call coder hello", "", ""))
	wantResult(t, task, a2a.TaskStateCompleted, "Hello from fakeacp.")
	if !strings.Contains(resultText(task), "tool result") {
		t.Fatalf("task result = %q, want the called agent's result", resultText(task))
	}
	if strings.Contains(messageText(task.History[0]), agentsLinePrefix) {
		t.Fatalf("task history = %q, want the caller's message without the agents line", messageText(task.History[0]))
	}

	unknown := send(t, c, userText("call nobody hi", "", ""))
	wantResult(t, unknown, a2a.TaskStateCompleted, "404")
}

func TestRemoteApprovalIsRelayedToCallerChat(t *testing.T) {
	t.Parallel()
	coder := startServer(t, serverOpts{name: "coder"})
	assistant := startServer(t, serverOpts{name: "assistant", channel: newFakeChannel("fake")})
	addContact(t, assistant, coder, "coder", auth.RoleOwner)
	linkOwner(t, assistant, "tg:1", "dm1")

	assistant.ch.write("tg:1", "dm1", "call coder ask")
	ref, q := assistant.ch.waitFor(t, "dm1", hasQuestion)
	if !strings.Contains(q.Text, "coder wants to run git push") {
		t.Fatalf("question text = %q, want it to name coder and the tool", q.Text)
	}
	assistant.ch.click("tg:1", "dm1", ref, q.Question, 0)
	assistant.ch.waitFor(t, "dm1", hasText("✓ Allow by owner"))

	ctx := context.Background()
	contextID, err := assistant.srv.store.ContextOf(ctx, "fake", "dm1")
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		last, err := assistant.srv.tasks.LatestInContext(ctx, contextID)
		if err != nil {
			t.Fatal(err)
		}
		if len(last.History) > 0 && strings.Contains(messageText(last.History[0]), "Pushed (allow).") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("latest turn history = %v, want coder's result posted into the conversation", last.History)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestRelayedApprovalSurvivesRestart(t *testing.T) {
	t.Parallel()
	coder := startServer(t, serverOpts{name: "coder"})
	assistant := startServer(t, serverOpts{name: "assistant", channel: newFakeChannel("fake")})
	addContact(t, assistant, coder, "coder", auth.RoleOwner)
	linkOwner(t, assistant, "tg:1", "dm1")

	assistant.ch.write("tg:1", "dm1", "call coder ask")
	ref, q := assistant.ch.waitFor(t, "dm1", hasQuestion)
	root := assistant.dir.Root
	assistant.stop()

	restarted := startServer(t, serverOpts{name: "assistant", dir: root, channel: newFakeChannel("fake")})
	restarted.ch.waitReady(t)
	time.Sleep(3 * watchMaxInterval)
	if n := restarted.ch.questions(); n != 0 {
		t.Fatalf("questions asked again after the restart = %d, want 0", n)
	}
	restarted.ch.click("tg:1", "dm1", ref, q.Question, 0)
	restarted.ch.waitEdited(t, ref, "✓ Allow by owner")
}

func TestContactClientIsShared(t *testing.T) {
	t.Parallel()
	coder := startServer(t, serverOpts{name: "coder"})
	assistant := startServer(t, serverOpts{name: "assistant"})
	addContact(t, assistant, coder, "coder", auth.RoleOwner)
	p, ctx := assistant.srv.peers, context.Background()

	first, err := p.contactClient(ctx, "coder")
	if err != nil {
		t.Fatalf("contactClient() error = %v", err)
	}
	for _, user := range []string{auth.OwnerID, "pair:anna"} {
		tg, err := p.target(ctx, "coder", user, 1)
		if err != nil {
			t.Fatalf("target() error = %v", err)
		}
		if got := tg.(remoteTarget).c; got != first {
			t.Fatalf("target() for %s built a new client, want the contact's client", user)
		}
	}

	c, err := p.store.Contact(ctx, "coder")
	if err != nil {
		t.Fatal(err)
	}
	c.Token = coder.pair("assistant-again", auth.RoleOwner)
	if err := p.store.PutContact(ctx, c); err != nil {
		t.Fatal(err)
	}
	second, err := p.contactClient(ctx, "coder")
	if err != nil {
		t.Fatalf("contactClient() after a token change error = %v", err)
	}
	if second == first {
		t.Fatalf("contactClient() after a token change = the old client, want a new one")
	}

	if err := p.store.DeleteContact(ctx, "coder"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.contactClient(ctx, "coder"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("contactClient() of a removed contact error = %v, want %v", err, store.ErrNotFound)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.clients) != 0 {
		t.Fatalf("clients after removing the contact = %d, want 0", len(p.clients))
	}
}

func TestContactCountsHops(t *testing.T) {
	t.Parallel()
	coder := startServer(t, serverOpts{name: "coder"})
	assistant := startServer(t, serverOpts{name: "assistant"})
	addContact(t, assistant, coder, "coder", auth.RoleOwner)
	p, ctx := assistant.srv.peers, context.Background()
	tests := []struct {
		name    string
		hops    int
		wantErr bool
	}{
		{name: "first hop", hops: 0},
		{name: "past the limit", hops: auth.MaxHops, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tg, err := p.target(ctx, "coder", auth.OwnerID, tc.hops)
			if err != nil {
				t.Fatalf("target() error = %v", err)
			}
			_, err = tg.send(ctx, userText("hello", "", ""))
			if (err != nil) != tc.wantErr {
				t.Fatalf("send() at %d hops error = %v, want error %v", tc.hops, err, tc.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "too many agents") {
				t.Fatalf("send() error = %v, want the hop limit", err)
			}
		})
	}
}

func TestAgentsURLHandles(t *testing.T) {
	t.Parallel()
	ts := startServer(t, serverOpts{})
	p := ts.srv.peers
	ctx := context.Background()
	owner := callClaims{ContextID: "ctx1", OnBehalfOf: auth.OwnerID, Hops: 1}
	h, err := p.handle(ctx, owner)
	if err != nil {
		t.Fatalf("handle() error = %v", err)
	}
	if again, err := p.handle(ctx, owner); err != nil || again != h {
		t.Fatalf("handle() again = %q, %v, want %q", again, err, h)
	}
	other, err := p.handle(ctx, callClaims{ContextID: "ctx1", OnBehalfOf: "pair:anna"})
	if err != nil {
		t.Fatalf("handle() error = %v", err)
	}
	if other == h {
		t.Fatalf("handle() for another user = %q, want a different handle", other)
	}
	got, ok := p.claimsOf(ctx, h)
	if !ok {
		t.Fatalf("claimsOf(%q) ok = false, want true", h)
	}
	if diff := cmp.Diff(owner, got); diff != "" {
		t.Fatalf("claimsOf() wrong result (-want +got) diff = %s", diff)
	}
	if _, ok := p.claimsOf(ctx, "unknown"); ok {
		t.Fatalf("claimsOf(unknown) ok = true, want false")
	}
}

// harnessClient returns a client which reaches ts the way its harness does.
func harnessClient(ts *testServer) *http.Client {
	var d net.Dialer
	return &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return d.DialContext(ctx, "unix", ts.srv.bridge.socket)
	}}}
}

// callAgent sends text through an agents URL and waits for a settled task.
func callAgent(t *testing.T, hc *http.Client, agentURL, text string) (*a2a.Task, error) {
	t.Helper()
	ctx := context.Background()
	card, err := agentcard.NewResolver(hc).Resolve(ctx, agentURL)
	if err != nil {
		return nil, err
	}
	client, err := a2aclient.NewFromCard(ctx, card, a2aclient.WithJSONRPCTransport(hc), a2aclient.WithRESTTransport(hc))
	if err != nil {
		t.Fatalf("NewFromCard() error = %v", err)
	}
	res, err := client.SendMessage(ctx, &a2a.SendMessageRequest{Message: a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart(text))})
	if err != nil {
		return nil, err
	}
	task, ok := res.(*a2a.Task)
	if !ok {
		t.Fatalf("SendMessage() = %T, want a task", res)
	}
	deadline := time.Now().Add(10 * time.Second)
	for !task.Status.State.Terminal() && task.Status.State != a2a.TaskStateInputRequired && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		if task, err = client.GetTask(ctx, &a2a.GetTaskRequest{ID: task.ID}); err != nil {
			t.Fatalf("GetTask() error = %v", err)
		}
	}
	return task, nil
}

func TestAgentsProxy(t *testing.T) {
	t.Parallel()
	ts := startServer(t, serverOpts{name: "assistant"})
	addNode(t, ts, "grok", 0)
	u, err := ts.srv.peers.agentsURL(context.Background(), callClaims{ContextID: "c1", OnBehalfOf: auth.OwnerID})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{name: "node", url: u + "/grok", want: "grok says: hi"},
		{name: "unknown agent", url: u + "/nobody", wantErr: true},
		{name: "unknown handle", url: socketHost + "/unknown/grok", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			task, err := callAgent(t, harnessClient(ts), tc.url, "hi")
			if (err != nil) != tc.wantErr {
				t.Fatalf("callAgent() error = %v, want error %v", err, tc.wantErr)
			}
			if err == nil && resultText(task) != tc.want {
				t.Fatalf("task result = %q, want %q", resultText(task), tc.want)
			}
		})
	}
}

func TestAgentsProxyOverTCPNeedsKey(t *testing.T) {
	t.Parallel()
	ts := startServer(t, serverOpts{name: "assistant"})
	addNode(t, ts, "grok", 0)
	hs := httptest.NewServer(ts.srv.peers.harnessRoutes("harness-key"))
	t.Cleanup(hs.Close)
	h, err := ts.srv.peers.handle(context.Background(), callClaims{ContextID: "c1", OnBehalfOf: auth.OwnerID})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		header string
		want   int
	}{
		{name: "no key", want: http.StatusUnauthorized},
		{name: "wrong key", header: "Bearer nope", want: http.StatusUnauthorized},
		{name: "key", header: "Bearer harness-key", want: http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, hs.URL+"/"+h+"/grok/.well-known/agent-card.json", nil)
			if err != nil {
				t.Fatal(err)
			}
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			if err := resp.Body.Close(); err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != tc.want {
				t.Fatalf("GET card status = %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
}

func TestSaveHarnessKey(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	cs := &credstore.Store{Path: filepath.Join(home, ".config", "a2a-cli", "credentials.json")}
	if err := cs.Put(credstore.Credential{URL: "https://other.example.com", Token: "a2p_mine"}); err != nil {
		t.Fatal(err)
	}
	for _, port := range []string{"40001", "40002"} {
		if err := saveHarnessKey(home, "http://host.docker.internal:"+port, "key-"+port); err != nil {
			t.Fatalf("saveHarnessKey() error = %v", err)
		}
	}
	creds, err := cs.List()
	if err != nil {
		t.Fatal(err)
	}
	var tokens []string
	for _, c := range creds {
		tokens = append(tokens, c.Token)
	}
	if diff := cmp.Diff([]string{"a2p_mine", "key-40002"}, tokens); diff != "" {
		t.Fatalf("credentials wrong result (-want +got) diff = %s", diff)
	}
	got, ok, err := cs.Lookup("http://host.docker.internal:40002/k3/coder")
	if err != nil || !ok || got.Token != "key-40002" {
		t.Fatalf("Lookup() = %+v, %v, %v, want the current key", got, ok, err)
	}
}

func TestHarnessCannotAnswerRemoteApproval(t *testing.T) {
	t.Parallel()
	coder := startServer(t, serverOpts{name: "coder"})
	assistant := startServer(t, serverOpts{name: "assistant"})
	addContact(t, assistant, coder, "coder", auth.RoleOwner)
	u, err := assistant.srv.peers.agentsURL(context.Background(), callClaims{ContextID: "c1", OnBehalfOf: auth.OwnerID})
	if err != nil {
		t.Fatal(err)
	}
	hc := harnessClient(assistant)
	task, err := callAgent(t, hc, u+"/coder", "ask")
	if err != nil {
		t.Fatalf("callAgent() error = %v", err)
	}
	if task.Status.State != a2a.TaskStateInputRequired {
		t.Fatalf("task state = %v, want a task waiting for an approval", task.Status.State)
	}
	ctx := context.Background()
	card, err := agentcard.NewResolver(hc).Resolve(ctx, u+"/coder")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	client, err := a2aclient.NewFromCard(ctx, card, a2aclient.WithJSONRPCTransport(hc), a2aclient.WithRESTTransport(hc))
	if err != nil {
		t.Fatalf("NewFromCard() error = %v", err)
	}
	answer := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("allow"))
	answer.TaskID, answer.ContextID = task.ID, task.ContextID
	if _, err := client.SendMessage(ctx, &a2a.SendMessageRequest{Message: answer}); err == nil {
		t.Fatalf("SendMessage() answering an approval error = nil, want error")
	}
}
