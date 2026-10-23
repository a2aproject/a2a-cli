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
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"github.com/a2aproject/a2a-cli/internal/auth"
	"github.com/a2aproject/a2a-cli/internal/relayapi"
	"github.com/a2aproject/a2a-cli/internal/store"
)

// nodeTaskTimeout fails a node task which posts no event for this long.
var nodeTaskTimeout = 10 * time.Minute

// webhookRetries are the delays before each wake delivery attempt.
var webhookRetries = []time.Duration{0, 2 * time.Second, 10 * time.Second, 30 * time.Second}

const inboxSize = 200

// Nodes hosts webhook nodes: agents which only run when woken, such as
// cloud agents. The server plays the A2A server role for them, wakes them
// with signed webhooks and turns their replies into task updates.
type Nodes struct {
	s *Server

	mu       sync.Mutex
	handlers map[string]a2asrv.RequestHandler
	open     map[a2a.TaskID]*nodeWaiter
	keys     map[a2a.TaskID]map[string]relayapi.EventResult
	inbox    map[string][]*relayapi.InboxEntry
	failed   map[string]bool
}

type nodeWaiter struct {
	node   string
	events chan nodeDelivery
	done   chan struct{}
	once   sync.Once
}

func (w *nodeWaiter) close() { w.once.Do(func() { close(w.done) }) }

type nodeDelivery struct {
	ev     relayapi.Event
	result chan error
}

func newNodes(s *Server) *Nodes {
	return &Nodes{
		s:        s,
		handlers: map[string]a2asrv.RequestHandler{},
		open:     map[a2a.TaskID]*nodeWaiter{},
		keys:     map[a2a.TaskID]map[string]relayapi.EventResult{},
		inbox:    map[string][]*relayapi.InboxEntry{},
		failed:   map[string]bool{},
	}
}

func nodeScope(id string) string { return "n/" + id }

// tasks returns the task store of a node. Tasks that were open when the
// server stopped are failed the first time it is used.
func (n *Nodes) tasks(ctx context.Context, id string) *store.TaskStore {
	ts := n.s.store.Tasks(nodeScope(id), n.s.viewer)
	n.mu.Lock()
	first := !n.failed[id]
	n.failed[id] = true
	n.mu.Unlock()
	if first {
		if _, err := ts.FailUnfinished(ctx, "The agent server restarted before the node answered."); err != nil {
			slog.Error("failing interrupted node tasks", "node", id, "error", err)
		}
	}
	return ts
}

// handler returns the A2A handler of a node.
func (n *Nodes) handler(id string) a2asrv.RequestHandler {
	n.mu.Lock()
	h, ok := n.handlers[id]
	n.mu.Unlock()
	if ok {
		return h
	}
	h = a2asrv.NewHandler(&nodeExecutor{n: n, node: id},
		a2asrv.WithTaskStore(n.tasks(context.Background(), id)),
		a2asrv.WithCallInterceptors(&nodeCallerInterceptor{}),
		a2asrv.WithCapabilityChecks(&a2a.AgentCapabilities{Streaming: true}),
		a2asrv.WithLogger(slog.Default()),
	)
	n.mu.Lock()
	n.handlers[id] = h
	n.mu.Unlock()
	return h
}

// nodeCallerInterceptor lets owners call nodes; the agent calls them in-process as an owner.
type nodeCallerInterceptor struct {
	a2asrv.PassthroughCallInterceptor
}

func (nodeCallerInterceptor) Before(ctx context.Context, callCtx *a2asrv.CallContext, _ *a2asrv.Request) (context.Context, any, error) {
	p, ok := principalOf(ctx)
	if !ok {
		return ctx, nil, a2a.ErrUnauthenticated
	}
	if !p.Has(auth.RoleOwner) {
		return ctx, nil, fmt.Errorf("%w: webhook nodes are reachable by the hosting agent and its owner", a2a.ErrUnauthorized)
	}
	callCtx.User = a2asrv.NewAuthenticatedUser(p.ID, map[string]any{"principal": p})
	return ctx, nil, nil
}

// register adds the node routes to a listener's mux.
func (n *Nodes) register(mux *http.ServeMux, kind string, base func(*http.Request) string) {
	serveNode := func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("node")
		node, err := n.s.store.Node(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusNotFound, "no such agent")
			return
		}
		rest := "/" + r.PathValue("rest")
		if r.Method == http.MethodGet && (rest == "/" || rest == a2asrv.WellKnownAgentCardPath || rest == "/.well-known/agent.json") {
			a2asrv.NewStaticAgentCardHandler(n.card(base(r), node)).ServeHTTP(w, r)
			return
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path, r2.URL.RawPath = rest, ""
		h := n.handler(id)
		inner := a2asrv.NewRESTHandler(h, a2asrv.WithTransportKeepAlive(15*time.Second))
		if r.Method == http.MethodPost && rest == "/" {
			inner = a2asrv.NewJSONRPCHandler(h, a2asrv.WithTransportKeepAlive(15*time.Second))
		}
		n.s.authenticate(kind, inner).ServeHTTP(w, r2)
	}
	mux.HandleFunc("/n/{node}", serveNode)
	mux.HandleFunc("/n/{node}/{rest...}", serveNode)
	mux.HandleFunc("GET /relay/api/node", n.nodeAPI(base, n.handleGetNode))
	mux.HandleFunc("POST /relay/api/tasks/{id}/events", n.nodeAPI(base, n.handleEvent))
	mux.HandleFunc("GET /relay/api/tasks/{id}", n.nodeAPI(base, n.handleGetTask))
	mux.HandleFunc("GET /relay/api/inbox", n.nodeAPI(base, n.handleInbox))
}

func (n *Nodes) card(baseURL string, node store.Node) *a2a.AgentCard {
	u := trimSlash(baseURL) + "/n/" + node.ID
	desc := node.Description
	if desc == "" {
		desc = node.Name + " is a webhook node hosted by " + n.s.config().Name + "."
	}
	return &a2a.AgentCard{
		Name:        node.Name,
		Description: desc,
		Version:     "1.0.0",
		SupportedInterfaces: []*a2a.AgentInterface{
			a2a.NewAgentInterface(u, a2a.TransportProtocolJSONRPC),
			a2a.NewAgentInterface(u, a2a.TransportProtocolHTTPJSON),
		},
		Capabilities:       a2a.AgentCapabilities{Streaming: true},
		DefaultInputModes:  []string{"text/plain", "application/json"},
		DefaultOutputModes: []string{"text/plain", "application/json"},
		Skills: []a2a.AgentSkill{{
			ID: "node", Name: node.Name, Tags: []string{"async"},
			Description: "Works asynchronously: a task stays submitted until the agent wakes up.",
		}},
	}
}

func (n *Nodes) info(node store.Node) NodeInfo {
	return NodeInfo{
		ID: node.ID, Name: node.Name, URL: trimSlash(n.s.bestURL()) + "/n/" + node.ID,
		Webhook: redactURL(node.Webhook), Paired: node.TokenHash != "", Created: node.Created,
	}
}

// put creates or replaces the node req.Name and mints a pairing code for it.
func (n *Nodes) put(ctx context.Context, req PairCodeRequest) (*PairCodeResponse, error) {
	if err := validateWebhook(req.Webhook); err != nil {
		return nil, err
	}
	if _, err := n.s.store.Contact(ctx, req.Name); err == nil {
		return nil, fmt.Errorf("%q is already a contact", req.Name)
	}
	node := store.Node{
		ID: req.Name, Name: req.Name, Description: req.Description, Webhook: req.Webhook, Headers: req.Headers,
		Secret: store.RandomString(40, store.Alphanumeric),
	}
	if err := n.s.store.PutNode(ctx, node); err != nil {
		return nil, err
	}
	return n.newCode(ctx, req.Name)
}

// newCode issues a pairing code for a node, replacing its token on use.
func (n *Nodes) newCode(ctx context.Context, id string) (*PairCodeResponse, error) {
	node, err := n.s.store.Node(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("no node %s", id)
	}
	code, expires, err := auth.MintCode(ctx, n.s.store, auth.PurposeNode, id, "")
	if err != nil {
		return nil, err
	}
	info := n.info(node)
	return &PairCodeResponse{
		Code: code, Expires: expires, URL: n.s.bestURL(), PairCommand: pairCommand(n.s.bestURL(), code), Node: &info,
	}, nil
}

// pair issues a new node token for the node of a taken node code, replacing
// the previous one.
func (n *Nodes) pair(w http.ResponseWriter, r *http.Request, baseURL, id string) {
	node, err := n.s.store.Node(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "the node was removed")
		return
	}
	secret := store.RandomString(40, store.Alphanumeric)
	token := relayapi.NewNodeToken(relayapi.APIBase(baseURL), node.ID, secret)
	node.TokenHash = store.Hash(token)
	if err := n.s.store.PutNode(r.Context(), node); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	slog.Info("node paired", "node", node.ID)
	writeJSON(w, http.StatusOK, PairResponse{
		Kind: PairKindNode, Token: token, Principal: "node:" + node.ID, Agent: node.Name, URL: trimSlash(baseURL) + "/n/" + node.ID,
	})
}

func (n *Nodes) wireInfo(baseURL string, node store.Node) relayapi.NodeInfo {
	return relayapi.NodeInfo{
		ID: node.ID, Name: node.Name, Description: node.Description, URL: trimSlash(baseURL) + "/n/" + node.ID,
		Webhook: redactURL(node.Webhook), Paired: node.TokenHash != "", CreatedAt: node.Created,
	}
}

// nodeAPI authenticates a node by its node token.
func (n *Nodes) nodeAPI(base func(*http.Request) string, h func(http.ResponseWriter, *http.Request, string, store.Node)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok, _ := auth.BearerToken(r.Header.Get("Authorization"))
		_, id, ok := relayapi.ParseNodeToken(tok)
		if !ok {
			writeError(w, http.StatusUnauthorized, "node token required")
			return
		}
		node, err := n.s.store.Node(r.Context(), id)
		if err != nil || node.TokenHash == "" || subtle.ConstantTimeCompare([]byte(node.TokenHash), []byte(store.Hash(tok))) != 1 {
			writeError(w, http.StatusUnauthorized, "invalid node token")
			return
		}
		h(w, r, base(r), node)
	}
}

func (n *Nodes) handleGetNode(w http.ResponseWriter, _ *http.Request, baseURL string, node store.Node) {
	writeJSON(w, http.StatusOK, n.wireInfo(baseURL, node))
}

func (n *Nodes) handleGetTask(w http.ResponseWriter, r *http.Request, _ string, node store.Node) {
	t, err := n.tasks(r.Context(), node.ID).Get(auth.WithPrincipal(r.Context(), auth.Owner()), a2a.TaskID(r.PathValue("id")))
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("no task %s for node %s", r.PathValue("id"), node.ID))
		return
	}
	writeJSON(w, http.StatusOK, t.Task)
}

func (n *Nodes) handleInbox(w http.ResponseWriter, r *http.Request, _ string, node store.Node) {
	all := r.URL.Query().Get("all") != ""
	n.mu.Lock()
	out := []relayapi.InboxEntry{}
	for _, e := range n.inbox[node.ID] {
		entry := *e
		if entry.Envelope.Kind == relayapi.KindTask || entry.Envelope.Kind == relayapi.KindMessage {
			entry.Open = n.open[a2a.TaskID(entry.Envelope.TaskID)] != nil
		}
		if all || !entry.Delivered || entry.Open {
			out = append(out, entry)
		}
	}
	n.mu.Unlock()
	writeJSON(w, http.StatusOK, relayapi.Inbox{Entries: out})
}

func (n *Nodes) handleEvent(w http.ResponseWriter, r *http.Request, _ string, node store.Node) {
	taskID := a2a.TaskID(r.PathValue("id"))
	var ev relayapi.Event
	if !readJSON(w, r, &ev) {
		return
	}
	state, ok := relayapi.ParseState(ev.State)
	if !ok {
		writeError(w, http.StatusBadRequest, "state must be one of working, input-required, auth-required, completed, failed, rejected, canceled")
		return
	}
	ev.State = string(state)
	for _, a := range ev.Artifacts {
		if a == nil || len(a.Parts) == 0 {
			writeError(w, http.StatusBadRequest, "every artifact needs at least one part")
			return
		}
	}

	n.mu.Lock()
	wt := n.open[taskID]
	if wt != nil && wt.node != node.ID {
		wt = nil
	}
	keys := n.keys[taskID]
	if ev.IdempotencyKey != "" && keys != nil {
		if res, seen := keys[ev.IdempotencyKey]; seen {
			n.mu.Unlock()
			res.Duplicate = true
			writeJSON(w, http.StatusOK, res)
			return
		}
	}
	if wt == nil {
		n.mu.Unlock()
		current := "unknown"
		if t, err := n.tasks(r.Context(), node.ID).Get(auth.WithPrincipal(r.Context(), auth.Owner()), taskID); err == nil {
			current = relayapi.StateName(t.Task.Status.State)
		}
		writeError(w, http.StatusConflict, fmt.Sprintf("task %s is %s and not waiting for an event from the node", taskID, current))
		return
	}
	res := relayapi.EventResult{TaskID: string(taskID), State: relayapi.StateName(state)}
	if ev.IdempotencyKey != "" {
		if n.keys[taskID] == nil {
			n.keys[taskID] = map[string]relayapi.EventResult{}
		}
		n.keys[taskID][ev.IdempotencyKey] = res
	}
	n.mu.Unlock()

	d := nodeDelivery{ev: ev, result: make(chan error, 1)}
	var err error
	select {
	case wt.events <- d:
		err = <-d.result
	case <-wt.done:
		err = errors.New("the task is no longer waiting for events")
	case <-r.Context().Done():
		err = r.Context().Err()
	}
	if err != nil {
		if ev.IdempotencyKey != "" {
			n.mu.Lock()
			delete(n.keys[taskID], ev.IdempotencyKey)
			n.mu.Unlock()
		}
		writeError(w, http.StatusConflict, fmt.Sprintf("task %s: %v", taskID, err))
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// nodeExecutor wakes a node and turns its events into A2A updates. An
// execution stays open until the node posts input-required or a terminal
// state, or posts nothing for nodeTaskTimeout.
type nodeExecutor struct {
	n    *Nodes
	node string
}

var _ a2asrv.AgentExecutor = (*nodeExecutor)(nil)

func (e *nodeExecutor) Execute(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		n := e.n
		kind, state := relayapi.KindMessage, a2a.TaskStateWorking
		if ec.StoredTask == nil {
			kind, state = relayapi.KindTask, a2a.TaskStateSubmitted
			if !yield(a2a.NewSubmittedTask(ec, ec.Message), nil) {
				return
			}
		} else if !yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateWorking, nil), nil) {
			return
		}

		w := n.openTask(ec.TaskID, e.node)
		defer n.closeTask(ec.TaskID, w)

		n.wake(ctx, e.node, relayapi.Envelope{
			Kind: kind, TaskID: string(ec.TaskID), ContextID: ec.ContextID, Sender: senderOf(ctx, n.s.config().Name),
			Message: ec.Message, Text: relayapi.TextOf(ec.Message), State: relayapi.StateName(state),
		})

		timer := time.NewTimer(nodeTaskTimeout)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-w.done:
				return
			case <-timer.C:
				msg := a2a.NewMessageForTask(a2a.MessageRoleAgent, ec,
					a2a.NewTextPart(fmt.Sprintf("The node did not respond within %s.", nodeTaskTimeout)))
				yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateFailed, msg), nil)
				return
			case d := <-w.events:
				finished, err := applyNodeEvent(ec, d.ev, yield)
				d.result <- err
				if finished || err != nil {
					return
				}
				timer.Reset(nodeTaskTimeout)
			}
		}
	}
}

var errTaskClosed = errors.New("the task is no longer waiting for events")

// applyNodeEvent emits the A2A events for a node event and reports whether
// the execution is over.
func applyNodeEvent(ec *a2asrv.ExecutorContext, ev relayapi.Event, yield func(a2a.Event, error) bool) (bool, error) {
	state, _ := relayapi.ParseState(ev.State)
	for _, art := range ev.Artifacts {
		if art.ID == "" {
			art.ID = a2a.NewArtifactID()
		}
		if !yield(&a2a.TaskArtifactUpdateEvent{TaskID: ec.TaskID, ContextID: ec.ContextID, Artifact: art, LastChunk: true}, nil) {
			return true, errTaskClosed
		}
	}
	parts := ev.Parts
	if ev.Text != "" {
		parts = append([]*a2a.Part{a2a.NewTextPart(ev.Text)}, parts...)
	}
	var msg *a2a.Message
	if len(parts) > 0 {
		if state == a2a.TaskStateCompleted {
			art := a2a.NewArtifactEvent(ec, parts...)
			art.Artifact.Name = "response"
			art.LastChunk = true
			if !yield(art, nil) {
				return true, errTaskClosed
			}
		} else {
			msg = a2a.NewMessageForTask(a2a.MessageRoleAgent, ec, parts...)
		}
	}
	if !yield(a2a.NewStatusUpdateEvent(ec, state, msg), nil) {
		return true, errTaskClosed
	}
	return state.Terminal() || state == a2a.TaskStateInputRequired || state == a2a.TaskStateAuthRequired, nil
}

func (e *nodeExecutor) Cancel(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		n := e.n
		n.mu.Lock()
		w := n.open[ec.TaskID]
		n.mu.Unlock()
		if w != nil {
			w.close()
		}
		n.wake(ctx, e.node, relayapi.Envelope{Kind: relayapi.KindCanceled, TaskID: string(ec.TaskID), ContextID: ec.ContextID,
			Sender: senderOf(ctx, n.s.config().Name), State: relayapi.StateName(a2a.TaskStateCanceled)})
		yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateCanceled, nil), nil)
	}
}

func senderOf(ctx context.Context, agent string) relayapi.Sender {
	p, ok := principalOf(ctx)
	if !ok {
		return relayapi.Sender{Kind: relayapi.SenderRelay}
	}
	if strings.HasPrefix(p.ID, "agent:") {
		return relayapi.Sender{Kind: relayapi.SenderAgent, Name: agent, OnBehalfOf: p.OnBehalfOf}
	}
	return relayapi.Sender{Kind: relayapi.SenderOwner, Name: p.ID}
}

func (n *Nodes) openTask(id a2a.TaskID, node string) *nodeWaiter {
	w := &nodeWaiter{node: node, events: make(chan nodeDelivery), done: make(chan struct{})}
	n.mu.Lock()
	defer n.mu.Unlock()
	if old := n.open[id]; old != nil {
		old.close()
	}
	n.open[id] = w
	return w
}

func (n *Nodes) closeTask(id a2a.TaskID, w *nodeWaiter) {
	w.close()
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.open[id] == w {
		delete(n.open, id)
	}
}

// wake records an envelope in the node's inbox and delivers it to the
// node's webhook in the background.
func (n *Nodes) wake(ctx context.Context, nodeID string, env relayapi.Envelope) *relayapi.InboxEntry {
	env.ID = "wake_" + store.RandomString(12, store.Alphanumeric)
	env.Node = nodeID
	env.CreatedAt = time.Now().UTC()
	if env.TaskID != "" && (env.Kind == relayapi.KindTask || env.Kind == relayapi.KindMessage) {
		env.ReplyURL = relayapi.APIBase(n.s.bestURL()) + "/api/tasks/" + env.TaskID + "/events"
	}
	env.Instructions = instructions(env)
	entry := &relayapi.InboxEntry{Envelope: env}

	n.mu.Lock()
	list := append(n.inbox[nodeID], entry)
	if len(list) > inboxSize {
		list = list[len(list)-inboxSize:]
	}
	n.inbox[nodeID] = list
	n.mu.Unlock()

	go n.deliver(context.WithoutCancel(ctx), entry)
	return entry
}

func (n *Nodes) deliver(ctx context.Context, entry *relayapi.InboxEntry) {
	env := entry.Envelope
	body, err := json.Marshal(env)
	if err != nil {
		slog.Error("encoding envelope", "error", err)
		return
	}
	for i, delay := range webhookRetries {
		time.Sleep(delay)
		node, err := n.s.store.Node(ctx, env.Node)
		if err != nil {
			return
		}
		err = postWebhook(ctx, node, env.ID, body)
		n.mu.Lock()
		entry.Attempts = i + 1
		if err == nil {
			entry.Delivered, entry.LastError = true, ""
		} else {
			entry.LastError = err.Error()
		}
		n.mu.Unlock()
		if err == nil {
			slog.Info("woke node", "node", node.ID, "kind", env.Kind, "task", env.TaskID)
			return
		}
		slog.Warn("webhook delivery failed", "node", node.ID, "attempt", i+1, "error", err)
	}
}

// SignatureHeader carries the HMAC-SHA256 of a wake: t=<unix time>,v1=<hex of HMAC(secret, "<t>.<body>")>.
const SignatureHeader = "X-A2A-Signature"

// Sign returns the signature header value of a wake body.
func Sign(secret string, t time.Time, body []byte) string {
	ts := strconv.FormatInt(t.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "."))
	mac.Write(body)
	return "t=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

func postWebhook(ctx context.Context, node store.Node, wakeID string, body []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, node.Webhook, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "a2a-server")
	req.Header.Set("X-A2A-Relay-Wake", wakeID)
	req.Header.Set(SignatureHeader, Sign(node.Secret, time.Now(), body))
	for k, v := range node.Headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Debug("closing webhook response", "error", err)
		}
	}()
	if resp.StatusCode/100 != 2 {
		b, err := io.ReadAll(io.LimitReader(resp.Body, 300))
		if err != nil {
			return fmt.Errorf("webhook returned %s", resp.Status)
		}
		return fmt.Errorf("webhook returned %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return nil
}

// instructions tell an LLM-driven node what a wake means and how to answer.
func instructions(env relayapi.Envelope) string {
	switch env.Kind {
	case relayapi.KindTask, relayapi.KindMessage:
		what := "New A2A task"
		if env.Kind == relayapi.KindMessage {
			what = "The caller answered your question on A2A task"
		}
		return fmt.Sprintf("%s %s from %s. "+
			"Reply with `%s reply %s --final \"<answer>\"` (add --artifact <file> to attach files). "+
			"Use `--status working --text \"<progress>\"` to report progress, `--input-required \"<question>\"` to ask the caller, "+
			"`--fail \"<reason>\"` to give up. Without the CLI, POST {\"state\":\"completed\",\"text\":\"<answer>\",\"idempotencyKey\":\"<unique>\"} "+
			"to replyUrl with the header `Authorization: Bearer <node token>`. The task fails if you post nothing for %s.",
			what, env.TaskID, env.Sender, relayapi.CLI, env.TaskID, nodeTaskTimeout)
	case relayapi.KindCanceled:
		return fmt.Sprintf("The caller canceled task %s. Stop working on it; replies are no longer accepted.", env.TaskID)
	case relayapi.KindPing:
		return "Test wake. No reply needed."
	}
	return ""
}

func validateWebhook(u string) error {
	p, err := url.Parse(u)
	if err != nil || (p.Scheme != "https" && p.Scheme != "http") || p.Host == "" {
		return fmt.Errorf("--webhook must be an http(s) URL, got %q", u)
	}
	return nil
}

// redactURL drops the query, which webhook providers often use for secrets.
func redactURL(u string) string {
	p, err := url.Parse(u)
	if err != nil {
		return ""
	}
	if p.RawQuery != "" {
		p.RawQuery = "…"
	}
	p.User = nil
	return p.String()
}
