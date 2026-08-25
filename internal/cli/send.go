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
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"time"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/flagparse"
	"github.com/a2aproject/a2a-cli/internal/utils"
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
)

type sendFlags struct {
	stream          bool
	async           bool
	payload         string
	taskID          string
	contextID       string
	history         int
	pollingInterval time.Duration
	parts           flagparse.Parts
	meta            flagparse.Metadata
}

type pollerFunc func(ctx context.Context, client *a2aclient.Client, req *a2a.SendMessageRequest, interval time.Duration) iter.Seq2[a2a.Event, error]

func newSendCmd(cfg *globalConfig, poller pollerFunc) *cobra.Command {
	var flags sendFlags

	cmd := &cobra.Command{
		Use:   "send [message]",
		Short: "Send a message to an agent",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.stream && flags.async {
				return fmt.Errorf("--stream is incompatible with --async")
			}
			baseCtx := withServiceParams(cmd.Context(), cfg)

			req, err := buildRequest(cmd, cfg, &flags, args)
			if err != nil {
				return err
			}

			ctx, cancel := context.WithTimeout(baseCtx, cfg.timeout)
			client, err := newAgentClient(ctx, cfg)
			if err != nil {
				return fmt.Errorf("failed to create a client: %w", err)
			}
			cancel()
			defer destroyClient(cfg, client)

			if flags.stream {
				ctx, debounceTimeout := utils.WithInactivityTimeout(baseCtx, cfg.timeout)

				err = handleStreaming(ctx, cfg, client, req, debounceTimeout)
				if err == nil {
					return nil
				}
				if !errors.Is(err, a2a.ErrUnsupportedOperation) {
					return utils.UnpackCause(ctx, err)
				}

				cfg.logf("falling back to polling (%v): %v", flags.pollingInterval, err)

				for event, err := range poller(ctx, client, req, flags.pollingInterval) {
					debounceTimeout()
					if err := handleStreamEntry(cfg, event, err); err != nil {
						return utils.UnpackCause(ctx, err)
					}
				}
				return nil
			}

			ctx, cancel = context.WithTimeout(baseCtx, cfg.timeout)
			defer cancel()

			result, err := client.SendMessage(ctx, req)
			if err != nil {
				return fmt.Errorf("failed to send message: %w", err)
			}
			if err := cfg.PrintSendResult(result); err != nil {
				return fmt.Errorf("failed to print result: %w", err)
			}
			return nil
		},
	}

	f := cmd.Flags()
	f.BoolVar(&flags.stream, "stream", false, "Use streaming response. Falls back to polling and synthetic events if a server does not support streaming.")
	f.BoolVar(&flags.async, "async", false, "Return immediately (fire-and-forget) instead of waiting for completion")
	f.StringVar(&flags.payload, "request-payload", "", "Full SendMessageRequest as a JSON file path or inline JSON string (mutually exclusive with the message-building flags)")
	f.StringVar(&flags.taskID, "task-id", "", "Task ID to continue an existing task")
	f.StringVar(&flags.contextID, "context-id", "", "Context ID to group this turn under")
	f.IntVar(&flags.history, "history", 0, "Request n history messages in the response")
	f.DurationVar(&flags.pollingInterval, "polling-interval", 5*time.Second, "Duration between GetTask requests in polling fallback mode.")
	flags.parts.Attach(f)
	flags.meta.Attach(f, "metadata", "Attach request metadata as a JSON object (repeatable)")

	return cmd
}

func buildRequest(cmd *cobra.Command, cfg *globalConfig, flags *sendFlags, args []string) (*a2a.SendMessageRequest, error) {
	if flags.payload != "" {
		if err := ensureNoPayloadOverrides(cmd, args); err != nil {
			return nil, err
		}
		req, err := parseRequestPayload(flags.payload)
		if err != nil {
			return nil, err
		}
		if req.Tenant == "" {
			req.Tenant = cfg.tenant
		}
		return req, nil
	}

	msg, err := buildMessage(args, flags)
	if err != nil {
		return nil, err
	}
	req := &a2a.SendMessageRequest{Message: msg, Tenant: cfg.tenant}
	flags.meta.ApplyTo(req)
	if flags.async || cmd.Flags().Changed("history") {
		req.Config = &a2a.SendMessageConfig{}
		if flags.async {
			req.Config.ReturnImmediately = true
		}
		if cmd.Flags().Changed("history") {
			req.Config.HistoryLength = &flags.history
		}
	}
	return req, nil
}

func handleStreaming(ctx context.Context, cfg *globalConfig, client *a2aclient.Client, req *a2a.SendMessageRequest, debounceTimeout utils.DebounceFunc) error {
	if card := client.Card(); card != nil && !card.Capabilities.Streaming {
		return fmt.Errorf("streaming not listed in agent capabilities: %w", a2a.ErrUnsupportedOperation)
	}
	for event, err := range client.SendStreamingMessage(ctx, req) {
		debounceTimeout()
		if err := handleStreamEntry(cfg, event, err); err != nil {
			return err
		}
	}
	return nil
}

func handleStreamEntry(cfg *globalConfig, event a2a.Event, err error) error {
	if err != nil {
		return fmt.Errorf("streaming error: %w", err)
	}
	if err := cfg.PrintEvent(event); err != nil {
		return fmt.Errorf("failed to print event: %w", err)
	}
	return nil
}

func buildMessage(positional []string, flags *sendFlags) (*a2a.Message, error) {
	parts, err := flags.parts.Parse()
	if err != nil {
		return nil, err
	}
	if len(positional) > 1 {
		return nil, fmt.Errorf("at most one positional argument is allowed, use --text-part for multi-part messages")
	}
	if len(positional) == 1 { // a2a send "check it out" --file-part <url> -> [TextPart("check it out"), FilePart("<url>")]
		parts = append([]*a2a.Part{a2a.NewTextPart(positional[0])}, parts...)
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("provide a message as text, or via --text-part, --file-part, --data-part, or --request-payload")
	}
	msg := a2a.NewMessage(a2a.MessageRoleUser, parts...)
	if flags.taskID != "" {
		msg.TaskID = a2a.TaskID(flags.taskID)
	}
	if flags.contextID != "" {
		msg.ContextID = flags.contextID
	}
	return msg, nil
}

// parseRequestPayload reads a full SendMessageRequest from ref, which is either
// a path to a JSON file or an inline JSON string, mirroring --data-part.
func parseRequestPayload(ref string) (*a2a.SendMessageRequest, error) {
	req := new(a2a.SendMessageRequest)
	if err := json.Unmarshal(flagparse.RawOrInline(ref), req); err != nil {
		return nil, fmt.Errorf("--request-payload %q is not a readable file or valid JSON: %w", ref, err)
	}
	if req.Message == nil {
		return nil, fmt.Errorf("--request-payload must include a message")
	}
	if req.Message.ID == "" {
		req.Message.ID = a2a.NewMessageID()
	}
	return req, nil
}

// ensureNoPayloadOverrides rejects flags and positional arguments that would
// conflict with a --request-payload, which already carries the whole request.
func ensureNoPayloadOverrides(cmd *cobra.Command, positional []string) error {
	if len(positional) > 0 {
		return fmt.Errorf("--request-payload cannot be combined with a positional message")
	}
	for _, name := range []string{"text-part", "file-part", "data-part", "task-id", "context-id", "metadata", "history", "async"} {
		if cmd.Flags().Changed(name) {
			return fmt.Errorf("--request-payload cannot be combined with --%s", name)
		}
	}
	return nil
}
