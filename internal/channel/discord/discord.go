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

// Package discord is the Discord channel of an agent server: a bot on the
// outbound gateway WebSocket, with application commands, buttons and a thread
// per conversation in guild channels.
package discord

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/a2aproject/a2a-cli/internal/channel"
)

const (
	maxText = 2000
	maxFile = 20 << 20
	// convsKey lists the conversations to catch up on after a reconnect.
	convsKey = "discord:convs"
)

// Channel is a Discord bot.
type Channel struct {
	token string
	kv    channel.KV

	once sync.Once
	err  error
	s    *discordgo.Session
	me   *discordgo.User

	mu    sync.Mutex
	in    chan<- channel.Inbound
	ctx   context.Context
	convs []string
}

var (
	_ channel.Channel   = (*Channel)(nil)
	_ channel.Describer = (*Channel)(nil)
)

// New returns a channel for the bot with token. kv keeps the last message
// seen per conversation.
func New(token string, kv channel.KV) *Channel { return &Channel{token: token, kv: kv} }

// Name implements [channel.Channel].
func (c *Channel) Name() string { return "discord" }

// Caps implements [channel.Channel].
func (c *Channel) Caps() channel.Caps {
	return channel.Caps{Edit: true, Buttons: true, Threads: true, Typing: true, MaxText: maxText, Markdown: true, Stream: true}
}

func (c *Channel) connect() error {
	c.once.Do(func() {
		s, err := discordgo.New("Bot " + c.token)
		if err != nil {
			c.err = fmt.Errorf("discord: %w", err)
			return
		}
		me, err := s.User("@me")
		if err != nil {
			c.err = fmt.Errorf("discord: checking the bot token: %w", err)
			return
		}
		s.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages | discordgo.IntentMessageContent
		c.s, c.me = s, me
	})
	return c.err
}

// Describe implements [channel.Describer].
func (c *Channel) Describe() string {
	if err := c.connect(); err != nil {
		return "(" + err.Error() + ")"
	}
	return "@" + c.me.Username
}

// LinkHint implements [channel.Describer].
func (c *Channel) LinkHint(code string) string {
	return fmt.Sprintf("send %s a direct message: /link %s", c.Describe(), code)
}

// Run implements [channel.Channel].
func (c *Channel) Run(ctx context.Context, in chan<- channel.Inbound) error {
	if err := c.connect(); err != nil {
		return err
	}
	c.mu.Lock()
	c.in, c.ctx = in, ctx
	c.mu.Unlock()
	if v, ok, err := c.kv.Get(ctx, convsKey); err == nil && ok && v != "" {
		c.convs = strings.Split(v, ",")
	}
	c.s.AddHandler(c.onReady)
	c.s.AddHandler(c.onMessage)
	c.s.AddHandler(c.onInteraction)
	if err := c.s.Open(); err != nil {
		return fmt.Errorf("discord gateway: %w", err)
	}
	<-ctx.Done()
	return c.s.Close()
}

func (c *Channel) deliver(m channel.Inbound) {
	c.mu.Lock()
	in, ctx := c.in, c.ctx
	c.mu.Unlock()
	select {
	case in <- m:
	case <-ctx.Done():
	}
}

// onReady registers the application commands and catches up on messages
// missed while disconnected, since the gateway doesn't replay them.
func (c *Channel) onReady(s *discordgo.Session, r *discordgo.Ready) {
	cmds := []*discordgo.ApplicationCommand{
		{Name: "link", Description: "Link your Discord account with a pairing code", Options: []*discordgo.ApplicationCommandOption{
			{Type: discordgo.ApplicationCommandOptionString, Name: "code", Description: "The pairing code", Required: true},
		}},
		{Name: "new", Description: "Start a new conversation"},
		{Name: "stop", Description: "Stop the current task"},
	}
	if _, err := s.ApplicationCommandBulkOverwrite(r.User.ID, "", cmds); err != nil {
		slog.Warn("discord: registering commands", "error", err)
	}
	c.mu.Lock()
	convs := slices.Clone(c.convs)
	c.mu.Unlock()
	for _, conv := range convs {
		c.catchUp(s, conv)
	}
}

func lastKey(conv string) string { return "discord:last:" + conv }

func (c *Channel) catchUp(s *discordgo.Session, conv string) {
	ctx := context.Background()
	last, ok, err := c.kv.Get(ctx, lastKey(conv))
	if err != nil || !ok || last == "" {
		return
	}
	msgs, err := s.ChannelMessages(conv, 50, "", last, "")
	if err != nil {
		slog.Warn("discord: catching up", "channel", conv, "error", err)
		return
	}
	// Messages come newest first.
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.GuildID == "" {
			if ch, err := s.State.Channel(conv); err == nil {
				m.GuildID = ch.GuildID
			}
		}
		c.onMessage(s, &discordgo.MessageCreate{Message: m})
	}
}

// track records the last message of a conversation for catching up.
func (c *Channel) track(conv, msgID string) {
	ctx := context.Background()
	if err := c.kv.Set(ctx, lastKey(conv), msgID); err != nil {
		slog.Warn("discord: recording last message", "error", err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if slices.Contains(c.convs, conv) {
		return
	}
	c.convs = append(c.convs, conv)
	if len(c.convs) > 200 {
		c.convs = c.convs[len(c.convs)-200:]
	}
	if err := c.kv.Set(ctx, convsKey, strings.Join(c.convs, ",")); err != nil {
		slog.Warn("discord: recording conversations", "error", err)
	}
}

func threadKey(id string) string { return "discord:thread:" + id }

func (c *Channel) ownThread(id string) bool {
	v, ok, err := c.kv.Get(context.Background(), threadKey(id))
	return err == nil && ok && v == "1"
}

func (c *Channel) onMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author == nil || m.Author.Bot || m.Author.ID == c.me.ID {
		return
	}
	text := m.Content
	direct := m.GuildID == ""
	conv := m.ChannelID
	if !direct && !c.ownThread(m.ChannelID) {
		// In guild channels the bot answers when mentioned and moves the
		// conversation into a thread of its own.
		if !slices.ContainsFunc(m.Mentions, func(u *discordgo.User) bool { return u.ID == c.me.ID }) {
			return
		}
		th, err := s.MessageThreadStartComplex(m.ChannelID, m.ID, &discordgo.ThreadStart{
			Name: threadName(text, c.me.ID), AutoArchiveDuration: 1440,
		})
		if err != nil {
			slog.Warn("discord: starting thread", "error", err)
		} else {
			conv = th.ID
			if err := c.kv.Set(context.Background(), threadKey(th.ID), "1"); err != nil {
				slog.Warn("discord: recording thread", "error", err)
			}
		}
	}
	text = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(text, "<@"+c.me.ID+">", ""), "<@!"+c.me.ID+">", ""))
	c.track(m.ChannelID, m.ID)
	in := channel.Inbound{
		Channel:   c.Name(),
		MessageID: m.ID,
		Identity:  "dc:" + m.Author.ID,
		Name:      m.Author.Username,
		Conv:      channel.ConvKey(conv),
		Direct:    direct,
		Text:      text,
		Authentic: true,
	}
	for _, a := range m.Attachments {
		f, err := download(a)
		if err != nil {
			slog.Warn("discord: downloading attachment", "error", err)
			continue
		}
		in.Files = append(in.Files, f)
	}
	c.deliver(in)
}

func threadName(text, botID string) string {
	name := strings.TrimSpace(strings.ReplaceAll(text, "<@"+botID+">", ""))
	if name == "" {
		name = "conversation"
	}
	if r := []rune(name); len(r) > 80 {
		name = string(r[:80])
	}
	return name
}

func download(a *discordgo.MessageAttachment) (channel.File, error) {
	if a.Size > maxFile {
		return channel.File{}, fmt.Errorf("attachment %s is too large", a.Filename)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return channel.File{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return channel.File{}, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Debug("discord: closing download", "error", err)
		}
	}()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxFile))
	if err != nil {
		return channel.File{}, err
	}
	return channel.File{Name: a.Filename, MediaType: a.ContentType, Data: data}, nil
}

func (c *Channel) onInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	user := i.User
	if i.Member != nil && i.Member.User != nil {
		user = i.Member.User
	}
	if user == nil {
		return
	}
	in := channel.Inbound{
		Channel:   c.Name(),
		MessageID: "int:" + i.ID,
		Identity:  "dc:" + user.ID,
		Name:      user.Username,
		Conv:      channel.ConvKey(i.ChannelID),
		Direct:    i.GuildID == "",
		Authentic: true,
	}
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		data := i.ApplicationCommandData()
		in.Text = "/" + data.Name
		if data.Name == "link" && len(data.Options) > 0 {
			in.Text = "link " + data.Options[0].StringValue()
		}
		respond(s, i, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "…", Flags: discordgo.MessageFlagsEphemeral},
		})
	case discordgo.InteractionMessageComponent:
		action, err := channel.ParseActionData(i.MessageComponentData().CustomID)
		if err != nil {
			return
		}
		in.Action = action
		if i.Message != nil {
			in.ReplyTo = channel.Ref{Conv: in.Conv, ID: i.Message.ID}
		}
		respond(s, i, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseDeferredMessageUpdate})
	default:
		return
	}
	c.deliver(in)
}

func respond(s *discordgo.Session, i *discordgo.InteractionCreate, r *discordgo.InteractionResponse) {
	if err := s.InteractionRespond(i.Interaction, r); err != nil {
		slog.Debug("discord: responding to interaction", "error", err)
	}
}

func components(q *channel.Question) []discordgo.MessageComponent {
	if q == nil {
		return []discordgo.MessageComponent{}
	}
	var rows []discordgo.MessageComponent
	var row []discordgo.MessageComponent
	for i, label := range q.Options {
		style := discordgo.SecondaryButton
		switch i {
		case 0:
			style = discordgo.PrimaryButton
		case len(q.Options) - 1:
			style = discordgo.DangerButton
		}
		row = append(row, discordgo.Button{Label: label, Style: style, CustomID: channel.ActionData(q.ID, i)})
		if len(row) == 5 {
			rows = append(rows, discordgo.ActionsRow{Components: row})
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, discordgo.ActionsRow{Components: row})
	}
	return rows
}

// Send implements [channel.Channel].
func (c *Channel) Send(ctx context.Context, m channel.Outbound) (channel.Ref, error) {
	if err := c.connect(); err != nil {
		return channel.Ref{}, err
	}
	var ref channel.Ref
	pieces := channel.Split(m.Text, maxText)
	for i, piece := range pieces {
		send := &discordgo.MessageSend{Content: piece}
		last := i == len(pieces)-1
		if last && m.Question != nil {
			send.Components = components(m.Question)
		}
		if last {
			for _, f := range m.Files {
				send.Files = append(send.Files, &discordgo.File{Name: f.Name, ContentType: f.MediaType, Reader: bytes.NewReader(f.Data)})
			}
		}
		if send.Content == "" && len(send.Files) == 0 {
			continue
		}
		msg, err := c.s.ChannelMessageSendComplex(string(m.Conv), send, discordgo.WithContext(ctx))
		if err != nil {
			return ref, fmt.Errorf("discord send: %w", err)
		}
		ref = channel.Ref{Conv: m.Conv, ID: msg.ID}
		c.track(string(m.Conv), msg.ID)
	}
	return ref, nil
}

// Edit implements [channel.Channel].
func (c *Channel) Edit(ctx context.Context, ref channel.Ref, m channel.Outbound) error {
	if err := c.connect(); err != nil {
		return err
	}
	text := m.Text
	if len(text) > maxText {
		text = text[:maxText]
	}
	comps := components(m.Question)
	_, err := c.s.ChannelMessageEditComplex(&discordgo.MessageEdit{
		ID: ref.ID, Channel: string(ref.Conv), Content: &text, Components: &comps,
	}, discordgo.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("discord edit: %w", err)
	}
	return nil
}
