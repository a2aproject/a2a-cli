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

package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"slices"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

// QuestionKind tells a permission request apart from a question the agent asks.
type QuestionKind string

const (
	// KindPermission is a request to run a tool that needs approval.
	KindPermission QuestionKind = "permission"
	// KindQuestion is a question the agent asks the user, with optional choices.
	KindQuestion QuestionKind = "question"
)

// Standard answers to a permission request.
const (
	OptionOnce   = "once"
	OptionAlways = "always"
	OptionReject = "reject"
)

// PermissionOptions are the answers offered for every permission request.
var PermissionOptions = []Option{
	{ID: OptionOnce, Label: "Allow once"},
	{ID: OptionAlways, Label: "Allow always", Description: "Allow matching requests for the rest of the conversation"},
	{ID: OptionReject, Label: "Deny"},
}

// Question is a request from the harness that needs a decision before the
// current turn can continue.
type Question struct {
	ID   string       `json:"id"`
	Kind QuestionKind `json:"kind"`
	// Tool is the tool or permission name, for example "bash" or "edit".
	Tool string `json:"tool,omitempty"`
	// Patterns are what the tool wants to access, for example a command line or a path.
	Patterns []string `json:"patterns,omitempty"`
	// Text is a human-readable summary of the request.
	Text     string   `json:"text"`
	Options  []Option `json:"options,omitempty"`
	Multiple bool     `json:"multiple,omitempty"`
}

// Option is one of the answers a [Question] offers.
type Option struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// Answer is a decision on a [Question].
type Answer struct {
	// Options are the selected option IDs.
	Options []string `json:"options,omitempty"`
	// Text is a free-form answer, used when the question allows it.
	Text string `json:"text,omitempty"`
}

// Allowed reports whether the answer approves a permission request.
func (a Answer) Allowed() bool {
	return slices.Contains(a.Options, OptionOnce) || slices.Contains(a.Options, OptionAlways)
}

// Questions decides who answers permission requests and questions.
type Questions interface {
	// Answer returns an answer and true to decide immediately, or false to
	// pause the task in input-required and let the A2A caller decide.
	Answer(ctx context.Context, execCtx *a2asrv.ExecutorContext, q Question) (Answer, bool, error)
}

// AskCaller routes every question to the A2A caller.
type AskCaller struct{}

// Answer implements [Questions].
func (AskCaller) Answer(context.Context, *a2asrv.ExecutorContext, Question) (Answer, bool, error) {
	return Answer{}, false, nil
}

// Rules answers permission requests by tool name. Allow and Deny hold glob
// patterns matched against [Question.Tool]. Anything not matched, including
// every [KindQuestion], goes to Fallback, or to the caller if it is nil.
type Rules struct {
	Allow    []string
	Deny     []string
	Fallback Questions
}

// Answer implements [Questions].
func (r Rules) Answer(ctx context.Context, execCtx *a2asrv.ExecutorContext, q Question) (Answer, bool, error) {
	if q.Kind == KindPermission {
		if matchAny(r.Deny, q.Tool) {
			return Answer{Options: []string{OptionReject}}, true, nil
		}
		if matchAny(r.Allow, q.Tool) {
			return Answer{Options: []string{OptionOnce}}, true, nil
		}
	}
	if r.Fallback != nil {
		return r.Fallback.Answer(ctx, execCtx, q)
	}
	return Answer{}, false, nil
}

func matchAny(patterns []string, s string) bool {
	for _, p := range patterns {
		if ok, _ := path.Match(p, s); ok {
			return true
		}
	}
	return false
}

// questionDataKey is the key of the data part which carries a [Question] in an
// input-required status message.
const questionDataKey = "a2aHarnessQuestion"

// InputRequiredMessage builds the status message which asks the caller to answer q.
// The text part is meant for people, the data part for programmatic clients.
func InputRequiredMessage(info a2a.TaskInfoProvider, q Question) *a2a.Message {
	var sb strings.Builder
	sb.WriteString(q.Text)
	if len(q.Options) > 0 {
		ids := make([]string, len(q.Options))
		for i, o := range q.Options {
			ids[i] = o.ID
		}
		fmt.Fprintf(&sb, "\nReply with one of: %s.", strings.Join(ids, ", "))
	}
	return a2a.NewMessageForTask(a2a.MessageRoleAgent, info,
		a2a.NewTextPart(sb.String()),
		a2a.NewDataPart(map[string]any{questionDataKey: toMap(q)}),
	)
}

// toMap converts v to plain JSON values: task stores copy events with gob,
// which only handles registered types in data parts.
func toMap(v any) map[string]any {
	var m map[string]any
	b, err := json.Marshal(v)
	if err == nil {
		err = json.Unmarshal(b, &m)
	}
	if err != nil {
		slog.Warn("converting question to JSON", "error", err)
	}
	return m
}

// ErrNoAnswer is returned by [ParseAnswer] when the message doesn't answer the question.
var ErrNoAnswer = errors.New("message does not answer the question")

// ParseAnswer extracts an answer to q from a caller's message. It accepts a
// data part shaped like [Answer] (or {"answer": "<option>"}), or text that
// names an option by ID or label. Permission requests also accept yes/no.
// A question without a matching option takes the text as a free-form answer.
func ParseAnswer(q Question, msg *a2a.Message) (Answer, error) {
	if msg == nil {
		return Answer{}, ErrNoAnswer
	}
	var text []string
	for _, p := range msg.Parts {
		if d := p.Data(); d != nil {
			if a, ok := answerFromData(d); ok {
				return a, nil
			}
		}
		if t := strings.TrimSpace(p.Text()); t != "" {
			text = append(text, t)
		}
	}
	reply := strings.Join(text, "\n")
	if reply == "" {
		return Answer{}, ErrNoAnswer
	}
	if id, ok := matchOption(q, reply); ok {
		return Answer{Options: []string{id}}, nil
	}
	if q.Kind == KindPermission {
		switch strings.ToLower(strings.Trim(reply, " .!")) {
		case "y", "yes", "ok", "approve", "approved", "allow", "lgtm":
			return Answer{Options: []string{OptionOnce}}, nil
		case "n", "no", "deny", "denied", "decline":
			return Answer{Options: []string{OptionReject}}, nil
		}
		return Answer{}, ErrNoAnswer
	}
	return Answer{Text: reply}, nil
}

func matchOption(q Question, reply string) (string, bool) {
	norm := strings.ToLower(strings.Trim(reply, " .!\"'`"))
	for _, o := range q.Options {
		if norm == strings.ToLower(o.ID) || norm == strings.ToLower(o.Label) {
			return o.ID, true
		}
	}
	return "", false
}

func answerFromData(d any) (Answer, bool) {
	raw, err := json.Marshal(d)
	if err != nil {
		return Answer{}, false
	}
	var v struct {
		Answer  string   `json:"answer"`
		Options []string `json:"options"`
		Text    string   `json:"text"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return Answer{}, false
	}
	if v.Answer != "" {
		return Answer{Options: []string{v.Answer}}, true
	}
	if len(v.Options) > 0 || v.Text != "" {
		return Answer{Options: v.Options, Text: v.Text}, true
	}
	return Answer{}, false
}

// MessageText concatenates the text of a message, rendering data parts as JSON.
func MessageText(msg *a2a.Message) string {
	if msg == nil {
		return ""
	}
	var out []string
	for _, p := range msg.Parts {
		switch {
		case p.Text() != "":
			out = append(out, p.Text())
		case p.Data() != nil:
			out = append(out, DataText(p.Data()))
		}
	}
	return strings.Join(out, "\n")
}

// Progress builds a working status update with a short text, used to report tool calls.
func Progress(info a2a.TaskInfoProvider, text string, data map[string]any) *a2a.TaskStatusUpdateEvent {
	parts := []*a2a.Part{a2a.NewTextPart(text)}
	if data != nil {
		parts = append(parts, a2a.NewDataPart(data))
	}
	return a2a.NewStatusUpdateEvent(info, a2a.TaskStateWorking, a2a.NewMessageForTask(a2a.MessageRoleAgent, info, parts...))
}

// Failed builds a failed status update carrying the error text.
func Failed(info a2a.TaskInfoProvider, text string) *a2a.TaskStatusUpdateEvent {
	return a2a.NewStatusUpdateEvent(info, a2a.TaskStateFailed,
		a2a.NewMessageForTask(a2a.MessageRoleAgent, info, a2a.NewTextPart(text)))
}

// DataText renders a data part value as JSON text for harnesses that only
// take text.
func DataText(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}
