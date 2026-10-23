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

// Package channel defines the messengers an agent server is reachable from.
// Implementations live in subpackages; the server's router turns their
// messages into A2A tasks.
package channel

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Channel is one messenger account of the agent, such as its Telegram bot.
type Channel interface {
	// Name is the channel name, such as telegram.
	Name() string
	// Caps reports what the channel supports.
	Caps() Caps
	// Run holds an outbound connection to the messenger and delivers
	// messages to in until ctx ends.
	Run(ctx context.Context, in chan<- Inbound) error
	// Send sends a message and returns a reference to it.
	Send(ctx context.Context, m Outbound) (Ref, error)
	// Edit replaces a sent message. Only called if Caps().Edit.
	Edit(ctx context.Context, ref Ref, m Outbound) error
}

// Describer is implemented by channels which can tell the user how to reach
// them and how to link an identity with a pairing code.
type Describer interface {
	// Describe returns the account, such as @my_coder_bot.
	Describe() string
	// LinkHint tells a person how to link themselves with code.
	LinkHint(code string) string
}

// Caps are what a channel supports.
type Caps struct {
	Edit    bool
	Buttons bool
	Threads bool
	Typing  bool
	// MaxText is the longest text of one message; longer text is split.
	MaxText  int
	Markdown bool
	// Stream is true when output should be streamed by editing a message,
	// false when only questions and the final result are sent.
	Stream bool
}

// ConvKey identifies a conversation within a channel: a chat, a thread or
// the root of an email thread.
type ConvKey string

// File is a file attached to a message.
type File struct {
	Name      string
	MediaType string
	Data      []byte
}

// Action is a click on a question's button.
type Action struct {
	QuestionID string
	Option     int
}

// Inbound is a message from a channel.
type Inbound struct {
	Channel string
	// MessageID is the dedup key; it becomes the A2A messageId.
	MessageID string
	// Identity is tg:<user>, dc:<user> or mail:<addr>.
	Identity string
	// Name is the sender's display name.
	Name string
	Conv ConvKey
	// Direct is true for a one-to-one chat with the sender.
	Direct bool
	Text   string
	Files  []File
	Action *Action
	// Authentic is true when the channel verified the sender: always for
	// bot APIs, after DMARC or DKIM for email.
	Authentic bool
	// ReplyTo is the message this one answers, if the channel tracks it.
	ReplyTo Ref
}

// Outbound is a message to a channel.
type Outbound struct {
	Conv  ConvKey
	Text  string
	Files []File
	// Question adds one button per option, if the channel has buttons.
	Question *Question
	// ReplyTo threads the message under another one, if supported.
	ReplyTo Ref
	// Subject is used by channels with subjects, such as email.
	Subject string
}

// Question is a decision offered with buttons.
type Question struct {
	ID      string
	Options []string
}

// Ref identifies a sent message, for edits and replies.
type Ref struct {
	Conv ConvKey
	ID   string
}

// ActionData encodes a button payload: h:<question id>:<option index>.
// It must stay within Telegram's 64-byte callback data limit.
func ActionData(questionID string, option int) string {
	return "h:" + questionID + ":" + strconv.Itoa(option)
}

// ParseActionData decodes a button payload made by [ActionData].
func ParseActionData(s string) (*Action, error) {
	rest, ok := strings.CutPrefix(s, "h:")
	if !ok {
		return nil, fmt.Errorf("not a question action: %q", s)
	}
	id, idx, ok := strings.Cut(rest, ":")
	n, err := strconv.Atoi(idx)
	if !ok || err != nil || id == "" || n < 0 {
		return nil, fmt.Errorf("malformed question action: %q", s)
	}
	return &Action{QuestionID: id, Option: n}, nil
}

// Split cuts text into pieces of at most limit bytes, preferring line breaks.
func Split(text string, limit int) []string {
	if limit <= 0 || len(text) <= limit {
		return []string{text}
	}
	var out []string
	for len(text) > limit {
		cut := strings.LastIndex(text[:limit], "\n")
		if cut < limit/2 {
			cut = limit
			for cut > 0 && !isRuneStart(text[cut]) {
				cut--
			}
		}
		out = append(out, text[:cut])
		text = strings.TrimPrefix(text[cut:], "\n")
	}
	if text != "" {
		out = append(out, text)
	}
	return out
}

func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }

// KV persists small channel state, such as the last message seen, across
// restarts.
type KV interface {
	// Get returns the value of key, or false if it is not set.
	Get(ctx context.Context, key string) (string, bool, error)
	// Set stores the value of key.
	Set(ctx context.Context, key, value string) error
}
