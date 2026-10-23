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

// Package telegram is the Telegram channel of an agent server: a bot reached
// with long polling, so no public URL is needed.
package telegram

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/a2aproject/a2a-cli/internal/channel"
)

// maxText is Telegram's message limit, minus room for MarkdownV2 escapes.
const maxText = 3500

// maxFile is the largest file a bot can download.
const maxFile = 20 << 20

// Channel is a Telegram bot.
type Channel struct {
	token string
	// ServerURL overrides the Bot API URL, for tests.
	ServerURL string

	once sync.Once
	err  error
	bot  *bot.Bot
	me   *models.User
	in   chan<- channel.Inbound
	mu   sync.Mutex
}

var (
	_ channel.Channel   = (*Channel)(nil)
	_ channel.Describer = (*Channel)(nil)
)

// New returns a channel for the bot with token.
func New(token string) *Channel { return &Channel{token: token} }

// Name implements [channel.Channel].
func (c *Channel) Name() string { return "telegram" }

// Caps implements [channel.Channel].
func (c *Channel) Caps() channel.Caps {
	return channel.Caps{Edit: true, Buttons: true, Threads: true, Typing: true, MaxText: maxText, Markdown: true, Stream: true}
}

func (c *Channel) connect(ctx context.Context) error {
	c.once.Do(func() {
		opts := []bot.Option{
			bot.WithDefaultHandler(c.handle),
			bot.WithAllowedUpdates(bot.AllowedUpdates{"message", "callback_query"}),
			bot.WithHTTPClient(50*time.Second, &http.Client{Timeout: 70 * time.Second}),
		}
		if c.ServerURL != "" {
			opts = append(opts, bot.WithServerURL(c.ServerURL))
		}
		b, err := bot.New(c.token, opts...)
		if err != nil {
			c.err = fmt.Errorf("connecting to the Telegram bot: %w", err)
			return
		}
		me, err := b.GetMe(ctx)
		if err != nil {
			c.err = fmt.Errorf("telegram getMe: %w", err)
			return
		}
		c.bot, c.me = b, me
	})
	return c.err
}

// Describe implements [channel.Describer].
func (c *Channel) Describe() string {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := c.connect(ctx); err != nil {
		return "(" + err.Error() + ")"
	}
	return "@" + c.me.Username
}

// LinkHint implements [channel.Describer]: a deep link which sends /start <code>.
func (c *Channel) LinkHint(code string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := c.connect(ctx); err != nil {
		return "send the bot: /start " + code
	}
	return "https://t.me/" + c.me.Username + "?start=" + code
}

// Run implements [channel.Channel]. Telegram keeps pending updates for a
// while, so a server that was briefly offline catches up on restart.
func (c *Channel) Run(ctx context.Context, in chan<- channel.Inbound) error {
	if err := c.connect(ctx); err != nil {
		return err
	}
	c.mu.Lock()
	c.in = in
	c.mu.Unlock()
	if _, err := c.bot.SetMyCommands(ctx, &bot.SetMyCommandsParams{Commands: []models.BotCommand{
		{Command: "new", Description: "Start a new conversation"},
		{Command: "stop", Description: "Stop the current task"},
	}}); err != nil {
		slog.Warn("telegram: setting commands", "error", err)
	}
	c.bot.Start(ctx)
	return nil
}

func (c *Channel) deliver(ctx context.Context, m channel.Inbound) {
	c.mu.Lock()
	in := c.in
	c.mu.Unlock()
	select {
	case in <- m:
	case <-ctx.Done():
	}
}

func (c *Channel) handle(ctx context.Context, b *bot.Bot, u *models.Update) {
	switch {
	case u.CallbackQuery != nil:
		c.handleCallback(ctx, b, u.CallbackQuery)
	case u.Message != nil:
		c.handleMessage(ctx, b, u.Message)
	}
}

func convKey(chatID int64, thread int) channel.ConvKey {
	if thread != 0 {
		return channel.ConvKey(strconv.FormatInt(chatID, 10) + ":" + strconv.Itoa(thread))
	}
	return channel.ConvKey(strconv.FormatInt(chatID, 10))
}

func parseConv(conv channel.ConvKey) (int64, int, error) {
	chat, thread, _ := strings.Cut(string(conv), ":")
	id, err := strconv.ParseInt(chat, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid telegram conversation %q", conv)
	}
	t := 0
	if thread != "" {
		if t, err = strconv.Atoi(thread); err != nil {
			return 0, 0, fmt.Errorf("invalid telegram conversation %q", conv)
		}
	}
	return id, t, nil
}

func displayName(u *models.User) string {
	if u.Username != "" {
		return "@" + u.Username
	}
	return strings.TrimSpace(u.FirstName + " " + u.LastName)
}

func (c *Channel) handleMessage(ctx context.Context, b *bot.Bot, msg *models.Message) {
	if msg.From == nil || msg.From.IsBot {
		return
	}
	text := msg.Text
	if text == "" {
		text = msg.Caption
	}
	direct := msg.Chat.Type == "private"
	if !direct {
		// In groups, the bot answers when addressed, so that it doesn't
		// treat every message of a linked user as a task.
		mention := "@" + c.me.Username
		replyToBot := msg.ReplyToMessage != nil && msg.ReplyToMessage.From != nil && msg.ReplyToMessage.From.ID == c.me.ID
		switch {
		case strings.Contains(text, mention):
			text = strings.TrimSpace(strings.ReplaceAll(text, mention, ""))
		case replyToBot, strings.HasPrefix(text, "/"):
		default:
			return
		}
	}
	thread := 0
	if msg.IsTopicMessage {
		thread = msg.MessageThreadID
	}
	conv := convKey(msg.Chat.ID, thread)
	m := channel.Inbound{
		Channel:   c.Name(),
		MessageID: fmt.Sprintf("%d:%d", msg.Chat.ID, msg.ID),
		Identity:  "tg:" + strconv.FormatInt(msg.From.ID, 10),
		Name:      displayName(msg.From),
		Conv:      conv,
		Direct:    direct,
		Text:      text,
		Authentic: true,
	}
	if msg.Document != nil {
		if f, err := c.download(ctx, b, msg.Document.FileID, msg.Document.FileName, msg.Document.MimeType); err == nil {
			m.Files = append(m.Files, f)
		} else {
			slog.Warn("telegram: downloading document", "error", err)
		}
	}
	if n := len(msg.Photo); n > 0 {
		if f, err := c.download(ctx, b, msg.Photo[n-1].FileID, "photo.jpg", "image/jpeg"); err == nil {
			m.Files = append(m.Files, f)
		} else {
			slog.Warn("telegram: downloading photo", "error", err)
		}
	}
	c.deliver(ctx, m)
}

func (c *Channel) download(ctx context.Context, b *bot.Bot, fileID, name, mediaType string) (channel.File, error) {
	f, err := b.GetFile(ctx, &bot.GetFileParams{FileID: fileID})
	if err != nil {
		return channel.File{}, err
	}
	if f.FileSize > maxFile {
		return channel.File{}, fmt.Errorf("file %s is too large (%d bytes)", name, f.FileSize)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.FileDownloadLink(f), nil)
	if err != nil {
		return channel.File{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return channel.File{}, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Debug("telegram: closing download", "error", err)
		}
	}()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxFile))
	if err != nil {
		return channel.File{}, err
	}
	return channel.File{Name: name, MediaType: mediaType, Data: data}, nil
}

func (c *Channel) handleCallback(ctx context.Context, b *bot.Bot, q *models.CallbackQuery) {
	if _, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{CallbackQueryID: q.ID}); err != nil {
		slog.Debug("telegram: answering callback", "error", err)
	}
	msg := q.Message.Message
	if msg == nil {
		return
	}
	action, err := channel.ParseActionData(q.Data)
	if err != nil {
		return
	}
	thread := 0
	if msg.IsTopicMessage {
		thread = msg.MessageThreadID
	}
	conv := convKey(msg.Chat.ID, thread)
	c.deliver(ctx, channel.Inbound{
		Channel:   c.Name(),
		MessageID: "cb:" + q.ID,
		Identity:  "tg:" + strconv.FormatInt(q.From.ID, 10),
		Name:      displayName(&q.From),
		Conv:      conv,
		Direct:    msg.Chat.Type == "private",
		Action:    action,
		Authentic: true,
		ReplyTo:   channel.Ref{Conv: conv, ID: strconv.Itoa(msg.ID)},
	})
}

func keyboard(q *channel.Question) models.ReplyMarkup {
	if q == nil {
		return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{}}
	}
	row := make([]models.InlineKeyboardButton, 0, len(q.Options))
	for i, label := range q.Options {
		row = append(row, models.InlineKeyboardButton{Text: label, CallbackData: channel.ActionData(q.ID, i)})
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{row}}
}

// isParseError reports whether Telegram rejected the MarkdownV2 entities.
func isParseError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "can't parse entities")
}

func isNotModified(err error) bool {
	return err != nil && strings.Contains(err.Error(), "message is not modified")
}

// Send implements [channel.Channel].
func (c *Channel) Send(ctx context.Context, m channel.Outbound) (channel.Ref, error) {
	if err := c.connect(ctx); err != nil {
		return channel.Ref{}, err
	}
	chat, thread, err := parseConv(m.Conv)
	if err != nil {
		return channel.Ref{}, err
	}
	var ref channel.Ref
	for _, f := range m.Files {
		msg, err := c.bot.SendDocument(ctx, &bot.SendDocumentParams{
			ChatID: chat, MessageThreadID: thread,
			Document: &models.InputFileUpload{Filename: f.Name, Data: bytes.NewReader(f.Data)},
		})
		if err != nil {
			return ref, fmt.Errorf("telegram sendDocument: %w", err)
		}
		ref = channel.Ref{Conv: m.Conv, ID: strconv.Itoa(msg.ID)}
	}
	pieces := channel.Split(m.Text, maxText)
	for i, piece := range pieces {
		if piece == "" {
			continue
		}
		params := &bot.SendMessageParams{
			ChatID: chat, MessageThreadID: thread, Text: ToMarkdownV2(piece), ParseMode: models.ParseModeMarkdown,
			LinkPreviewOptions: &models.LinkPreviewOptions{IsDisabled: bot.True()},
		}
		if i == len(pieces)-1 && m.Question != nil {
			params.ReplyMarkup = keyboard(m.Question)
		}
		msg, err := c.bot.SendMessage(ctx, params)
		if isParseError(err) {
			params.Text, params.ParseMode = piece, ""
			msg, err = c.bot.SendMessage(ctx, params)
		}
		if err != nil {
			return ref, fmt.Errorf("telegram sendMessage: %w", err)
		}
		ref = channel.Ref{Conv: m.Conv, ID: strconv.Itoa(msg.ID)}
	}
	return ref, nil
}

// Edit implements [channel.Channel].
func (c *Channel) Edit(ctx context.Context, ref channel.Ref, m channel.Outbound) error {
	if err := c.connect(ctx); err != nil {
		return err
	}
	chat, _, err := parseConv(ref.Conv)
	if err != nil {
		return err
	}
	id, err := strconv.Atoi(ref.ID)
	if err != nil {
		return fmt.Errorf("invalid telegram message id %q", ref.ID)
	}
	text := m.Text
	if len(text) > maxText {
		text = text[:maxText]
	}
	params := &bot.EditMessageTextParams{
		ChatID: chat, MessageID: id, Text: ToMarkdownV2(text), ParseMode: models.ParseModeMarkdown,
		ReplyMarkup:        keyboard(m.Question),
		LinkPreviewOptions: &models.LinkPreviewOptions{IsDisabled: bot.True()},
	}
	_, err = c.bot.EditMessageText(ctx, params)
	if isParseError(err) {
		params.Text, params.ParseMode = text, ""
		_, err = c.bot.EditMessageText(ctx, params)
	}
	if err != nil && !isNotModified(err) {
		return fmt.Errorf("telegram editMessageText: %w", err)
	}
	return nil
}
