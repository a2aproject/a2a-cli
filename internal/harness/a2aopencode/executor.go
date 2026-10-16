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

// Package a2aopencode implements an A2A AgentExecutor on top of the HTTP API
// of `opencode serve`.
//
// Every A2A context maps to an opencode session. Assistant text streams into
// a "response" artifact, tool calls become working status updates, and
// permission requests and questions pause the task in input-required until
// [harness.Questions] or the caller answers them.
package a2aopencode

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"github.com/a2aproject/a2a-cli/internal/harness"
)

// Config configures an [Executor].
type Config struct {
	// Runner starts `opencode serve`. Defaults to [harness.LocalRunner].
	Runner harness.Runner
	// Binary is the opencode executable. Defaults to "opencode".
	Binary string
	// Args are extra arguments for `opencode serve`.
	Args []string
	// Env is extra environment for the opencode process.
	Env []string
	// Dir is the workspace opencode runs in.
	Dir string
	// URL attaches to an already running opencode server instead of starting one.
	URL string
	// Log receives the output of the opencode process. Discarded if nil.
	Log io.Writer

	// Sessions maps A2A contexts to opencode sessions. Defaults to in-memory.
	Sessions harness.SessionStore
	// Questions answers permission requests and questions. Defaults to asking the caller.
	Questions harness.Questions

	// Model is "provider/model". Empty uses the opencode default.
	Model string
	// Agent is the opencode agent, such as "build" or "plan". Empty uses the default.
	Agent string
	// Permission rules are added to every session the executor creates, for
	// example {Permission: "bash", Pattern: "*", Action: "ask"}.
	Permission []PermissionRule
}

// Executor implements [a2asrv.AgentExecutor] for opencode.
type Executor struct {
	cfg    Config
	client *client
	cmd    *exec.Cmd
	cancel context.CancelFunc

	done    chan struct{}
	exitErr error

	mu    sync.Mutex
	subs  map[string]*queue     // opencode session ID -> events
	turns map[a2a.TaskID]*turn  // tasks with a turn in progress
	busy  map[string]a2a.TaskID // opencode session ID -> task running in it
}

var _ a2asrv.AgentExecutor = (*Executor)(nil)

// NewExecutor starts opencode (unless [Config.URL] is set), waits until it is
// healthy and subscribes to its events. Call [Executor.Close] to stop it.
func NewExecutor(ctx context.Context, cfg Config) (*Executor, error) {
	if cfg.Runner == nil {
		cfg.Runner = harness.LocalRunner{}
	}
	if cfg.Binary == "" {
		cfg.Binary = "opencode"
	}
	if cfg.Sessions == nil {
		cfg.Sessions = harness.NewMemorySessions()
	}
	if cfg.Questions == nil {
		cfg.Questions = harness.AskCaller{}
	}
	if cfg.Log == nil {
		cfg.Log = io.Discard
	}

	bgCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	e := &Executor{
		cfg:    cfg,
		cancel: cancel,
		done:   make(chan struct{}),
		subs:   map[string]*queue{},
		turns:  map[a2a.TaskID]*turn{},
		busy:   map[string]a2a.TaskID{},
	}

	baseURL := strings.TrimSuffix(cfg.URL, "/")
	if baseURL == "" {
		port, err := freePort()
		if err != nil {
			cancel()
			return nil, err
		}
		args := append([]string{"serve", "--hostname", "127.0.0.1", "--port", strconv.Itoa(port)}, cfg.Args...)
		cmd, err := cfg.Runner.Command(bgCtx, harness.Command{Path: cfg.Binary, Args: args, Dir: cfg.Dir, Env: cfg.Env})
		if err != nil {
			cancel()
			return nil, err
		}
		cmd.Stdout, cmd.Stderr = cfg.Log, cfg.Log
		if err := cmd.Start(); err != nil {
			cancel()
			return nil, fmt.Errorf("starting %s: %w", cfg.Binary, err)
		}
		e.cmd = cmd
		go func() {
			e.exitErr = cmd.Wait()
			close(e.done)
		}()
		baseURL = fmt.Sprintf("http://127.0.0.1:%d", port)
	}

	e.client = &client{baseURL: baseURL, http: &http.Client{}}
	if cfg.URL != "" {
		e.client.dir = cfg.Dir
	}
	healthCtx, healthCancel := context.WithCancel(ctx)
	defer healthCancel()
	go func() {
		select {
		case <-e.done:
			healthCancel()
		case <-healthCtx.Done():
		}
	}()
	if err := e.client.waitHealthy(healthCtx, 60*time.Second); err != nil {
		e.Close()
		if e.exitErr != nil {
			return nil, fmt.Errorf("%s exited: %w", cfg.Binary, e.exitErr)
		}
		return nil, err
	}
	go e.client.subscribe(bgCtx, e.dispatch)
	return e, nil
}

// URL returns the address of the opencode server.
func (e *Executor) URL() string { return e.client.baseURL }

// Done is closed when the opencode process the executor started exits. It is
// never closed when attached to an external server.
func (e *Executor) Done() <-chan struct{} { return e.done }

// Err returns the exit status of the opencode process after [Executor.Done] is closed.
func (e *Executor) Err() error { return e.exitErr }

// Close stops the event subscription and the opencode process if the executor started it.
func (e *Executor) Close() {
	e.cancel()
	if e.cmd != nil {
		<-e.done
	}
}

func (e *Executor) dispatch(ev event) {
	var p struct {
		SessionID string `json:"sessionID"`
	}
	if json.Unmarshal(ev.Properties, &p) != nil || p.SessionID == "" {
		return
	}
	e.mu.Lock()
	q := e.subs[p.SessionID]
	e.mu.Unlock()
	if q != nil {
		q.push(ev)
	}
}

// turn is one prompt being processed by opencode. It outlives an Execute call
// when the task pauses in input-required.
type turn struct {
	sessionID string
	events    *queue
	text      harness.TextStream

	roles     map[string]string // message ID -> role
	partTypes map[string]string // part ID -> part type
	emitted   map[string]int    // text part ID -> bytes already streamed
	lastPart  string            // text part which received text last
	toolState map[string]string // tool part ID -> last reported status
	errMsg    string

	pending *pendingQuestion
}

type pendingQuestion struct {
	q         harness.Question
	questions int // number of questions in an opencode question request
}

// Execute implements [a2asrv.AgentExecutor].
func (e *Executor) Execute(ctx context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		if execCtx.StoredTask == nil {
			if !yield(a2a.NewSubmittedTask(execCtx, execCtx.Message), nil) {
				return
			}
		}

		e.mu.Lock()
		t := e.turns[execCtx.TaskID]
		e.mu.Unlock()

		if t != nil && t.pending != nil {
			if done := e.answerPending(ctx, execCtx, t, yield); done {
				return
			}
		} else {
			var err error
			if t, err = e.startTurn(ctx, execCtx); err != nil {
				yield(harness.Failed(execCtx, err.Error()), nil)
				return
			}
		}

		if !yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateWorking, nil), nil) {
			return
		}
		e.pump(ctx, execCtx, t, yield)
	}
}

// answerPending forwards the caller's reply to the pending opencode request.
// It returns true if Execute should stop, for example because the reply
// didn't answer the question and the task stays in input-required.
func (e *Executor) answerPending(ctx context.Context, execCtx *a2asrv.ExecutorContext, t *turn, yield func(a2a.Event, error) bool) bool {
	p := t.pending
	ans, err := harness.ParseAnswer(p.q, execCtx.Message)
	if err != nil {
		q := p.q
		q.Text = "I didn't understand the reply. " + q.Text
		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateInputRequired, harness.InputRequiredMessage(execCtx, q)), nil)
		return true
	}
	if err := e.reply(ctx, p, ans); err != nil {
		yield(harness.Failed(execCtx, fmt.Sprintf("forwarding answer to opencode: %v", err)), nil)
		e.endTurn(execCtx.TaskID, t)
		return true
	}
	t.pending = nil
	return false
}

func (e *Executor) reply(ctx context.Context, p *pendingQuestion, ans harness.Answer) error {
	if p.q.Kind == harness.KindPermission {
		reply := harness.OptionReject
		if len(ans.Options) > 0 {
			reply = ans.Options[0]
		}
		return e.client.replyPermission(ctx, p.q.ID, reply, ans.Text)
	}
	if len(ans.Options) == 0 && ans.Text == "" {
		return e.client.rejectQuestion(ctx, p.q.ID)
	}
	answers := make([][]string, p.questions)
	switch {
	case len(ans.Options) > 0:
		labels := make([]string, 0, len(ans.Options))
		for _, id := range ans.Options {
			labels = append(labels, optionLabel(p.q, id))
		}
		answers[0] = labels
	case p.questions > 1:
		// One answer per line when several questions were asked at once.
		lines := strings.Split(ans.Text, "\n")
		for i := range answers {
			if i < len(lines) {
				answers[i] = []string{strings.TrimSpace(lines[i])}
			}
		}
	default:
		answers[0] = []string{ans.Text}
	}
	return e.client.replyQuestion(ctx, p.q.ID, answers)
}

func optionLabel(q harness.Question, id string) string {
	for _, o := range q.Options {
		if o.ID == id {
			return o.Label
		}
	}
	return id
}

func (e *Executor) startTurn(ctx context.Context, execCtx *a2asrv.ExecutorContext) (*turn, error) {
	sessionID, err := e.session(ctx, execCtx)
	if err != nil {
		return nil, err
	}

	t := &turn{
		sessionID: sessionID,
		events:    newQueue(),
		text:      harness.TextStream{Name: "response"},
		roles:     map[string]string{},
		partTypes: map[string]string{},
		emitted:   map[string]int{},
		toolState: map[string]string{},
	}

	e.mu.Lock()
	if other, ok := e.busy[sessionID]; ok {
		e.mu.Unlock()
		return nil, fmt.Errorf("context %s is busy with task %s, wait for it to finish or cancel it", execCtx.ContextID, other)
	}
	e.busy[sessionID] = execCtx.TaskID
	e.subs[sessionID] = t.events
	e.turns[execCtx.TaskID] = t
	e.mu.Unlock()

	req := promptRequest{Parts: toOpencodeParts(execCtx.Message), Agent: e.cfg.Agent}
	if provider, model, ok := strings.Cut(e.cfg.Model, "/"); ok {
		req.Model = &modelRef{ProviderID: provider, ModelID: model}
	}
	if err := e.client.promptAsync(ctx, sessionID, req); err != nil {
		e.endTurn(execCtx.TaskID, t)
		return nil, fmt.Errorf("sending prompt to opencode: %w", err)
	}
	return t, nil
}

// session returns the opencode session of the A2A context, creating it on first use.
func (e *Executor) session(ctx context.Context, execCtx *a2asrv.ExecutorContext) (string, error) {
	id, ok, err := e.cfg.Sessions.Get(ctx, execCtx.ContextID)
	if err != nil {
		return "", err
	}
	if ok && e.client.sessionExists(ctx, id) {
		return id, nil
	}
	id, err = e.client.createSession(ctx, "a2a "+execCtx.ContextID, e.cfg.Permission)
	if err != nil {
		return "", fmt.Errorf("creating opencode session: %w", err)
	}
	if err := e.cfg.Sessions.Put(ctx, execCtx.ContextID, id); err != nil {
		return "", err
	}
	return id, nil
}

func (e *Executor) endTurn(taskID a2a.TaskID, t *turn) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.turns[taskID] == t {
		delete(e.turns, taskID)
	}
	if e.subs[t.sessionID] == t.events {
		delete(e.subs, t.sessionID)
	}
	if e.busy[t.sessionID] == taskID {
		delete(e.busy, t.sessionID)
	}
}

// pump translates opencode events to A2A events until the turn finishes or
// pauses for input.
func (e *Executor) pump(ctx context.Context, execCtx *a2asrv.ExecutorContext, t *turn, yield func(a2a.Event, error) bool) {
	for {
		ev, err := t.events.pop(ctx)
		if err != nil {
			// The execution was canceled by the server: stop the harness too.
			e.abort(context.WithoutCancel(ctx), t.sessionID)
			e.endTurn(execCtx.TaskID, t)
			return
		}

		var out []a2a.Event
		var stop bool
		switch ev.Type {
		case "message.updated":
			t.onMessage(ev)
		case "message.part.updated":
			out = t.onPart(execCtx, ev)
		case "message.part.delta":
			out = t.onDelta(execCtx, ev)
		case "session.error":
			t.onError(ev)
		case "permission.asked", "question.asked":
			q, n, err := toQuestion(ev)
			if err != nil {
				slog.Warn("unparsable opencode request", "type", ev.Type, "error", err)
				continue
			}
			ans, ok, err := e.cfg.Questions.Answer(ctx, execCtx, q)
			if err != nil {
				out, stop = []a2a.Event{harness.Failed(execCtx, err.Error())}, true
				e.abort(ctx, t.sessionID)
				break
			}
			p := &pendingQuestion{q: q, questions: n}
			if ok {
				if err := e.reply(ctx, p, ans); err != nil {
					out, stop = []a2a.Event{harness.Failed(execCtx, err.Error())}, true
				}
				break
			}
			t.pending = p
			// The turn stays registered: the next message on this task answers the question.
			yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateInputRequired, harness.InputRequiredMessage(execCtx, q)), nil)
			return
		case "session.idle":
			stop = true
			if t.errMsg != "" {
				out = append(out, harness.Failed(execCtx, t.errMsg))
			} else {
				out = append(out, a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCompleted, nil))
			}
		}

		for _, o := range out {
			if !yield(o, nil) {
				e.abort(context.WithoutCancel(ctx), t.sessionID)
				e.endTurn(execCtx.TaskID, t)
				return
			}
		}
		if stop {
			e.endTurn(execCtx.TaskID, t)
			return
		}
	}
}

// Cancel implements [a2asrv.AgentExecutor].
func (e *Executor) Cancel(ctx context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		e.mu.Lock()
		t := e.turns[execCtx.TaskID]
		e.mu.Unlock()
		if t != nil {
			if err := e.client.abort(ctx, t.sessionID); err != nil {
				yield(nil, fmt.Errorf("aborting opencode session: %w", err))
				return
			}
			e.endTurn(execCtx.TaskID, t)
		}
		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCanceled, nil), nil)
	}
}

func (t *turn) onMessage(ev event) {
	var p struct {
		Info struct {
			ID    string `json:"id"`
			Role  string `json:"role"`
			Error *struct {
				Name string `json:"name"`
				Data struct {
					Message string `json:"message"`
				} `json:"data"`
			} `json:"error"`
		} `json:"info"`
	}
	if json.Unmarshal(ev.Properties, &p) != nil {
		return
	}
	t.roles[p.Info.ID] = p.Info.Role
	if p.Info.Error != nil && p.Info.Error.Name != "MessageAbortedError" {
		t.errMsg = errorText(p.Info.Error.Name, p.Info.Error.Data.Message)
	}
}

func (t *turn) onError(ev event) {
	var p struct {
		Error *struct {
			Name string `json:"name"`
			Data struct {
				Message string `json:"message"`
			} `json:"data"`
		} `json:"error"`
	}
	if json.Unmarshal(ev.Properties, &p) == nil && p.Error != nil && p.Error.Name != "MessageAbortedError" {
		t.errMsg = errorText(p.Error.Name, p.Error.Data.Message)
	}
}

func errorText(name, msg string) string {
	if msg == "" {
		return name
	}
	return msg
}

type part struct {
	ID        string `json:"id"`
	MessageID string `json:"messageID"`
	Type      string `json:"type"`
	Text      string `json:"text"`
	Synthetic bool   `json:"synthetic"`
	Tool      string `json:"tool"`
	State     struct {
		Status string         `json:"status"`
		Title  string         `json:"title"`
		Input  map[string]any `json:"input"`
		Error  string         `json:"error"`
	} `json:"state"`
}

func (t *turn) onPart(info a2a.TaskInfoProvider, ev event) []a2a.Event {
	var p struct {
		Part part `json:"part"`
	}
	if json.Unmarshal(ev.Properties, &p) != nil {
		return nil
	}
	pt := p.Part
	t.partTypes[pt.ID] = pt.Type
	if t.roles[pt.MessageID] != "assistant" {
		return nil
	}
	switch pt.Type {
	case "text":
		if pt.Synthetic || len(pt.Text) <= t.emitted[pt.ID] {
			return nil
		}
		// Deltas may have been missed; catch up with the full text of the part.
		return t.emitText(info, pt.ID, pt.Text[t.emitted[pt.ID]:])
	case "tool":
		return t.onTool(info, pt)
	}
	return nil
}

func (t *turn) onDelta(info a2a.TaskInfoProvider, ev event) []a2a.Event {
	var p struct {
		MessageID string `json:"messageID"`
		PartID    string `json:"partID"`
		Field     string `json:"field"`
		Delta     string `json:"delta"`
	}
	if json.Unmarshal(ev.Properties, &p) != nil || p.Field != "text" {
		return nil
	}
	if t.roles[p.MessageID] != "assistant" || t.partTypes[p.PartID] != "text" {
		return nil
	}
	return t.emitText(info, p.PartID, p.Delta)
}

func (t *turn) emitText(info a2a.TaskInfoProvider, partID, text string) []a2a.Event {
	if t.lastPart != "" && t.lastPart != partID {
		t.text.Break()
	}
	t.lastPart = partID
	t.emitted[partID] += len(text)
	if evt := t.text.Write(info, text); evt != nil {
		return []a2a.Event{evt}
	}
	return nil
}

func (t *turn) onTool(info a2a.TaskInfoProvider, pt part) []a2a.Event {
	status := pt.State.Status
	if status == "pending" || t.toolState[pt.ID] == status {
		return nil
	}
	t.toolState[pt.ID] = status
	data := map[string]any{"tool": pt.Tool, "status": status, "input": pt.State.Input}
	summary := toolSummary(pt)
	switch status {
	case "running":
		return []a2a.Event{harness.Progress(info, "Running "+summary, data)}
	case "completed":
		return []a2a.Event{harness.Progress(info, "Finished "+summary, data)}
	case "error":
		data["error"] = pt.State.Error
		return []a2a.Event{harness.Progress(info, fmt.Sprintf("Failed %s: %s", summary, pt.State.Error), data)}
	}
	return nil
}

func toolSummary(pt part) string {
	for _, k := range []string{"command", "filePath", "path", "pattern", "url"} {
		if v, ok := pt.State.Input[k].(string); ok && v != "" {
			return fmt.Sprintf("%s: %s", pt.Tool, v)
		}
	}
	if pt.State.Title != "" {
		return fmt.Sprintf("%s: %s", pt.Tool, pt.State.Title)
	}
	return pt.Tool
}

// toQuestion converts a permission.asked or question.asked event. For
// question requests it also returns the number of questions asked.
func toQuestion(ev event) (harness.Question, int, error) {
	if ev.Type == "permission.asked" {
		var p struct {
			ID         string   `json:"id"`
			Permission string   `json:"permission"`
			Patterns   []string `json:"patterns"`
		}
		if err := json.Unmarshal(ev.Properties, &p); err != nil {
			return harness.Question{}, 0, err
		}
		text := fmt.Sprintf("Permission required: %s", p.Permission)
		if len(p.Patterns) > 0 {
			text += " for " + strings.Join(p.Patterns, "; ")
		}
		return harness.Question{
			ID: p.ID, Kind: harness.KindPermission, Tool: p.Permission, Patterns: p.Patterns,
			Text: text + ".", Options: harness.PermissionOptions,
		}, 0, nil
	}

	var p struct {
		ID        string `json:"id"`
		Questions []struct {
			Question string `json:"question"`
			Header   string `json:"header"`
			Multiple bool   `json:"multiple"`
			Options  []struct {
				Label       string `json:"label"`
				Description string `json:"description"`
			} `json:"options"`
		} `json:"questions"`
	}
	if err := json.Unmarshal(ev.Properties, &p); err != nil {
		return harness.Question{}, 0, err
	}
	if len(p.Questions) == 0 {
		return harness.Question{}, 0, errors.New("question request without questions")
	}
	q := harness.Question{ID: p.ID, Kind: harness.KindQuestion}
	var texts []string
	for _, item := range p.Questions {
		texts = append(texts, item.Question)
	}
	q.Text = strings.Join(texts, "\n")
	if len(p.Questions) == 1 {
		q.Multiple = p.Questions[0].Multiple
		for _, o := range p.Questions[0].Options {
			q.Options = append(q.Options, harness.Option{ID: o.Label, Label: o.Label, Description: o.Description})
		}
	} else {
		q.Text += "\nAnswer each question on its own line."
	}
	return q, len(p.Questions), nil
}

func toOpencodeParts(msg *a2a.Message) []map[string]any {
	var out []map[string]any
	for _, p := range msg.Parts {
		switch c := p.Content.(type) {
		case a2a.Text:
			out = append(out, map[string]any{"type": "text", "text": string(c)})
		case a2a.Data:
			out = append(out, map[string]any{"type": "text", "text": harness.DataText(c.Value)})
		case a2a.Raw:
			mime := p.MediaType
			if mime == "" {
				mime = "application/octet-stream"
			}
			out = append(out, map[string]any{
				"type": "file", "mime": mime, "filename": p.Filename,
				"url": "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(c),
			})
		case a2a.URL:
			out = append(out, map[string]any{"type": "file", "mime": p.MediaType, "filename": p.Filename, "url": string(c)})
		}
	}
	return out
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		return 0, err
	}
	return port, nil
}

// abort stops the running turn of a session. It is best effort: the turn
// may already be over.
func (e *Executor) abort(ctx context.Context, sessionID string) {
	if err := e.client.abort(ctx, sessionID); err != nil {
		slog.Debug("aborting opencode session", "session", sessionID, "error", err)
	}
}
