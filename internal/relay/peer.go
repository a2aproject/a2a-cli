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
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
)

// handleSend sends a message from a node to a peer agent and watches the
// peer task in the background. The node gets the task ID right away and the
// result as a result wake once the task stops, so it never has to poll.
func (s *Server) handleSend(w http.ResponseWriter, r *http.Request, c apiCall) {
	var req SendRequest
	if !readJSON(w, r, &req) {
		return
	}
	parts := req.Parts
	if req.Text != "" {
		parts = append([]*a2a.Part{a2a.NewTextPart(req.Text)}, parts...)
	}
	if req.Peer == "" || len(parts) == 0 {
		writeError(w, http.StatusBadRequest, "peer and a message are required")
		return
	}
	peer := req.Peer
	if !strings.Contains(peer, "://") {
		peer = s.nodeURL(r, peer)
	}
	token := req.Token
	if token == "" && c.caller.Kind == SenderNode && strings.HasPrefix(peer, s.base(r)+"/n/") {
		// Peers on this relay identify the sender by its node token.
		token = c.token
	}
	hc := &http.Client{Transport: &BearerTransport{Token: token, Base: s.cfg.HTTPClient.Transport}}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	card, err := agentcard.NewResolver(hc).Resolve(ctx, peer)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("resolving agent card of %s: %v", peer, err))
		return
	}
	client, err := a2aclient.NewFromCard(ctx, card, a2aclient.WithJSONRPCTransport(hc), a2aclient.WithRESTTransport(hc))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	msg := a2a.NewMessage(a2a.MessageRoleUser, parts...)
	msg.ContextID = req.ContextID
	msg.TaskID = a2a.TaskID(req.TaskID)

	first := make(chan sendStart, 1)
	go s.watchPeer(c.node.ID, peer, client, card.Capabilities.Streaming, msg, first)
	select {
	case st := <-first:
		if st.err != nil {
			writeError(w, http.StatusBadGateway, fmt.Sprintf("sending to %s: %v", peer, st.err))
			return
		}
		writeJSON(w, http.StatusOK, st.resp)
	case <-time.After(20 * time.Second):
		// The peer holds the request; the result still arrives as a wake.
		writeJSON(w, http.StatusAccepted, SendResponse{Peer: peer, State: "pending"})
	}
}

type sendStart struct {
	resp SendResponse
	err  error
}

func (s *Server) watchPeer(nodeID, peer string, client *a2aclient.Client, streaming bool, msg *a2a.Message, first chan<- sendStart) {
	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.TaskTimeout)
	defer cancel()
	defer func() {
		if err := client.Destroy(); err != nil {
			slog.Debug("closing peer client", "peer", peer, "error", err)
		}
	}()

	started := false
	start := func(resp SendResponse, err error) {
		if !started {
			started = true
			first <- sendStart{resp, err}
		}
	}
	report := func(env Envelope) {
		env.Kind, env.Peer, env.Sender = KindResult, peer, Sender{Kind: SenderRelay}
		s.wake(nil, nodeID, env)
	}
	reportMessage := func(m *a2a.Message) {
		start(SendResponse{Peer: peer, ContextID: m.ContextID, Message: m}, nil)
		report(Envelope{ContextID: m.ContextID, Message: m, Text: TextOf(m), State: "message"})
	}

	var taskID a2a.TaskID
	var contextID string
	var state a2a.TaskState
	track := func(id a2a.TaskID, ctxID string, st a2a.TaskState) {
		taskID, contextID = id, ctxID
		if st != "" {
			state = st
		}
		start(SendResponse{Peer: peer, TaskID: string(taskID), ContextID: contextID, State: StateName(state)}, nil)
	}

	if streaming {
		for ev, err := range client.SendStreamingMessage(ctx, &a2a.SendMessageRequest{Message: msg}) {
			if err != nil {
				if taskID == "" {
					start(SendResponse{}, err)
					return
				}
				// Fall back to polling below.
				slog.Warn("peer stream ended", "peer", peer, "task", taskID, "error", err)
				break
			}
			switch e := ev.(type) {
			case *a2a.Message:
				reportMessage(e)
				return
			case *a2a.Task:
				track(e.ID, e.ContextID, e.Status.State)
			case *a2a.TaskStatusUpdateEvent:
				track(e.TaskID, e.ContextID, e.Status.State)
			case *a2a.TaskArtifactUpdateEvent:
				track(e.TaskID, e.ContextID, "")
			}
		}
	} else {
		res, err := client.SendMessage(ctx, &a2a.SendMessageRequest{Message: msg, Config: &a2a.SendMessageConfig{ReturnImmediately: true}})
		if err != nil {
			start(SendResponse{}, err)
			return
		}
		switch e := res.(type) {
		case *a2a.Message:
			reportMessage(e)
			return
		case *a2a.Task:
			track(e.ID, e.ContextID, e.Status.State)
		}
	}
	if taskID == "" {
		start(SendResponse{}, errors.New("the peer returned neither a task nor a message"))
		return
	}

	task, err := s.waitPeerTask(ctx, client, taskID)
	env := Envelope{TaskID: string(taskID), ContextID: contextID, State: StateName(state)}
	if task != nil {
		env.Task, env.State, env.Text = task, StateName(task.Status.State), textOfTask(task)
	}
	if err != nil {
		env.Error = err.Error()
	}
	report(env)
}

// waitPeerTask returns the peer task once it stops. The stream usually ends
// exactly then, so the first check normally returns at once.
func (s *Server) waitPeerTask(ctx context.Context, client *a2aclient.Client, id a2a.TaskID) (*a2a.Task, error) {
	interval := s.cfg.PollInterval
	var last *a2a.Task
	var lastErr error
	for {
		task, err := client.GetTask(ctx, &a2a.GetTaskRequest{ID: id})
		if err == nil {
			last, lastErr = task, nil
			if stops(task.Status.State) {
				return task, nil
			}
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			if lastErr == nil {
				lastErr = fmt.Errorf("gave up waiting for the task after %s", s.cfg.TaskTimeout)
			}
			return last, lastErr
		case <-time.After(interval):
		}
		interval = min(interval*2, 30*time.Second)
	}
}

func textOfTask(t *a2a.Task) string {
	var out []string
	for _, a := range t.Artifacts {
		if txt := textOfParts(a.Parts); txt != "" {
			out = append(out, txt)
		}
	}
	if txt := TextOf(t.Status.Message); txt != "" {
		out = append(out, txt)
	}
	return strings.Join(out, "\n\n")
}

// BearerTransport adds a bearer token to every request.
type BearerTransport struct {
	Token string
	Base  http.RoundTripper
}

// RoundTrip implements [http.RoundTripper].
func (t *BearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if t.Token != "" {
		r = r.Clone(r.Context())
		r.Header.Set("Authorization", "Bearer "+t.Token)
	}
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(r)
}
