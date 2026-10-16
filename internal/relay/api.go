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

// Package relay implements the A2A relay behind `a2a server relay`: a public A2A server that
// plays the server role for nodes which can't accept connections or stay
// running, such as cloud agents woken by a webhook.
//
// A caller talks standard A2A to a node's URL on the relay. The relay creates
// the task, POSTs an [Envelope] to the node's webhook and keeps the task open.
// The node, once awake, posts [Event]s for the task to the relay API with its
// node token, and the relay passes them on to the caller as A2A task updates.
package relay

import (
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

// CLI is the command a node runs to talk to the relay. Wakes and the skill
// refer to it in their instructions.
const CLI = "a2a server relay"

// Envelope kinds.
const (
	// KindTask is a new task from a caller.
	KindTask = "task"
	// KindMessage is a follow-up message on a task the node put in input-required.
	KindMessage = "message"
	// KindResult is an update on a task the node started with send: the peer
	// task reached input-required or a terminal state.
	KindResult = "result"
	// KindCanceled tells the node a caller canceled one of its tasks.
	KindCanceled = "canceled"
	// KindPing is a test wake sent by ping.
	KindPing = "ping"
)

// Envelope is the JSON body the relay POSTs to a node's webhook. Every kind
// has the same shape, so a node handles all wakes in one place.
type Envelope struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Node      string `json:"node"`
	TaskID    string `json:"taskId,omitempty"`
	ContextID string `json:"contextId,omitempty"`
	// Sender is who sent the message: a caller for task and message, the
	// relay itself for the rest.
	Sender Sender `json:"sender"`
	// Text is the text of Message (or of the peer task's result), joined, for
	// nodes that only care about text.
	Text    string       `json:"text,omitempty"`
	Message *a2a.Message `json:"message,omitempty"`
	// State is the task state: the peer task's for result, otherwise the
	// relay task's at the time of the wake.
	State string `json:"state,omitempty"`
	// Task is the peer task, for result.
	Task *a2a.Task `json:"task,omitempty"`
	// Peer is the agent URL the node sent the message to, for result.
	Peer string `json:"peer,omitempty"`
	// Error is set on result when the relay failed to reach the peer.
	Error string `json:"error,omitempty"`
	// ReplyURL is where events for TaskID go.
	ReplyURL string `json:"replyUrl,omitempty"`
	// Instructions tell an LLM-driven node how to answer the wake.
	Instructions string    `json:"instructions,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

// Sender kinds.
const (
	SenderAnonymous = "anonymous"
	SenderGrant     = "grant"
	SenderNode      = "node"
	SenderOwner     = "owner"
	SenderRelay     = "relay"
)

// Sender identifies the caller of a node.
type Sender struct {
	// Kind is anonymous, grant (a caller token issued with grant), node
	// (another node on this relay), owner (the relay admin) or relay.
	Kind string `json:"kind"`
	// Name is the grant name or the node ID.
	Name string `json:"name,omitempty"`
}

func (s Sender) String() string {
	if s.Name == "" {
		return s.Kind
	}
	return s.Kind + ":" + s.Name
}

// Event is the JSON body a node posts to POST /api/tasks/{id}/events.
type Event struct {
	// IdempotencyKey makes retries safe: an event with a key the task has
	// already seen is acknowledged without being applied again.
	IdempotencyKey string `json:"idempotencyKey,omitempty"`
	// State is working, input-required, completed, failed, rejected or
	// canceled.
	State string `json:"state"`
	// Text and Parts become the status message, except for completed, where
	// they become the "response" artifact.
	Text  string      `json:"text,omitempty"`
	Parts []*a2a.Part `json:"parts,omitempty"`
	// Artifacts are added to the task before the status update.
	Artifacts []*a2a.Artifact `json:"artifacts,omitempty"`
}

// EventResult is the response to an event.
type EventResult struct {
	TaskID    string `json:"taskId"`
	State     string `json:"state"`
	Duplicate bool   `json:"duplicate,omitempty"`
}

// CreateNodeRequest is the body of POST /api/nodes.
type CreateNodeRequest struct {
	Name           string            `json:"name,omitempty"`
	Description    string            `json:"description,omitempty"`
	Webhook        string            `json:"webhook"`
	WebhookHeaders map[string]string `json:"webhookHeaders,omitempty"`
	Public         bool              `json:"public,omitempty"`
}

// PairingResponse is returned when a node is created or a new pairing code
// is issued.
type PairingResponse struct {
	Node        NodeInfo  `json:"node"`
	PairingCode string    `json:"pairingCode"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

// PairRequest is the body of POST /api/pair.
type PairRequest struct {
	Code string `json:"code"`
}

// PairResponse carries the node token, which is shown only once.
type PairResponse struct {
	Node  NodeInfo `json:"node"`
	Token string   `json:"token"`
}

// NodeInfo describes a node to its owner.
type NodeInfo struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	URL         string    `json:"url"`
	Webhook     string    `json:"webhook"`
	Public      bool      `json:"public"`
	Paired      bool      `json:"paired"`
	Grants      []string  `json:"grants"`
	CreatedAt   time.Time `json:"createdAt"`
}

// GrantRequest is the body of POST /api/grants. A name of the form
// node:<id> allows another node on the relay and returns no token.
type GrantRequest struct {
	Name string `json:"name"`
}

// GrantResponse carries the caller token, which is shown only once.
type GrantResponse struct {
	Name  string `json:"name"`
	Token string `json:"token,omitempty"`
}

// SendRequest is the body of POST /api/send.
type SendRequest struct {
	// Peer is an agent URL or the ID of a node on this relay.
	Peer      string      `json:"peer"`
	Text      string      `json:"text,omitempty"`
	Parts     []*a2a.Part `json:"parts,omitempty"`
	ContextID string      `json:"contextId,omitempty"`
	TaskID    string      `json:"taskId,omitempty"`
	// Token is a bearer token for the peer, such as a grant it issued.
	Token string `json:"token,omitempty"`
}

// SendResponse identifies the peer task. Its result arrives as a result wake.
type SendResponse struct {
	Peer      string `json:"peer"`
	TaskID    string `json:"taskId,omitempty"`
	ContextID string `json:"contextId,omitempty"`
	State     string `json:"state,omitempty"`
	// Message is set when the peer answered with a message instead of a task.
	Message *a2a.Message `json:"message,omitempty"`
}

// InboxEntry is a wake together with its delivery status.
type InboxEntry struct {
	Envelope  Envelope `json:"envelope"`
	Delivered bool     `json:"delivered"`
	Attempts  int      `json:"attempts"`
	LastError string   `json:"lastError,omitempty"`
	// Open is true while the task waits for an event from the node.
	Open bool `json:"open,omitempty"`
}

// Inbox is the response of GET /api/inbox.
type Inbox struct {
	Entries []InboxEntry `json:"entries"`
}

// StateName returns the short lowercase name of a task state, as used in
// [Event.State], such as input-required.
func StateName(s a2a.TaskState) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(string(s), "TASK_STATE_")), "_", "-")
}

// ParseState parses a short state name, or the full TASK_STATE_ name.
func ParseState(s string) (a2a.TaskState, bool) {
	full := a2a.TaskState("TASK_STATE_" + strings.ReplaceAll(strings.ToUpper(strings.TrimPrefix(s, "TASK_STATE_")), "-", "_"))
	switch full {
	case a2a.TaskStateWorking, a2a.TaskStateInputRequired, a2a.TaskStateCompleted,
		a2a.TaskStateFailed, a2a.TaskStateRejected, a2a.TaskStateCanceled, a2a.TaskStateAuthRequired:
		return full, true
	}
	return "", false
}

// TextOf joins the text parts of a message.
func TextOf(m *a2a.Message) string {
	if m == nil {
		return ""
	}
	return textOfParts(m.Parts)
}

func textOfParts(parts []*a2a.Part) string {
	var out []string
	for _, p := range parts {
		if t := p.Text(); t != "" {
			out = append(out, t)
		}
	}
	return strings.Join(out, "\n")
}
