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
	"io"
	"iter"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
)

type pollerFunc func(ctx context.Context, client *a2aclient.Client, req *a2a.SendMessageRequest, interval time.Duration) iter.Seq2[a2a.Event, error]

func newSendCmd(cfg *globalConfig, poller pollerFunc) *cobra.Command {
	var (
		stream          bool
		async           bool
		jsonBody        string
		taskID          string
		contextID       string
		history         int
		pollingInterval time.Duration
		parts           partsBuilder
	)

	cmd := &cobra.Command{
		Use:   "send [message]",
		Short: "Send a message to an agent",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if stream && async {
				return fmt.Errorf("--stream is incompatible with --async")
			}

			msg, err := buildMessage(args, jsonBody, &parts)
			if err != nil {
				return err
			}
			if taskID != "" {
				msg.TaskID = a2a.TaskID(taskID)
			}
			if contextID != "" {
				msg.ContextID = contextID
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), cfg.timeout)
			defer cancel()
			ctx = withServiceParams(ctx, cfg)

			client, err := newAgentClient(ctx, cfg)
			if err != nil {
				return fmt.Errorf("failed to create a client: %w", err)
			}
			defer func() {
				if err := client.Destroy(); err != nil {
					cfg.logf("failed to destroy client: %v", err)
				}
			}()

			req := &a2a.SendMessageRequest{Message: msg, Tenant: cfg.tenant}
			if async || cmd.Flags().Changed("history") {
				req.Config = &a2a.SendMessageConfig{}
				if async {
					req.Config.ReturnImmediately = true
				}
				if cmd.Flags().Changed("history") {
					req.Config.HistoryLength = &history
				}
			}

			if stream {
				if err := handleStreaming(ctx, cfg, client, req); err != nil {
					if !errors.Is(err, a2a.ErrUnsupportedOperation) {
						return err
					}
					cfg.logf("falling back to polling (%v): %v", pollingInterval, err)
					for event, err := range poller(ctx, client, req, pollingInterval) {
						if err := handleStreamEntry(cfg, event, err); err != nil {
							return err
						}
					}
				}
				return nil
			}

			result, err := client.SendMessage(ctx, req)
			if err != nil {
				return fmt.Errorf("failed to send message: %w", err)
			}
			if err := cfg.printSendResult(result); err != nil {
				return fmt.Errorf("failed to print result: %w", err)
			}
			return nil
		},
	}

	f := cmd.Flags()
	f.BoolVar(&stream, "stream", false, "Use streaming response. Falls back to polling and synthetic events if a server does not support streaming.")
	f.BoolVar(&async, "async", false, "Return immediately (fire-and-forget) instead of waiting for completion")
	f.Var(&textPartValue{&parts}, "text", "Add a text part (repeatable, order-preserving)")
	f.Var(&filePartValue{&parts}, "file", "Add a file part from a local path (inlined) or URL (by reference); repeatable")
	f.Var(&dataPartValue{&parts}, "data", "Add a JSON data part from a file, or '-' to read stdin (repeatable)")
	f.Var(&mediaTypeValue{&parts}, "media-type", "Media type for the immediately preceding part flag")
	f.StringVar(&jsonBody, "json", "", "Raw JSON Message object (mutually exclusive with part flags)")
	f.StringVar(&taskID, "task-id", "", "Task ID to continue an existing task")
	f.StringVar(&contextID, "context-id", "", "Context ID to group this turn under")
	f.IntVar(&history, "history", 0, "Request n history messages in the response")
	f.DurationVar(&pollingInterval, "polling-interval", 5*time.Second, "Duration between GetTask requests in polling fallback mode.")

	return cmd
}

func handleStreaming(ctx context.Context, cfg *globalConfig, client *a2aclient.Client, req *a2a.SendMessageRequest) error {
	if card := client.Card(); card != nil && !card.Capabilities.Streaming {
		return fmt.Errorf("streaming not listed in agent capabilities: %w", a2a.ErrUnsupportedOperation)
	}
	for event, err := range client.SendStreamingMessage(ctx, req) {
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
	if err := cfg.printEvent(event); err != nil {
		return fmt.Errorf("failed to print event: %w", err)
	}
	return nil
}

// buildMessage assembles a user message from the ordered part flags, an optional
// trailing positional text, or a raw --json Message object. --json is mutually
// exclusive with the part flags and positional text.
func buildMessage(positional []string, jsonBody string, parts *partsBuilder) (*a2a.Message, error) {
	if jsonBody != "" {
		if len(parts.specs) > 0 || len(positional) > 0 {
			return nil, fmt.Errorf("--json cannot be combined with --text/--file/--data or a positional message")
		}
		msg := new(a2a.Message)
		if err := json.Unmarshal([]byte(jsonBody), msg); err != nil {
			return nil, fmt.Errorf("parsing --json: %w", err)
		}
		if msg.ID == "" {
			msg.ID = a2a.NewMessageID()
		}
		return msg, nil
	}

	built, err := parts.build()
	if err != nil {
		return nil, err
	}
	if len(positional) > 0 {
		built = append(built, a2a.NewTextPart(strings.Join(positional, " ")))
	}
	if len(built) == 0 {
		return nil, fmt.Errorf("provide a message as text, or via --text, --file, --data, or --json")
	}
	return a2a.NewMessage(a2a.MessageRoleUser, built...), nil
}

// partKind identifies which content flag produced a part spec.
type partKind int

const (
	kindText partKind = iota
	kindFile
	kindData
)

// partSpec is a single deferred part, captured in command-line order.
type partSpec struct {
	kind      partKind
	value     string
	mediaType string
}

// partsBuilder accumulates part specs in the order their flags appear on the
// command line. pflag calls Set on each flag Value in order, so interleaved
// --text/--file/--data flags preserve their sequence, and --media-type binds to
// the part flag immediately preceding it.
type partsBuilder struct {
	specs []partSpec
}

func (b *partsBuilder) add(kind partKind, value string) {
	b.specs = append(b.specs, partSpec{kind: kind, value: value})
}

func (b *partsBuilder) setMediaType(mediaType string) error {
	if len(b.specs) == 0 {
		return fmt.Errorf("--media-type must follow a --text, --file, or --data flag")
	}
	b.specs[len(b.specs)-1].mediaType = mediaType
	return nil
}

func (b *partsBuilder) build() ([]*a2a.Part, error) {
	if len(b.specs) == 0 {
		return nil, nil
	}
	out := make([]*a2a.Part, 0, len(b.specs))
	for _, s := range b.specs {
		p, err := s.toPart()
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (s partSpec) toPart() (*a2a.Part, error) {
	switch s.kind {
	case kindText:
		p := a2a.NewTextPart(s.value)
		if s.mediaType != "" {
			p.MediaType = s.mediaType
		}
		return p, nil
	case kindFile:
		return buildFilePart(s.value, s.mediaType)
	case kindData:
		return buildDataPart(s.value, s.mediaType)
	default:
		return nil, fmt.Errorf("unknown part kind %d", s.kind)
	}
}

// buildFilePart turns a --file value into a Part: a local filesystem path is
// inlined as raw bytes (file-with-bytes), while a URL is carried by reference
// (file-with-uri) and never fetched by the CLI.
func buildFilePart(ref, mediaType string) (*a2a.Part, error) {
	if looksLikeURL(ref) {
		return a2a.NewFileURLPart(a2a.URL(ref), mediaType), nil
	}

	path := strings.TrimPrefix(ref, "file://")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading --file %q: %w", ref, err)
	}
	p := a2a.NewRawPart(data)
	p.Filename = filepath.Base(path)
	if mediaType == "" {
		mediaType = mime.TypeByExtension(filepath.Ext(path))
	}
	if mediaType != "" {
		p.MediaType = mediaType
	}
	return p, nil
}

// buildDataPart turns a --data value into a structured DataPart, reading JSON
// from a file path or from stdin when the value is "-".
func buildDataPart(ref, mediaType string) (*a2a.Part, error) {
	var (
		raw []byte
		err error
	)
	if ref == "-" {
		raw, err = io.ReadAll(os.Stdin)
	} else {
		raw, err = os.ReadFile(ref)
	}
	if err != nil {
		return nil, fmt.Errorf("reading --data %q: %w", ref, err)
	}

	var data any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("parsing --data %q as JSON: %w", ref, err)
	}
	p := a2a.NewDataPart(data)
	if mediaType != "" {
		p.MediaType = mediaType
	}
	return p, nil
}

// looksLikeURL reports whether ref should be treated as a remote reference
// (file-with-uri) rather than a local filesystem path.
func looksLikeURL(ref string) bool {
	u, err := url.Parse(ref)
	if err != nil {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "s3", "gs", "ftp", "ftps":
		return true
	}
	return false
}

// pflag.Value adapters. Each appends to the shared partsBuilder as pflag
// encounters the flag, preserving command-line order across the part flags.

type textPartValue struct{ b *partsBuilder }

func (v *textPartValue) String() string     { return "" }
func (v *textPartValue) Set(s string) error { v.b.add(kindText, s); return nil }
func (v *textPartValue) Type() string       { return "string" }

type filePartValue struct{ b *partsBuilder }

func (v *filePartValue) String() string     { return "" }
func (v *filePartValue) Set(s string) error { v.b.add(kindFile, s); return nil }
func (v *filePartValue) Type() string       { return "path|url" }

type dataPartValue struct{ b *partsBuilder }

func (v *dataPartValue) String() string     { return "" }
func (v *dataPartValue) Set(s string) error { v.b.add(kindData, s); return nil }
func (v *dataPartValue) Type() string       { return "path|-" }

type mediaTypeValue struct{ b *partsBuilder }

func (v *mediaTypeValue) String() string     { return "" }
func (v *mediaTypeValue) Set(s string) error { return v.b.setMediaType(s) }
func (v *mediaTypeValue) Type() string       { return "string" }
