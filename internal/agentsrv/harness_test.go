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
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"

	"github.com/a2aproject/a2a-cli/internal/agentdir"
	"github.com/a2aproject/a2a-cli/internal/channel"
)

var fakeACP string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "fakeacp")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fakeACP = filepath.Join(dir, "fakeacp")
	build := exec.Command("go", "build", "-o", fakeACP, "./testdata/fakeacp")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "building fakeacp:", err)
		os.Exit(1)
	}
	watchInterval, watchMaxInterval = 200*time.Millisecond, time.Second
	webhookRetries = []time.Duration{0, 100 * time.Millisecond}
	channelRetryMin = 10 * time.Millisecond
	code := m.Run()
	if err := os.RemoveAll(dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(code)
}

// testServer is an agent server running fakeacp in a temporary directory.
type testServer struct {
	t    *testing.T
	dir  agentdir.Dir
	srv  *Server
	ch   *fakeChannel
	done chan error
}

type serverOpts struct {
	name     string
	dir      string
	channel  *fakeChannel
	approver string
}

// startServer starts an agent server and stops it when the test ends.
func startServer(t *testing.T, o serverOpts) *testServer {
	t.Helper()
	if o.name == "" {
		o.name = "coder"
	}
	if o.dir == "" {
		// Unix socket paths are short, so the agent lives in a short temp dir.
		d, err := os.MkdirTemp("", "a2a")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.RemoveAll(d); err != nil {
				t.Errorf("removing %s: %v", d, err)
			}
		})
		o.dir = d
	}
	dir, err := agentdir.Open(o.dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &agentdir.Config{Name: o.name, Harness: "fake", Command: []string{fakeACP}, Sandbox: SandboxLocal}
	if dir.Exists() {
		if cfg, err = dir.LoadConfig(); err != nil {
			t.Fatal(err)
		}
	}
	if o.approver != "" {
		cfg.Approvals.Approvers = []string{o.approver}
	}
	if err := dir.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	ts := &testServer{t: t, dir: dir, ch: o.channel, done: make(chan error, 1)}
	ready := make(chan *Server, 1)
	opts := Options{Dir: dir, Config: cfg, Secrets: agentdir.Secrets{}, Ready: func(s *Server) { ready <- s }}
	if o.channel != nil {
		opts.Channels = []channel.Channel{o.channel}
	}
	go func() { ts.done <- Run(context.Background(), opts) }()
	select {
	case ts.srv = <-ready:
	case err := <-ts.done:
		t.Fatalf("Run() error = %v", err)
	case <-time.After(30 * time.Second):
		t.Fatal("server did not start")
	}
	t.Cleanup(ts.stop)
	return ts
}

func (ts *testServer) stop() {
	if ts.srv == nil {
		return
	}
	ts.srv.Stop()
	select {
	case err := <-ts.done:
		if err != nil {
			ts.t.Errorf("Run() error = %v", err)
		}
	case <-time.After(30 * time.Second):
		ts.t.Errorf("server did not stop")
	}
	ts.srv = nil
}

func (ts *testServer) localURL() string {
	local, _, _ := ts.srv.URLs()
	return local
}

// pair mints a code over the control socket and exchanges it for a token.
func (ts *testServer) pair(name, role string) string {
	ts.t.Helper()
	ctx := context.Background()
	code, err := NewClient(ts.dir).Pair(ctx, PairCodeRequest{Name: name, Role: role})
	if err != nil {
		ts.t.Fatalf("Pair() error = %v", err)
	}
	tok, err := pairWith(ctx, http.DefaultClient, ts.localURL(), code.Code)
	if err != nil {
		ts.t.Fatalf("pairWith() error = %v", err)
	}
	return tok
}

// client returns an A2A client authenticated with token.
func (ts *testServer) client(token string) *a2aclient.Client {
	ts.t.Helper()
	u, err := url.Parse(ts.localURL())
	if err != nil {
		ts.t.Fatalf("url.Parse() error = %v", err)
	}
	hc := &http.Client{Transport: &headerTransport{base: http.DefaultTransport, host: u.Host, header: http.Header{"Authorization": {"Bearer " + token}}}}
	ctx := context.Background()
	card, err := agentcard.NewResolver(hc).Resolve(ctx, ts.localURL())
	if err != nil {
		ts.t.Fatalf("Resolve() error = %v", err)
	}
	c, err := a2aclient.NewFromCard(ctx, card, a2aclient.WithJSONRPCTransport(hc), a2aclient.WithRESTTransport(hc))
	if err != nil {
		ts.t.Fatalf("NewFromCard() error = %v", err)
	}
	ts.t.Cleanup(func() {
		if err := c.Destroy(); err != nil {
			ts.t.Errorf("Destroy() error = %v", err)
		}
	})
	return c
}

func userText(text, taskID, contextID string) *a2a.Message {
	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart(text))
	msg.TaskID, msg.ContextID = a2a.TaskID(taskID), contextID
	return msg
}

func send(t *testing.T, c *a2aclient.Client, msg *a2a.Message) *a2a.Task {
	t.Helper()
	res, err := c.SendMessage(context.Background(), &a2a.SendMessageRequest{Message: msg})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	task, ok := res.(*a2a.Task)
	if !ok {
		t.Fatalf("SendMessage() = %T, want *a2a.Task", res)
	}
	return task
}

func sendAsync(t *testing.T, c *a2aclient.Client, msg *a2a.Message) *a2a.Task {
	t.Helper()
	res, err := c.SendMessage(context.Background(), &a2a.SendMessageRequest{Message: msg, Config: &a2a.SendMessageConfig{ReturnImmediately: true}})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	return res.(*a2a.Task)
}

// waitState polls a task until it reaches state.
func waitState(t *testing.T, c *a2aclient.Client, id a2a.TaskID, state a2a.TaskState) *a2a.Task {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		task, err := c.GetTask(context.Background(), &a2a.GetTaskRequest{ID: id})
		if err != nil {
			t.Fatalf("GetTask() error = %v", err)
		}
		if task.Status.State == state {
			return task
		}
		if time.Now().After(deadline) {
			t.Fatalf("task %s state = %s, want %s", id, task.Status.State, state)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func wantResult(t *testing.T, task *a2a.Task, state a2a.TaskState, text string) {
	t.Helper()
	if task.Status.State != state {
		t.Fatalf("task state = %s (%s), want %s", task.Status.State, messageText(task.Status.Message), state)
	}
	if got := resultText(task); !strings.Contains(got, text) {
		t.Fatalf("task result = %q, want it to contain %q", got, text)
	}
}

// fakeChannel records what the router sends and lets tests send messages.
type fakeChannel struct {
	name    string
	buttons bool
	// failures is how many times Run fails before it connects.
	failures int

	mu    sync.Mutex
	in    chan<- channel.Inbound
	seq   int
	msgs  map[string]channel.Outbound
	order []string
	ready chan struct{}
}

func newFakeChannel(name string) *fakeChannel {
	return &fakeChannel{name: name, buttons: true, msgs: map[string]channel.Outbound{}, ready: make(chan struct{})}
}

func (f *fakeChannel) Name() string { return f.name }

func (f *fakeChannel) Caps() channel.Caps {
	return channel.Caps{Edit: true, Buttons: f.buttons, MaxText: 4000, Stream: true}
}

func (f *fakeChannel) Describe() string { return "@fake_bot" }

func (f *fakeChannel) LinkHint(code string) string { return "/start " + code }

func (f *fakeChannel) Run(ctx context.Context, in chan<- channel.Inbound) error {
	f.mu.Lock()
	if f.failures > 0 {
		f.failures--
		f.mu.Unlock()
		return errors.New("messenger unreachable")
	}
	f.in = in
	f.mu.Unlock()
	close(f.ready)
	<-ctx.Done()
	return nil
}

func (f *fakeChannel) Send(_ context.Context, m channel.Outbound) (channel.Ref, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	id := strconv.Itoa(f.seq)
	f.msgs[id] = m
	f.order = append(f.order, id)
	return channel.Ref{Conv: m.Conv, ID: id}, nil
}

func (f *fakeChannel) Edit(_ context.Context, ref channel.Ref, m channel.Outbound) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.msgs[ref.ID] = m
	return nil
}

var msgSeq struct {
	sync.Mutex
	n int
}

// write delivers a message from identity in conversation conv.
func (f *fakeChannel) write(identity, conv, text string) {
	f.deliver(f.message(identity, conv, text))
}

func (f *fakeChannel) message(identity, conv, text string) channel.Inbound {
	msgSeq.Lock()
	msgSeq.n++
	id := strconv.Itoa(msgSeq.n)
	msgSeq.Unlock()
	return channel.Inbound{Channel: f.name, MessageID: "m" + id, Identity: identity, Name: identity, Conv: channel.ConvKey(conv), Direct: true, Text: text, Authentic: true}
}

func (f *fakeChannel) deliver(in channel.Inbound) {
	<-f.ready
	f.in <- in
}

// waitReady waits until the router runs the channel.
func (f *fakeChannel) waitReady(t *testing.T) {
	t.Helper()
	select {
	case <-f.ready:
	case <-time.After(10 * time.Second):
		t.Fatal("channel did not start")
	}
}

// click presses option of a question message.
func (f *fakeChannel) click(identity, conv string, ref string, q *channel.Question, option int) {
	<-f.ready
	msgSeq.Lock()
	msgSeq.n++
	id := strconv.Itoa(msgSeq.n)
	msgSeq.Unlock()
	f.in <- channel.Inbound{Channel: f.name, MessageID: "cb" + id, Identity: identity, Conv: channel.ConvKey(conv), Direct: true,
		Action: &channel.Action{QuestionID: q.ID, Option: option}, Authentic: true, ReplyTo: channel.Ref{Conv: channel.ConvKey(conv), ID: ref}}
}

// waitFor waits until a message in conv satisfies pred and returns its ID.
func (f *fakeChannel) waitFor(t *testing.T, conv string, pred func(channel.Outbound) bool) (string, channel.Outbound) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		for _, id := range f.order {
			m := f.msgs[id]
			if string(m.Conv) == conv && pred(m) {
				f.mu.Unlock()
				return id, m
			}
		}
		f.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var all []string
	for _, id := range f.order {
		all = append(all, fmt.Sprintf("%s[%s]: %q q=%v", id, f.msgs[id].Conv, f.msgs[id].Text, f.msgs[id].Question))
	}
	t.Fatalf("no matching message in %s; messages:\n%s", conv, strings.Join(all, "\n"))
	return "", channel.Outbound{}
}

func hasText(s string) func(channel.Outbound) bool {
	return func(m channel.Outbound) bool { return strings.Contains(m.Text, s) }
}

func hasQuestion(m channel.Outbound) bool { return m.Question != nil }
