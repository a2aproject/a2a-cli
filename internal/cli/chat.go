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

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
)

const chatQuit = "/quit"

func newChatCmd(cfg *globalConfig, isTerminal func() bool) *cobra.Command {
	return &cobra.Command{
		Use:   "chat [message]",
		Short: "Chat with an agent, carrying the conversation across turns",
		Long: "Chat with an agent interactively. Each reply continues the same conversation: the context id is carried " +
			"across turns, and an input-required task is continued with your next message. Type " + chatQuit + " or press " +
			"Ctrl-D to exit; the context and task ids are printed so you can resume with send.\n\n" +
			"When standard input or output is not a terminal, chat sends [message] once, like send, and reads no input.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			interactive := isTerminal()
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

			session := &chatSession{cfg: cfg, client: client}
			if !interactive {
				_, err := session.turn(baseCtx, args[0])
				return err
			}
			return session.run(baseCtx, cmd.InOrStdin(), args)
		},
	}
}

// chatSession carries a conversation's identifiers from one turn to the next.
type chatSession struct {
	cfg       *globalConfig
	client    *a2aclient.Client
	contextID string
	taskID    a2a.TaskID
}

func (s *chatSession) run(ctx context.Context, in io.Reader, args []string) error {
	defer s.printResume()

	if len(args) == 1 {
		if done, err := s.turn(ctx, args[0]); err != nil || done {
			return err
		}
	}

	lines := bufio.NewScanner(in)
	for {
		_, _ = fmt.Fprint(s.cfg.stderr(), "> ")
		if !lines.Scan() {
			_, _ = fmt.Fprintln(s.cfg.stderr())
			return lines.Err()
		}
		text := strings.TrimSpace(lines.Text())
		if text == "" {
			continue
		}
		if text == chatQuit {
			return nil
		}
		if done, err := s.turn(ctx, text); err != nil || done {
			return err
		}
	}
}

// turn sends one message and reports done when the conversation cannot continue.
func (s *chatSession) turn(ctx context.Context, text string) (done bool, err error) {
	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart(text))
	msg.ContextID = s.contextID
	msg.TaskID = s.taskID

	ctx, cancel := context.WithTimeout(ctx, s.cfg.timeout)
	defer cancel()

	result, err := s.client.SendMessage(ctx, &a2a.SendMessageRequest{Message: msg, Tenant: s.cfg.tenant})
	if err != nil {
		return false, fmt.Errorf("failed to send message: %w", err)
	}
	if err := s.cfg.handleEvent(result); err != nil {
		return false, err
	}

	switch r := result.(type) {
	case *a2a.Task:
		s.contextID = r.ContextID
		s.taskID = ""
		switch r.Status.State {
		case a2a.TaskStateInputRequired:
			s.taskID = r.ID
		case a2a.TaskStateAuthRequired:
			s.taskID = r.ID
			_, _ = fmt.Fprintln(s.cfg.stderr(), "The agent needs authentication to continue this task. "+
				"Provide credentials with --auth or --svc-param, then resume it with send.")
			return true, nil
		}
	case *a2a.Message:
		if r.ContextID != "" {
			s.contextID = r.ContextID
		}
		s.taskID = ""
	}
	return false, nil
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
