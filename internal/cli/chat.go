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
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/chat"
	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
)

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
		Short: "Chat interactively with an agent",
		Long: "Start an interactive chat session with an agent. Chat reuses the same context ID " +
			"across turns and continues tasks that require input.\n\n" +
			"Controls:\n" +
			"  " + chat.Exit + ", " + chat.Quit + ", Ctrl-D   Exit and print the context and task IDs to resume later\n" +
			"  Ctrl-C                 Cancel an active turn (and its agent task), or exit at the prompt\n\n" +
			"Chat requires a terminal; use send to send a single message. " +
			"Use chat --context-id and --task-id to resume a conversation.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !deps.isTerminal() {
				return clierr.Usage("chat needs a terminal; use send to send a single message")
			}

			baseCtx := withServiceParams(cmd.Context(), cfg)
			ctx, cancel := context.WithTimeout(baseCtx, cfg.timeout)
			client, err := newAgentClient(ctx, cfg)
			cancel()
			if err != nil {
				return fmt.Errorf("failed to create a client: %w", err)
			}
			defer destroyClient(cfg, client)

			session := chat.NewSession(chat.Config{
				Send: func(ctx context.Context, msg *a2a.Message) error {
					return sendChatMessage(ctx, cfg, client, &flags, deps.poller, msg)
				},
				Cancel: func(ctx context.Context, id a2a.TaskID) error {
					ctx, cancel := context.WithTimeout(ctx, cfg.timeout)
					defer cancel()
					_, err := client.CancelTask(ctx, &a2a.CancelTaskRequest{ID: id, Tenant: cfg.tenant})
					return err
				},
				RenderError: func(err error) { cfg.renderError(err) },
				Notify:      deps.notifyInterrupt,
				Out:         cfg.stderr(),
			}, flags.contextID, a2a.TaskID(flags.taskID))
			cfg.handlers = append(cfg.handlers, session.Track)

			var first string
			if len(args) == 1 {
				first = args[0]
			}
			return session.Run(baseCtx, cmd.InOrStdin(), first)
		},
	}

	f := cmd.Flags()
	f.BoolVar(&flags.stream, "stream", false, "Stream responses (falls back to polling if unsupported)")
	f.StringVar(&flags.taskID, "task-id", "", "Task ID to continue an existing task")
	f.StringVar(&flags.contextID, "context-id", "", "Context ID to continue an existing conversation")
	f.DurationVar(&flags.pollInterval, "poll-interval", 2*time.Second, "Interval between GetTask requests when polling")

	return cmd
}

func sendChatMessage(ctx context.Context, cfg *globalConfig, client *a2aclient.Client, flags *chatFlags, poller pollerFunc, msg *a2a.Message) error {
	req := &a2a.SendMessageRequest{Message: msg, Tenant: cfg.tenant}
	if flags.stream {
		return sendStreaming(ctx, cfg, client, req, poller, flags.pollInterval)
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.timeout)
	defer cancel()
	result, err := client.SendMessage(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	return cfg.handleEvent(result)
}

// stdioIsTerminal reports whether both standard input and output are terminals.
func stdioIsTerminal() bool {
	return isCharDevice(os.Stdin) && isCharDevice(os.Stdout)
}

func isCharDevice(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
