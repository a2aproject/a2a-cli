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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

// executor plays the server role for a node: it wakes the node through its
// webhook and turns the events the node posts into A2A task updates. An
// execution stays open until the node posts input-required or a terminal
// state, or stops posting for TaskTimeout.
type executor struct {
	s    *Server
	node string
}

var _ a2asrv.AgentExecutor = (*executor)(nil)

// waiter connects an open execution to POST /api/tasks/{id}/events.
type waiter struct {
	node   string
	events chan delivery
	done   chan struct{}
	once   sync.Once
}

func (w *waiter) close() { w.once.Do(func() { close(w.done) }) }

type delivery struct {
	ev     Event
	result chan error
}

// taskInfo is what the relay remembers about a task addressed to a node,
// including after its execution ends.
type taskInfo struct {
	node    string
	user    string
	keys    map[string]EventResult
	updated time.Time
}

var errTaskClosed = errors.New("the task is no longer waiting for events")

func (e *executor) Execute(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		s := e.s
		kind, state := KindMessage, a2a.TaskStateWorking
		if ec.StoredTask == nil {
			kind, state = KindTask, a2a.TaskStateSubmitted
			if !yield(a2a.NewSubmittedTask(ec, ec.Message), nil) {
				return
			}
		} else if !yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateWorking, nil), nil) {
			// A follow-up on an input-required task: the caller answered, so the
			// task is no longer waiting for it.
			return
		}

		var user string
		if ec.User != nil {
			user = ec.User.Name
		}
		w := s.openTask(ec.TaskID, e.node, user)
		defer s.closeTask(ec.TaskID, w)

		s.wake(nil, e.node, Envelope{
			Kind: kind, TaskID: string(ec.TaskID), ContextID: ec.ContextID, Sender: senderOf(ec.User),
			Message: ec.Message, Text: TextOf(ec.Message), State: StateName(state),
		})

		timer := time.NewTimer(s.cfg.TaskTimeout)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-w.done:
				return
			case <-timer.C:
				msg := a2a.NewMessageForTask(a2a.MessageRoleAgent, ec,
					a2a.NewTextPart(fmt.Sprintf("The agent did not respond within %s.", s.cfg.TaskTimeout)))
				yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateFailed, msg), nil)
				return
			case d := <-w.events:
				finished, err := apply(ec, d.ev, yield)
				d.result <- err
				if finished || err != nil {
					return
				}
				timer.Reset(s.cfg.TaskTimeout)
			}
		}
	}
}

// apply emits the A2A events for a node event. It reports whether the
// execution is over.
func apply(ec *a2asrv.ExecutorContext, ev Event, yield func(a2a.Event, error) bool) (bool, error) {
	state, _ := ParseState(ev.State)
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
	return stops(state), nil
}

// stops reports whether a task in this state no longer waits for its agent.
func stops(s a2a.TaskState) bool {
	return s.Terminal() || s == a2a.TaskStateInputRequired || s == a2a.TaskStateAuthRequired
}

func (e *executor) Cancel(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		e.s.mu.Lock()
		w := e.s.open[ec.TaskID]
		e.s.mu.Unlock()
		if w != nil {
			w.close()
		}
		e.s.wake(nil, e.node, Envelope{Kind: KindCanceled, TaskID: string(ec.TaskID), ContextID: ec.ContextID,
			Sender: senderOf(ec.User), State: StateName(a2a.TaskStateCanceled)})
		yield(a2a.NewStatusUpdateEvent(ec, a2a.TaskStateCanceled, nil), nil)
	}
}

func senderOf(u *a2asrv.User) Sender {
	if u != nil {
		if s, ok := u.Attributes["sender"].(Sender); ok {
			return s
		}
	}
	return Sender{Kind: SenderAnonymous}
}

func (s *Server) openTask(id a2a.TaskID, nodeID, user string) *waiter {
	w := &waiter{node: nodeID, events: make(chan delivery), done: make(chan struct{})}
	s.mu.Lock()
	defer s.mu.Unlock()
	if old := s.open[id]; old != nil {
		old.close()
	}
	s.open[id] = w
	ti := s.taskInfo[id]
	if ti == nil {
		s.pruneTasksLocked()
		ti = &taskInfo{node: nodeID, user: user, keys: map[string]EventResult{}}
		s.taskInfo[id] = ti
	}
	ti.updated = time.Now()
	return w
}

func (s *Server) closeTask(id a2a.TaskID, w *waiter) {
	w.close()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.open[id] == w {
		delete(s.open, id)
	}
	if ti := s.taskInfo[id]; ti != nil {
		ti.updated = time.Now()
	}
}

// pruneTasksLocked forgets closed tasks after a week, so a long-running relay
// doesn't keep every idempotency key forever.
func (s *Server) pruneTasksLocked() {
	if len(s.taskInfo) < 1000 {
		return
	}
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	for id, ti := range s.taskInfo {
		if s.open[id] == nil && ti.updated.Before(cutoff) {
			delete(s.taskInfo, id)
		}
	}
}

func (s *Server) handleEvent(w http.ResponseWriter, r *http.Request, c apiCall) {
	taskID := a2a.TaskID(r.PathValue("id"))
	var ev Event
	if !readJSON(w, r, &ev) {
		return
	}
	state, ok := ParseState(ev.State)
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

	s.mu.Lock()
	ti := s.taskInfo[taskID]
	if ti == nil || ti.node != c.node.ID {
		s.mu.Unlock()
		writeError(w, http.StatusNotFound, fmt.Sprintf("no task %s for node %s", taskID, c.node.ID))
		return
	}
	if ev.IdempotencyKey != "" {
		if res, seen := ti.keys[ev.IdempotencyKey]; seen {
			s.mu.Unlock()
			res.Duplicate = true
			writeJSON(w, http.StatusOK, res)
			return
		}
	}
	wt := s.open[taskID]
	if wt == nil {
		user := ti.user
		s.mu.Unlock()
		current := "unknown"
		if t, err := s.tasks.Get(asTaskUser(r.Context(), user), taskID); err == nil {
			current = StateName(t.Task.Status.State)
		}
		writeError(w, http.StatusConflict, fmt.Sprintf("task %s is %s and not waiting for an event from the agent", taskID, current))
		return
	}
	res := EventResult{TaskID: string(taskID), State: StateName(state)}
	if ev.IdempotencyKey != "" {
		// Reserve the key so a concurrent retry is reported as a duplicate.
		ti.keys[ev.IdempotencyKey] = res
	}
	s.mu.Unlock()

	d := delivery{ev: ev, result: make(chan error, 1)}
	var err error
	select {
	case wt.events <- d:
		err = <-d.result
	case <-wt.done:
		err = errTaskClosed
	case <-r.Context().Done():
		err = r.Context().Err()
	}
	if err != nil {
		if ev.IdempotencyKey != "" {
			s.mu.Lock()
			delete(ti.keys, ev.IdempotencyKey)
			s.mu.Unlock()
		}
		writeError(w, http.StatusConflict, fmt.Sprintf("task %s: %v", taskID, err))
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request, c apiCall) {
	taskID := a2a.TaskID(r.PathValue("id"))
	s.mu.Lock()
	ti := s.taskInfo[taskID]
	s.mu.Unlock()
	if ti == nil || ti.node != c.node.ID {
		writeError(w, http.StatusNotFound, fmt.Sprintf("no task %s for node %s", taskID, c.node.ID))
		return
	}
	t, err := s.tasks.Get(asTaskUser(r.Context(), ti.user), taskID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, t.Task)
}

func (s *Server) handleInbox(w http.ResponseWriter, r *http.Request, c apiCall) {
	all := r.URL.Query().Get("all") != ""
	s.mu.Lock()
	out := []InboxEntry{}
	for _, e := range s.inbox[c.node.ID] {
		entry := *e
		if entry.Envelope.Kind == KindTask || entry.Envelope.Kind == KindMessage {
			entry.Open = s.open[a2a.TaskID(entry.Envelope.TaskID)] != nil
		}
		if all || !entry.Delivered || entry.Open {
			out = append(out, entry)
		}
	}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, Inbox{Entries: out})
}

// wake records an envelope in the node's inbox and delivers it to the
// node's webhook in the background. r is used for links and may be nil.
func (s *Server) wake(r *http.Request, nodeID string, env Envelope) *InboxEntry {
	env.ID = "wake_" + randomString(12)
	env.Node = nodeID
	env.CreatedAt = time.Now().UTC()
	if env.TaskID != "" && (env.Kind == KindTask || env.Kind == KindMessage) {
		env.ReplyURL = s.base(r) + "/api/tasks/" + env.TaskID + "/events"
	}
	env.Instructions = s.instructions(env)
	entry := &InboxEntry{Envelope: env}

	s.mu.Lock()
	list := append(s.inbox[nodeID], entry)
	if len(list) > inboxSize {
		list = list[len(list)-inboxSize:]
	}
	s.inbox[nodeID] = list
	s.mu.Unlock()

	go s.deliver(entry)
	return entry
}

func (s *Server) deliver(entry *InboxEntry) {
	env := entry.Envelope
	body, err := json.Marshal(env)
	if err != nil {
		slog.Error("encoding envelope", "error", err)
		return
	}
	for i, delay := range s.cfg.WebhookRetries {
		time.Sleep(delay)
		n, ok := s.store.get(env.Node)
		if !ok {
			return
		}
		err := s.postWebhook(n, env.ID, body)
		s.mu.Lock()
		entry.Attempts = i + 1
		if err == nil {
			entry.Delivered, entry.LastError = true, ""
		} else {
			entry.LastError = err.Error()
		}
		s.mu.Unlock()
		if err == nil {
			slog.Info("woke node", "node", n.ID, "kind", env.Kind, "task", env.TaskID, "wake", env.ID)
			return
		}
		slog.Warn("webhook delivery failed", "node", n.ID, "wake", env.ID, "attempt", i+1, "error", err)
	}
}

func (s *Server) postWebhook(n node, wakeID string, body []byte) error {
	if n.Webhook == "" {
		return errors.New("node has no webhook")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.Webhook, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "a2a-relay")
	req.Header.Set("X-A2A-Relay-Wake", wakeID)
	for k, v := range n.WebhookHeaders {
		req.Header.Set(k, v)
	}
	resp, err := s.cfg.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer closeBody(resp)
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("webhook returned %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return nil
}

// instructions tell an LLM-driven node what a wake means and how to answer.
func (s *Server) instructions(env Envelope) string {
	switch env.Kind {
	case KindTask, KindMessage:
		what := "New A2A task"
		if env.Kind == KindMessage {
			what = "The caller answered your question on A2A task"
		}
		return fmt.Sprintf("%s %s from %s. Check the sender before acting. "+
			"Reply with `%s reply %s --final \"<answer>\"` (add --artifact <file> to attach files). "+
			"Use `--status working --text \"<progress>\"` to report progress, `--input-required \"<question>\"` to ask the caller, "+
			"`--fail \"<reason>\"` to give up. Without the CLI, POST {\"state\":\"completed\",\"text\":\"<answer>\",\"idempotencyKey\":\"<unique>\"} "+
			"to replyUrl with the header `Authorization: Bearer <node token>`. The task fails if you post nothing for %s.",
			what, env.TaskID, env.Sender, CLI, env.TaskID, s.cfg.TaskTimeout)
	case KindResult:
		if env.Error != "" {
			return fmt.Sprintf("Sending to %s failed: %s", env.Peer, env.Error)
		}
		return fmt.Sprintf("Update on task %s you sent to %s: it is %s; text and task hold the result. "+
			"If it is input-required, answer with `%s send %s --task %s --context %s \"<answer>\"`.",
			env.TaskID, env.Peer, env.State, CLI, env.Peer, env.TaskID, env.ContextID)
	case KindCanceled:
		return fmt.Sprintf("The caller canceled task %s. Stop working on it; the relay no longer accepts replies for it.", env.TaskID)
	case KindPing:
		return "Test wake from the relay. No reply needed."
	}
	return ""
}
