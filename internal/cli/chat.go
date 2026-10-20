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

package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
)

const chatQuit = "/quit"

type chatFlags struct {
	stream       bool
	taskID       string
	contextID    string
	pollInterval time.Duration
}

func newChatCmd(cfg *globalConfig, deps deps) *cobra.Command {
	var flags chatFlags

	cmd := &cobra.Command{
		Use:   "chat [message]",
		Short: "Chat with an agent, carrying the conversation across turns",
		Long: "Chat with an agent interactively. Each reply continues the same conversation: the context id is carried " +
			"across turns, and an input-required task is continued with your next message. Type " + chatQuit + " or press " +
			"Ctrl-D to exit; the context and task ids are printed so you can resume with send, or with chat --context-id and --task-id. " +
			"Ctrl-C cancels a running turn and asks the agent to cancel its task, or exits at the prompt.\n\n" +
			"When standard input or output is not a terminal, chat sends [message] once, like send, and reads no input.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			interactive := deps.isTerminal()
			if !interactive && len(args) == 0 {
				return clierr.Usage("chat needs a terminal; pass a message to send it once, or use send")
			}

			baseCtx := withServiceParams(cmd.Context(), cfg)
			ctx, cancel := context.WithTimeout(baseCtx, cfg.timeout)
			client, err := newAgentClient(ctx, cfg)
			cancel()
			if err != nil {
				return fmt.Errorf("failed to create a client: %w", err)
			}
			defer destroyClient(cfg, client)

			session := &chatSession{
				cfg:       cfg,
				client:    client,
				flags:     &flags,
				poller:    deps.poller,
				notify:    deps.notifyInterrupt,
				contextID: flags.contextID,
				taskID:    a2a.TaskID(flags.taskID),
			}
			cfg.handlers = append(cfg.handlers, session.track)
			if !interactive {
				_, err := session.turn(baseCtx, args[0])
				return err
			}
			return session.run(baseCtx, cmd.InOrStdin(), args)
		},
	}

	f := cmd.Flags()
	f.BoolVar(&flags.stream, "stream", false, "Use streaming response. Falls back to polling and synthetic events if a server does not support streaming.")
	f.StringVar(&flags.taskID, "task-id", "", "Task ID to continue an existing task")
	f.StringVar(&flags.contextID, "context-id", "", "Context ID to group this turn under")
	f.DurationVar(&flags.pollInterval, "poll-interval", 2*time.Second, "Duration between GetTask requests in polling fallback mode.")

	return cmd
}

// chatSession carries a conversation's identifiers from one turn to the next.
type chatSession struct {
	cfg          *globalConfig
	client       *a2aclient.Client
	flags        *chatFlags
	poller       pollerFunc
	notify       func(context.Context) (context.Context, context.CancelFunc)
	contextID    string
	taskID       a2a.TaskID
	authRequired bool
	// running is the task the current turn is working on, once the agent has named it.
	running a2a.TaskID
}

func (s *chatSession) run(ctx context.Context, in io.Reader, args []string) error {
	defer s.printResume()

	if len(args) == 1 && s.reply(ctx, args[0]) {
		return nil
	}

	stop := make(chan struct{})
	defer close(stop)
	lines, scanErr := scanLines(in, stop)
	for {
		_, _ = fmt.Fprint(s.cfg.stderr(), "> ")
		line, ok, err := s.next(ctx, lines, scanErr)
		if !ok {
			_, _ = fmt.Fprintln(s.cfg.stderr())
			return err
		}
		text := strings.TrimSpace(line)
		if text == "" {
			continue
		}
		if text == chatQuit {
			return nil
		}
		if s.reply(ctx, text) {
			return nil
		}
	}
}

// scanLines reads lines on a goroutine so an interrupt is noticed while chat waits for
// input. It sends the scan error, if any, before closing lines, and quits once stop is
// closed; a read already blocked on a terminal only returns when the process exits.
func scanLines(in io.Reader, stop <-chan struct{}) (<-chan string, <-chan error) {
	lines := make(chan string)
	scanErr := make(chan error, 1)
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(in)
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
func (s *chatSession) next(ctx context.Context, lines <-chan string, scanErr <-chan error) (string, bool, error) {
	ctx, stop := s.notify(ctx)
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

// reply runs one turn and reports whether the session should end. Ctrl-C cancels the
// turn and the task it left running, and a failed turn is printed the way send prints
// its errors; either way the conversation goes on.
func (s *chatSession) reply(ctx context.Context, text string) (done bool) {
	ctx, stop := s.notify(ctx)
	defer stop()
	done, err := s.turn(ctx, text)
	switch {
	case err != nil && ctx.Err() != nil:
		s.cancelRunning(ctx)
	case err != nil:
		s.cfg.renderError(err)
	}
	return done
}

// cancelRunning asks the agent to stop the task an interrupted turn left running. The
// turn's context is already canceled, so the request gets a fresh timeout.
func (s *chatSession) cancelRunning(ctx context.Context) {
	id := s.running
	if id == "" {
		_, _ = fmt.Fprintln(s.cfg.stderr(), "Interrupted.")
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.cfg.timeout)
	defer cancel()
	if _, err := s.client.CancelTask(ctx, &a2a.CancelTaskRequest{ID: id, Tenant: s.cfg.tenant}); err != nil {
		_, _ = fmt.Fprintf(s.cfg.stderr(), "Interrupted. Could not cancel task %s: %v\n", id, err)
		return
	}
	_, _ = fmt.Fprintf(s.cfg.stderr(), "Interrupted. Canceled task %s.\n", id)
	s.taskID, s.running = "", ""
}

// turn sends one message and reports done when the conversation cannot continue.
func (s *chatSession) turn(ctx context.Context, text string) (done bool, err error) {
	s.running = s.taskID
	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart(text))
	msg.ContextID = s.contextID
	msg.TaskID = s.taskID
	req := &a2a.SendMessageRequest{Message: msg, Tenant: s.cfg.tenant}

	if s.flags.stream {
		err = sendStreaming(ctx, s.cfg, s.client, req, s.poller, s.flags.pollInterval)
	} else {
		err = s.send(ctx, req)
	}
	if err != nil {
		return false, err
	}

	if s.authRequired {
		_, _ = fmt.Fprintln(s.cfg.stderr(), "The agent needs authentication to continue this task. "+
			"Provide credentials with --auth or --svc-param, then resume it with send.")
		return true, nil
	}
	return false, nil
}

func (s *chatSession) send(ctx context.Context, req *a2a.SendMessageRequest) error {
	ctx, cancel := context.WithTimeout(ctx, s.cfg.timeout)
	defer cancel()

	result, err := s.client.SendMessage(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	return s.cfg.handleEvent(result)
}

// track follows the conversation's identifiers through every event a turn prints.
func (s *chatSession) track(event a2a.Event) error {
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

// follow keeps a task's id for the next turn only while the task waits on the user.
func (s *chatSession) follow(contextID string, taskID a2a.TaskID, state a2a.TaskState) {
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

func (s *chatSession) printResume() {
	if s.contextID != "" {
		_, _ = fmt.Fprintf(s.cfg.stderr(), "contextId: %s\n", s.contextID)
	}
	if s.taskID != "" {
		_, _ = fmt.Fprintf(s.cfg.stderr(), "taskId:    %s\n", s.taskID)
	}
}

// stdioIsTerminal reports whether both standard input and output are terminals.
func stdioIsTerminal() bool {
	return isCharDevice(os.Stdin) && isCharDevice(os.Stdout)
}

func isCharDevice(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
