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

// Package a2aclaude implements an A2A AgentExecutor on top of Claude Code's
// headless mode (`claude -p --output-format stream-json`).
//
// Every A2A context maps to a Claude Code session, resumed with --resume on
// each turn. Text deltas stream into a "response" artifact and tool calls
// become working status updates.
//
// In headless mode Claude Code denies tools that aren't pre-approved and
// reports them in the result's permission_denials. The executor turns those
// into an input-required question; once approved, it resumes the session with
// the tools added to --allowedTools and asks Claude to retry.
package a2aclaude

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"os/exec"
	"slices"
	"strings"
	"sync"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"github.com/a2aproject/a2a-cli/internal/harness"
)

// Config configures an [Executor].
type Config struct {
	// Runner starts claude. Defaults to [harness.LocalRunner].
	Runner harness.Runner
	// Binary is the Claude Code executable. Defaults to "claude".
	Binary string
	// Args are extra arguments for every claude invocation.
	Args []string
	// Env is extra environment for the claude process.
	Env []string
	// Dir is the workspace claude runs in.
	Dir string
	// Log receives claude's stderr. Discarded if nil.
	Log io.Writer

	// Sessions maps A2A contexts to Claude Code sessions. Defaults to in-memory.
	Sessions harness.SessionStore
	// Questions answers permission requests. Defaults to asking the caller.
	Questions harness.Questions

	// Model is passed as --model if set.
	Model string
	// PermissionMode is passed as --permission-mode if set, for example "acceptEdits".
	PermissionMode string
	// AllowedTools are pre-approved for every session, for example "Read", "Bash(git status)".
	AllowedTools []string
}

// Executor implements [a2asrv.AgentExecutor] for Claude Code.
type Executor struct {
	cfg Config

	mu      sync.Mutex
	procs   map[a2a.TaskID]*exec.Cmd
	pending map[a2a.TaskID]*pendingApproval
	granted map[string][]string // context ID -> tools approved with "always"
}

var _ a2asrv.AgentExecutor = (*Executor)(nil)

type pendingApproval struct {
	q      harness.Question
	tools  []string // --allowedTools specifiers that approve exactly the denied calls
	names  []string // tool names, used for "always"
	stream harness.TextStream
}

// NewExecutor creates an [Executor].
func NewExecutor(cfg Config) *Executor {
	if cfg.Runner == nil {
		cfg.Runner = harness.LocalRunner{}
	}
	if cfg.Binary == "" {
		cfg.Binary = "claude"
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
	return &Executor{
		cfg:     cfg,
		procs:   map[a2a.TaskID]*exec.Cmd{},
		pending: map[a2a.TaskID]*pendingApproval{},
		granted: map[string][]string{},
	}
}

// Execute implements [a2asrv.AgentExecutor].
func (e *Executor) Execute(ctx context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		if execCtx.StoredTask == nil {
			if !yield(a2a.NewSubmittedTask(execCtx, execCtx.Message), nil) {
				return
			}
		}

		prompt := harness.MessageText(execCtx.Message)
		stream := &harness.TextStream{Name: "response"}
		var extraTools []string

		e.mu.Lock()
		p := e.pending[execCtx.TaskID]
		delete(e.pending, execCtx.TaskID)
		e.mu.Unlock()

		if p != nil {
			ans, err := harness.ParseAnswer(p.q, execCtx.Message)
			if err != nil {
				e.setPending(execCtx.TaskID, p)
				q := p.q
				q.Text = "I didn't understand the reply. " + q.Text
				yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateInputRequired, harness.InputRequiredMessage(execCtx, q)), nil)
				return
			}
			stream = &p.stream
			prompt, extraTools = e.applyAnswer(execCtx.ContextID, p, ans)
		}

		if !yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateWorking, nil), nil) {
			return
		}

		for {
			res, err := e.run(ctx, execCtx, prompt, extraTools, stream, yield)
			if err != nil {
				yield(harness.Failed(execCtx, err.Error()), nil)
				return
			}
			if res == nil { // the consumer stopped
				return
			}
			if len(res.Denials) == 0 {
				if res.IsError {
					yield(harness.Failed(execCtx, res.Result), nil)
					return
				}
				if !stream.Started() && res.Result != "" {
					if !yield(stream.Write(execCtx, res.Result), nil) {
						return
					}
				}
				yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCompleted, nil), nil)
				return
			}

			p := newPendingApproval(res.Denials, *stream)
			ans, ok, err := e.cfg.Questions.Answer(ctx, execCtx, p.q)
			if err != nil {
				yield(harness.Failed(execCtx, err.Error()), nil)
				return
			}
			if !ok {
				e.setPending(execCtx.TaskID, p)
				yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateInputRequired, harness.InputRequiredMessage(execCtx, p.q)), nil)
				return
			}
			prompt, extraTools = e.applyAnswer(execCtx.ContextID, p, ans)
			stream.Break()
		}
	}
}

func (e *Executor) setPending(id a2a.TaskID, p *pendingApproval) {
	e.mu.Lock()
	e.pending[id] = p
	e.mu.Unlock()
}

// applyAnswer returns the prompt and the tools to allow for the run which continues after an answer.
func (e *Executor) applyAnswer(contextID string, p *pendingApproval, ans harness.Answer) (string, []string) {
	if !ans.Allowed() {
		prompt := fmt.Sprintf("The user denied permission for: %s. Do not retry it; finish the task without it.", strings.Join(p.tools, ", "))
		if ans.Text != "" {
			prompt += " The user said: " + ans.Text
		}
		return prompt, nil
	}
	if slices.Contains(ans.Options, harness.OptionAlways) {
		e.mu.Lock()
		e.granted[contextID] = append(e.granted[contextID], p.names...)
		e.mu.Unlock()
	}
	return fmt.Sprintf("The user approved: %s. Retry the denied tool calls and continue.", strings.Join(p.tools, ", ")), p.tools
}

func newPendingApproval(denials []denial, stream harness.TextStream) *pendingApproval {
	p := &pendingApproval{stream: stream}
	var patterns []string
	for _, d := range denials {
		spec := d.ToolName
		if cmd, ok := d.ToolInput["command"].(string); ok && d.ToolName == "Bash" {
			spec = fmt.Sprintf("Bash(%s)", cmd)
			patterns = append(patterns, cmd)
		} else if path, ok := d.ToolInput["file_path"].(string); ok {
			patterns = append(patterns, path)
		}
		if !slices.Contains(p.tools, spec) {
			p.tools = append(p.tools, spec)
		}
		if !slices.Contains(p.names, d.ToolName) {
			p.names = append(p.names, d.ToolName)
		}
	}
	p.q = harness.Question{
		ID:       denials[0].ToolUseID,
		Kind:     harness.KindPermission,
		Tool:     strings.Join(p.names, ","),
		Patterns: patterns,
		Text:     fmt.Sprintf("Permission required: Claude wants to use %s.", strings.Join(p.tools, ", ")),
		Options:  harness.PermissionOptions,
	}
	return p
}

type denial struct {
	ToolName  string         `json:"tool_name"`
	ToolUseID string         `json:"tool_use_id"`
	ToolInput map[string]any `json:"tool_input"`
}

type result struct {
	Subtype   string   `json:"subtype"`
	IsError   bool     `json:"is_error"`
	Result    string   `json:"result"`
	SessionID string   `json:"session_id"`
	Denials   []denial `json:"permission_denials"`
}

// line is one line of claude's stream-json output.
type line struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype"`
	SessionID string `json:"session_id"`
	Event     *struct {
		Type  string `json:"type"`
		Delta *struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta"`
		ContentBlock *struct {
			Type string `json:"type"`
		} `json:"content_block"`
	} `json:"event"`
	Message *struct {
		Content []struct {
			Type      string         `json:"type"`
			Text      string         `json:"text"`
			Name      string         `json:"name"`
			Input     map[string]any `json:"input"`
			ToolUseID string         `json:"tool_use_id"`
			IsError   bool           `json:"is_error"`
			Content   any            `json:"content"`
		} `json:"content"`
	} `json:"message"`
}

// run executes one claude invocation. It returns a nil result without an
// error if the consumer stopped accepting events.
func (e *Executor) run(ctx context.Context, execCtx *a2asrv.ExecutorContext, prompt string, extraTools []string, stream *harness.TextStream, yield func(a2a.Event, error) bool) (*result, error) {
	args := []string{"-p", "--output-format", "stream-json", "--verbose", "--include-partial-messages"}
	sessionID, ok, err := e.cfg.Sessions.Get(ctx, execCtx.ContextID)
	if err != nil {
		return nil, err
	}
	if ok {
		args = append(args, "--resume", sessionID)
	}
	if e.cfg.Model != "" {
		args = append(args, "--model", e.cfg.Model)
	}
	if e.cfg.PermissionMode != "" {
		args = append(args, "--permission-mode", e.cfg.PermissionMode)
	}
	e.mu.Lock()
	tools := slices.Concat(e.cfg.AllowedTools, e.granted[execCtx.ContextID], extraTools)
	e.mu.Unlock()
	if len(tools) > 0 {
		args = append(args, "--allowedTools", strings.Join(tools, ","))
	}
	args = append(args, e.cfg.Args...)

	cmd, err := e.cfg.Runner.Command(ctx, harness.Command{Path: e.cfg.Binary, Args: args, Dir: e.cfg.Dir, Env: e.cfg.Env})
	if err != nil {
		return nil, err
	}
	cmd.Stdin = strings.NewReader(prompt)
	var stderr bytes.Buffer
	cmd.Stderr = io.MultiWriter(&stderr, e.cfg.Log)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting %s: %w", e.cfg.Binary, err)
	}
	e.mu.Lock()
	e.procs[execCtx.TaskID] = cmd
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		delete(e.procs, execCtx.TaskID)
		e.mu.Unlock()
	}()

	var res *result
	sawDelta := false
	stopped := false
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 64*1024), 32*1024*1024)
	for sc.Scan() && !stopped {
		var l line
		if json.Unmarshal(sc.Bytes(), &l) != nil {
			continue
		}
		var out []a2a.Event
		switch l.Type {
		case "system":
			if l.Subtype == "init" && l.SessionID != "" && l.SessionID != sessionID {
				if err := e.cfg.Sessions.Put(ctx, execCtx.ContextID, l.SessionID); err != nil {
					return nil, err
				}
			}
		case "stream_event":
			if l.Event == nil {
				break
			}
			if l.Event.Type == "content_block_start" && l.Event.ContentBlock != nil && l.Event.ContentBlock.Type == "text" {
				stream.Break()
			}
			if l.Event.Delta != nil && l.Event.Delta.Type == "text_delta" {
				sawDelta = true
				if evt := stream.Write(execCtx, l.Event.Delta.Text); evt != nil {
					out = append(out, evt)
				}
			}
		case "assistant":
			if l.Message == nil {
				break
			}
			for _, c := range l.Message.Content {
				switch c.Type {
				case "text":
					// Older versions don't emit partial messages.
					if !sawDelta {
						stream.Break()
						if evt := stream.Write(execCtx, c.Text); evt != nil {
							out = append(out, evt)
						}
					}
				case "tool_use":
					out = append(out, harness.Progress(execCtx, "Running "+toolSummary(c.Name, c.Input),
						map[string]any{"tool": c.Name, "status": "running", "input": c.Input}))
				}
			}
		case "user":
			if l.Message == nil {
				break
			}
			for _, c := range l.Message.Content {
				if c.Type == "tool_result" && c.IsError {
					out = append(out, harness.Progress(execCtx, fmt.Sprintf("Tool call failed: %v", c.Content),
						map[string]any{"status": "error", "toolUseId": c.ToolUseID}))
				}
			}
		case "result":
			res = new(result)
			if err := json.Unmarshal(sc.Bytes(), res); err != nil {
				return nil, fmt.Errorf("parsing result: %w", err)
			}
		}
		for _, o := range out {
			if !yield(o, nil) {
				stopped = true
				break
			}
		}
	}
	if stopped {
		kill(cmd)
		if err := cmd.Wait(); err != nil {
			slog.Debug("claude stopped", "error", err)
		}
		return nil, nil
	}
	waitErr := cmd.Wait()
	if res == nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" && waitErr != nil {
			msg = waitErr.Error()
		}
		return nil, fmt.Errorf("claude exited without a result: %s", msg)
	}
	return res, nil
}

// Cancel implements [a2asrv.AgentExecutor].
func (e *Executor) Cancel(_ context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		e.mu.Lock()
		cmd := e.procs[execCtx.TaskID]
		delete(e.pending, execCtx.TaskID)
		e.mu.Unlock()
		if cmd != nil && cmd.Process != nil {
			kill(cmd)
		}
		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCanceled, nil), nil)
	}
}

func toolSummary(name string, input map[string]any) string {
	for _, k := range []string{"command", "file_path", "path", "pattern", "url"} {
		if v, ok := input[k].(string); ok && v != "" {
			return fmt.Sprintf("%s: %s", name, v)
		}
	}
	return name
}

func kill(cmd *exec.Cmd) {
	if err := cmd.Process.Kill(); err != nil {
		slog.Debug("killing claude", "error", err)
	}
}
