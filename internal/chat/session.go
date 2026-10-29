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

// Package chat runs a conversation with an agent, carrying its identifiers from one turn to the next.
package chat

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

// Inputs that end a session.
const (
	Quit = "/quit"
	Exit = "/exit"
)

// maxLineSize bounds a single line of input, so a large paste does not end the session.
const maxLineSize = 1 << 20

// NotifyFunc returns a context canceled when the user interrupts the process.
type NotifyFunc func(context.Context) (context.Context, context.CancelFunc)

// Config is how a Session talks to the agent and the user.
type Config struct {
	// Send sends one message and must pass every event it receives to Session.Track.
	Send func(ctx context.Context, msg *a2a.Message) error
	// Cancel asks the agent to cancel a task.
	Cancel func(ctx context.Context, id a2a.TaskID) error
	// RenderError reports a failed turn to the user.
	RenderError func(error)
	Notify      NotifyFunc
	// Out receives the prompt and session messages.
	Out io.Writer
}

// Session is a conversation with one agent.
type Session struct {
	cfg          Config
	contextID    string
	taskID       a2a.TaskID
	authRequired bool
	// running is the task the current turn is working on, once the agent has named it.
	running a2a.TaskID
}

// NewSession starts a conversation, optionally in an existing context or task.
func NewSession(cfg Config, contextID string, taskID a2a.TaskID) *Session {
	return &Session{cfg: cfg, contextID: contextID, taskID: taskID}
}

// Run reads messages from in until Quit or Exit, the end of input, an interrupt at the prompt,
// or a task that needs authentication. A non-empty first is sent before reading input.
func (s *Session) Run(ctx context.Context, in io.Reader, first string) error {
	defer s.printResume()

	if first != "" && s.reply(ctx, first) {
		return nil
	}

	stop := make(chan struct{})
	defer close(stop)
	lines, scanErr := scanLines(in, stop)
	for {
		s.printf("> ")
		line, ok, err := s.next(ctx, lines, scanErr)
		if !ok {
			s.printf("\n")
			return err
		}
		text := strings.TrimSpace(line)
		if text == "" {
			continue
		}
		if text == Quit || text == Exit {
			return nil
		}
		if s.reply(ctx, text) {
			return nil
		}
	}
}

// Track follows the conversation's identifiers through the events of a turn.
func (s *Session) Track(event a2a.Event) error {
	switch e := event.(type) {
	case *a2a.Task:
		s.follow(e.ContextID, e.ID, e.Status.State)
	case *a2a.TaskStatusUpdateEvent:
		s.follow(e.ContextID, e.TaskID, e.Status.State)
	case *a2a.Message:
		if e.ContextID != "" {
			s.contextID = e.ContextID
		}
		s.taskID, s.running, s.authRequired = "", "", false
	}
	return nil
}

// scanLines reads lines on a goroutine so an interrupt is noticed while waiting for
// input. It sends the scan error, if any, before closing lines, and quits once stop is
// closed; a read already blocked on a terminal only returns when the process exits.
func scanLines(in io.Reader, stop <-chan struct{}) (<-chan string, <-chan error) {
	lines := make(chan string)
	scanErr := make(chan error, 1)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(in)
		scanner.Buffer(nil, maxLineSize)
		for scanner.Scan() {
			select {
			case lines <- scanner.Text():
			case <-stop:
				return
			}
		}
		scanErr <- scanner.Err()
	}()
	return lines, scanErr
}

// next waits for a line of input. It reports !ok at the end of input, or when
// interrupted at the prompt.
func (s *Session) next(ctx context.Context, lines <-chan string, scanErr <-chan error) (string, bool, error) {
	ctx, stop := s.cfg.Notify(ctx)
	defer stop()
	select {
	case line, ok := <-lines:
		if !ok {
			return "", false, <-scanErr
		}
		return line, true, nil
	case <-ctx.Done():
		return "", false, nil
	}
}

// reply runs one turn and reports whether the session should end. A turn that is
// interrupted or times out cancels the task it left running, and a failed turn is
// reported; either way the conversation goes on.
func (s *Session) reply(ctx context.Context, text string) (done bool) {
	turnCtx, stop := s.cfg.Notify(ctx)
	defer stop()
	done, err := s.turn(turnCtx, text)
	switch {
	case err == nil:
	case turnCtx.Err() != nil:
		s.printf("Interrupted.\n")
		s.cancelRunning(ctx)
	case errors.Is(err, context.DeadlineExceeded):
		s.cfg.RenderError(err)
		s.cancelRunning(ctx)
	default:
		s.cfg.RenderError(err)
	}
	return done
}

// cancelRunning asks the agent to stop the task an abandoned turn left running. The
// request gets its own interrupt scope, so another interrupt abandons it.
func (s *Session) cancelRunning(ctx context.Context) {
	id := s.running
	if id == "" {
		return
	}
	ctx, stop := s.cfg.Notify(ctx)
	defer stop()
	if err := s.cfg.Cancel(ctx, id); err != nil {
		s.printf("Could not cancel task %s: %v\n", id, err)
		return
	}
	s.printf("Canceled task %s.\n", id)
	s.taskID, s.running = "", ""
}

// turn sends one message and reports done when the conversation cannot continue.
func (s *Session) turn(ctx context.Context, text string) (done bool, err error) {
	s.running = s.taskID
	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart(text))
	msg.ContextID = s.contextID
	msg.TaskID = s.taskID
	if err := s.cfg.Send(ctx, msg); err != nil {
		return false, err
	}
	if s.authRequired {
		s.printf("The agent needs authentication to continue this task. " +
			"Provide credentials with --auth or --svc-param, then resume it with send.\n")
		return true, nil
	}
	return false, nil
}

// follow keeps a task's id for the next turn only while the task waits on the user.
func (s *Session) follow(contextID string, taskID a2a.TaskID, state a2a.TaskState) {
	s.contextID = contextID
	s.taskID, s.authRequired = "", false
	s.running = ""
	if !state.Terminal() {
		s.running = taskID
	}
	switch state {
	case a2a.TaskStateInputRequired:
		s.taskID = taskID
	case a2a.TaskStateAuthRequired:
		s.taskID, s.authRequired = taskID, true
	}
}

func (s *Session) printResume() {
	if s.contextID != "" {
		s.printf("contextId: %s\n", s.contextID)
	}
	if s.taskID != "" {
		s.printf("taskId:    %s\n", s.taskID)
	}
}

// printf writes a session message. A failed write to the user's terminal leaves
// nothing better to report it to, so it is dropped.
func (s *Session) printf(format string, args ...any) {
	_, _ = fmt.Fprintf(s.cfg.Out, format, args...)
}
