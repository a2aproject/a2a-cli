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

package agentsrv

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"

	"github.com/a2aproject/a2a-cli/internal/auth"
	"github.com/a2aproject/a2a-cli/internal/channel"
	"github.com/a2aproject/a2a-cli/internal/store"
)

// Router turns channel messages into A2A tasks of the agent and renders
// task events back into the conversation.
type Router struct {
	s   *Server
	ctx context.Context
	in  chan channel.Inbound

	mu    sync.Mutex
	chans map[string]*runningChannel
	convs map[string]*convMutex
	// asked maps question IDs to the messages which show them, so that they
	// can be updated once answered.
	asked map[string][]askedRef
}

type runningChannel struct {
	ch     channel.Channel
	cancel context.CancelFunc
	done   chan struct{}
	// err is the last error of Run, cleared when it runs again.
	err error
}

type convMutex struct {
	sync.Mutex
	refs int
}

// Channel restarts back off from channelRetryMin to channelRetryMax.
var (
	channelRetryMin = 5 * time.Second
	channelRetryMax = 5 * time.Minute
)

// newRouter returns a router which dispatches channel messages until ctx ends.
func newRouter(ctx context.Context, s *Server) *Router {
	r := &Router{
		s:     s,
		ctx:   ctx,
		in:    make(chan channel.Inbound, 64),
		chans: map[string]*runningChannel{},
		convs: map[string]*convMutex{},
		asked: map[string][]askedRef{},
	}
	go r.dispatch(ctx)
	return r
}

// add starts a channel, replacing a running one of the same name after it
// stopped. A failing channel is restarted with backoff rather than stopping
// the server, so that a messenger outage doesn't take the agent down.
func (r *Router) add(ch channel.Channel) {
	r.remove(ch.Name())
	cctx, cancel := context.WithCancel(r.ctx)
	rc := &runningChannel{ch: ch, cancel: cancel, done: make(chan struct{})}
	r.mu.Lock()
	r.chans[ch.Name()] = rc
	r.mu.Unlock()
	go r.run(cctx, rc)
}

func (r *Router) run(ctx context.Context, rc *runningChannel) {
	defer close(rc.done)
	delay := channelRetryMin
	for {
		err := rc.ch.Run(ctx, r.in)
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			err = errors.New("stopped unexpectedly")
		}
		slog.Error("channel stopped; restarting", "channel", rc.ch.Name(), "in", delay, "error", err)
		r.mu.Lock()
		rc.err = err
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		delay = min(delay*2, channelRetryMax)
		r.mu.Lock()
		rc.err = nil
		r.mu.Unlock()
	}
}

// remove stops a channel and waits until it stopped, so that a replacement
// never polls the same account at the same time.
func (r *Router) remove(name string) {
	r.mu.Lock()
	rc := r.chans[name]
	delete(r.chans, name)
	r.mu.Unlock()
	if rc == nil {
		return
	}
	rc.cancel()
	<-rc.done
}

// stopAll stops every channel and waits until they stopped or ctx ends.
func (r *Router) stopAll(ctx context.Context) error {
	r.mu.Lock()
	running := make([]*runningChannel, 0, len(r.chans))
	for name, rc := range r.chans {
		running = append(running, rc)
		delete(r.chans, name)
	}
	r.mu.Unlock()
	for _, rc := range running {
		rc.cancel()
	}
	for _, rc := range running {
		select {
		case <-rc.done:
		case <-ctx.Done():
			return fmt.Errorf("waiting for %s to stop: %w", rc.ch.Name(), ctx.Err())
		}
	}
	return nil
}

func (r *Router) channel(name string) channel.Channel {
	r.mu.Lock()
	defer r.mu.Unlock()
	if rc := r.chans[name]; rc != nil {
		return rc.ch
	}
	return nil
}

func (r *Router) channels() []channel.Channel {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]channel.Channel, 0, len(r.chans))
	for _, rc := range r.chans {
		out = append(out, rc.ch)
	}
	slices.SortFunc(out, func(a, b channel.Channel) int { return strings.Compare(a.Name(), b.Name()) })
	return out
}

func (r *Router) status() []ChannelStatus {
	var out []ChannelStatus
	for _, ch := range r.channels() {
		out = append(out, *r.channelStatus(ch, ""))
	}
	return out
}

func (r *Router) channelStatus(ch channel.Channel, hint string) *ChannelStatus {
	st := &ChannelStatus{Name: ch.Name()}
	if d, ok := ch.(channel.Describer); ok {
		st.Account = d.Describe()
	}
	if hint != "" {
		st.Account = strings.TrimSpace(st.Account + "\n" + hint)
	}
	r.mu.Lock()
	if rc := r.chans[ch.Name()]; rc != nil && rc.err != nil {
		st.Error = rc.err.Error()
	}
	r.mu.Unlock()
	return st
}

// announce prints the channels, and how to link the owner if nobody is
// linked as owner yet.
func (r *Router) announce(ctx context.Context) error {
	for _, ch := range r.channels() {
		line := "✓ " + ch.Name()
		if d, ok := ch.(channel.Describer); ok {
			line += " " + d.Describe()
		}
		r.s.logf("%s", line)
		if hint := r.ownerHint(ctx, ch); hint != "" {
			r.s.logf("%s", hint)
		}
	}
	return nil
}

// ownerHint mints an owner code for a channel nobody is linked on as owner.
func (r *Router) ownerHint(ctx context.Context, ch channel.Channel) string {
	links, err := r.s.store.LinksOf(ctx, auth.OwnerID)
	if err != nil {
		slog.Error("listing owner links", "error", err)
		return ""
	}
	if slices.ContainsFunc(links, func(l store.Link) bool { return l.Channel == ch.Name() }) {
		return ""
	}
	d, ok := ch.(channel.Describer)
	if !ok {
		return ""
	}
	code, _, err := auth.MintCode(ctx, r.s.store, auth.PurposePair, "owner", auth.RoleOwner)
	if err != nil {
		slog.Error("minting owner code", "error", err)
		return ""
	}
	return fmt.Sprintf("→ link yourself as owner: %s   (%d min)", d.LinkHint(code), int(auth.CodeTTL.Minutes()))
}

func (r *Router) linkHints(code string) []string {
	var out []string
	for _, ch := range r.channels() {
		if d, ok := ch.(channel.Describer); ok {
			out = append(out, ch.Name()+": "+d.LinkHint(code))
		}
	}
	return out
}

func (r *Router) dispatch(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case in := <-r.in:
			go r.handle(ctx, in)
		}
	}
}

// lockConv serializes the handling of one conversation's messages. The
// returned function unlocks it.
func (r *Router) lockConv(ch string, conv channel.ConvKey) func() {
	key := ch + "\x00" + string(conv)
	r.mu.Lock()
	l := r.convs[key]
	if l == nil {
		l = &convMutex{}
		r.convs[key] = l
	}
	l.refs++
	r.mu.Unlock()
	l.Lock()
	return func() {
		l.Unlock()
		r.mu.Lock()
		defer r.mu.Unlock()
		if l.refs--; l.refs == 0 {
			delete(r.convs, key)
		}
	}
}

func (r *Router) reply(ctx context.Context, ch channel.Channel, in channel.Inbound, text string) {
	if _, err := ch.Send(ctx, channel.Outbound{Conv: in.Conv, Text: text, ReplyTo: channel.Ref{Conv: in.Conv, ID: in.MessageID}}); err != nil {
		slog.Warn("replying in channel", "channel", ch.Name(), "error", err)
	}
}

func (r *Router) handle(ctx context.Context, in channel.Inbound) {
	ch := r.channel(in.Channel)
	if ch == nil {
		return
	}
	first, err := r.s.store.FirstSeen(ctx, in.Channel+"|"+in.MessageID)
	if err != nil {
		slog.Error("dedup", "error", err)
		return
	}
	if !first {
		return
	}
	if in.Action != nil {
		r.handleAction(ctx, ch, in)
		return
	}
	if code, ok := auth.LinkCode(in.Text); ok {
		p, err := auth.Link(ctx, r.s.store, &in, code)
		if err != nil {
			r.reply(ctx, ch, in, "That code is invalid or has expired. Ask the owner for a new one.")
			return
		}
		r.reply(ctx, ch, in, fmt.Sprintf("Linked as %s. Write to talk to %s.", strings.Join(p.Roles, ", "), r.s.config().Name))
		return
	}
	p, ok, err := r.s.chain.Authenticate(ctx, auth.Request{Channel: &in})
	if err != nil {
		slog.Error("authenticating channel message", "error", err)
		return
	}
	if !ok {
		if in.Direct && in.Authentic {
			r.reply(ctx, ch, in, "I don't know you yet. Ask my owner for a code (`a2a server pair --name <you>`), then send: link <code>")
		}
		return
	}
	if in.Prepare != nil {
		if err := in.Prepare(ctx, &in); err != nil {
			slog.Warn("preparing channel message", "channel", in.Channel, "error", err)
		}
	}
	// Stops and answers must not wait for the turn they stop or unblock,
	// which holds the conversation.
	if isCommand(in.Text, "stop") {
		r.stop(ctx, ch, in, p)
		return
	}
	if r.answerTyped(ctx, ch, in, p) {
		return
	}

	defer r.lockConv(in.Channel, in.Conv)()
	switch {
	case isCommand(in.Text, "new"):
		if err := r.s.store.SetContext(ctx, in.Channel, string(in.Conv), a2a.NewContextID()); err != nil {
			slog.Error("starting a new context", "error", err)
			return
		}
		r.reply(ctx, ch, in, "Started a new conversation.")
		return
	case isCommand(in.Text, "start"):
		r.reply(ctx, ch, in, fmt.Sprintf("Hi! I'm %s. Write what you need; /new starts a new conversation, /stop stops the current task.", r.s.config().Name))
		return
	}

	contextID, err := r.contextOf(ctx, in.Channel, in.Conv)
	if err != nil {
		slog.Error("resolving conversation context", "error", err)
		return
	}
	msg := a2a.NewMessage(a2a.MessageRoleUser, messageParts(in)...)
	msg.ID = in.Channel + ":" + in.MessageID
	msg.ContextID = contextID
	msg.Metadata = map[string]any{"channel": in.Channel, "identity": in.Identity, "sender": in.Name}
	if last, err := r.s.tasks.LatestInContext(ctx, contextID); err == nil && last.Status.State == a2a.TaskStateInputRequired {
		msg.TaskID = last.ID
	}
	r.stream(ctx, ch, in.Conv, channel.Ref{Conv: in.Conv, ID: in.MessageID}, p, &a2a.SendMessageRequest{Message: msg})
}

// isCommand reports whether text is the bot command /name, optionally
// addressed as /name@bot.
func isCommand(text, name string) bool {
	cmd := strings.ToLower(strings.TrimSpace(text))
	return cmd == "/"+name || strings.HasPrefix(cmd, "/"+name+"@")
}

func messageParts(in channel.Inbound) []*a2a.Part {
	var parts []*a2a.Part
	if in.Text != "" {
		parts = append(parts, a2a.NewTextPart(in.Text))
	}
	for _, f := range in.Files {
		p := a2a.NewRawPart(f.Data)
		p.Filename, p.MediaType = f.Name, f.MediaType
		parts = append(parts, p)
	}
	if len(parts) == 0 {
		parts = append(parts, a2a.NewTextPart("(empty message)"))
	}
	return parts
}

func (r *Router) contextOf(ctx context.Context, ch string, conv channel.ConvKey) (string, error) {
	id, err := r.s.store.ContextOf(ctx, ch, string(conv))
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return "", err
	}
	id = a2a.NewContextID()
	return id, r.s.store.SetContext(ctx, ch, string(conv), id)
}

func (r *Router) stop(ctx context.Context, ch channel.Channel, in channel.Inbound, p auth.Principal) {
	contextID, err := r.s.store.ContextOf(ctx, in.Channel, string(in.Conv))
	if err != nil {
		r.reply(ctx, ch, in, "Nothing is running.")
		return
	}
	last, err := r.s.tasks.LatestInContext(ctx, contextID)
	if err != nil || last.Status.State.Terminal() {
		r.reply(ctx, ch, in, "Nothing is running.")
		return
	}
	if _, err := r.s.handler.CancelTask(inProcess(ctx, p), &a2a.CancelTaskRequest{ID: last.ID}); err != nil {
		r.reply(ctx, ch, in, "Couldn't stop the task: "+err.Error())
		return
	}
	r.reply(ctx, ch, in, "Stopped.")
}

// stream sends a message to the agent and renders the task in the conversation.
func (r *Router) stream(ctx context.Context, ch channel.Channel, conv channel.ConvKey, replyTo channel.Ref, p auth.Principal, req *a2a.SendMessageRequest) {
	rd := newRenderer(r, ch, conv, replyTo, p)
	for ev, err := range r.s.handler.SendStreamingMessage(inProcess(ctx, p), req) {
		if err != nil {
			rd.fail(ctx, err)
			return
		}
		if rd.onEvent(ctx, ev) {
			return
		}
	}
	rd.finish(ctx)
}

// follow renders the rest of a task which continues after someone else
// answered its question.
func (r *Router) follow(ctx context.Context, ch channel.Channel, conv channel.ConvKey, p auth.Principal, taskID a2a.TaskID) {
	events, unsubscribe := r.s.bus.subscribe(taskID)
	go func() {
		defer unsubscribe()
		rd := newRenderer(r, ch, conv, channel.Ref{}, p)
		rd.following = true
		timeout := time.NewTimer(25 * time.Hour)
		defer timeout.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timeout.C:
				return
			case ev := <-events:
				if rd.onEvent(ctx, ev) {
					return
				}
			}
		}
	}()
}

// handleAction answers a question from a button click.
func (r *Router) handleAction(ctx context.Context, ch channel.Channel, in channel.Inbound) {
	rec, err := r.s.store.QuestionByID(ctx, in.Action.QuestionID)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			slog.Error("loading question", "question", in.Action.QuestionID, "error", err)
		}
		return
	}
	if rec.AnsweredBy != "" {
		r.reply(ctx, ch, in, "Already answered.")
		return
	}
	p, ok, err := r.s.chain.Authenticate(ctx, auth.Request{Channel: &in})
	if err != nil {
		slog.Error("authenticating channel action", "error", err)
		return
	}
	if !ok {
		return
	}
	q := questionFromRecord(rec)
	if in.Action.Option >= len(q.Options) {
		return
	}
	r.answerIn(ctx, ch, in, p, rec, q, q.Options[in.Action.Option])
}

// answerTyped answers a question asked in this conversation by text, for
// channels without buttons and for people who type instead of clicking.
func (r *Router) answerTyped(ctx context.Context, ch channel.Channel, in channel.Inbound, p auth.Principal) bool {
	id, ok, err := r.s.store.Get(ctx, pendingKey(in.Channel, in.Conv))
	if err != nil || !ok || id == "" {
		return false
	}
	rec, err := r.s.store.QuestionByID(ctx, id)
	if err != nil || rec.AnsweredBy != "" {
		return false
	}
	q := questionFromRecord(rec)
	opt, ok := q.matchOption(in.Text)
	if !ok {
		return false
	}
	r.answerIn(ctx, ch, in, p, rec, q, opt)
	return true
}

func pendingKey(ch string, conv channel.ConvKey) string { return "pending:" + ch + ":" + string(conv) }

// answerIn answers a question from a conversation. If the question belongs
// to the task of this conversation, the rest of the task is rendered here.
func (r *Router) answerIn(ctx context.Context, ch channel.Channel, in channel.Inbound, p auth.Principal, rec store.Question, q *hitlQuestion, opt hitlOption) {
	if q.Type == questionPermission && !r.s.canApprove(ctx, p) {
		r.reply(ctx, ch, in, "Only the owner or an approver can answer this.")
		return
	}
	if err := r.s.store.Set(ctx, pendingKey(in.Channel, in.Conv), ""); err != nil {
		slog.Warn("clearing pending question", "error", err)
	}
	who := in.Name
	if who == "" {
		who = displayPrincipal(p.ID)
	}
	own, err := r.s.store.ContextOf(ctx, in.Channel, string(in.Conv))
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		slog.Warn("resolving conversation context", "error", err)
	}
	if rec.Contact == "" && rec.ContextID == own {
		if err := r.s.store.AnswerQuestion(ctx, rec.ID, p.ID, opt.ID); err != nil {
			if !errors.Is(err, store.ErrAnswered) {
				slog.Error("answering question", "question", rec.ID, "error", err)
				r.reply(ctx, ch, in, "Couldn't record the answer. Try again.")
				return
			}
			r.reply(ctx, ch, in, "Already answered.")
			return
		}
		r.markAnswered(ctx, rec.ID, opt.Label, who)
		msg := answerMessage(a2a.TaskID(rec.TaskID), rec.ContextID, q.ID, opt.ID)
		defer r.lockConv(in.Channel, in.Conv)()
		r.stream(ctx, ch, in.Conv, channel.Ref{}, p, &a2a.SendMessageRequest{Message: msg})
		return
	}
	if err := r.s.answer(ctx, rec, p, opt); err != nil {
		if errors.Is(err, store.ErrAnswered) {
			r.reply(ctx, ch, in, "Already answered.")
			return
		}
		r.reply(ctx, ch, in, "Couldn't send the answer: "+err.Error())
		return
	}
	r.markAnswered(ctx, rec.ID, opt.Label, who)
}

// markAnswered replaces the buttons of a question with who answered it.
func (r *Router) markAnswered(ctx context.Context, questionID, label, who string) {
	r.mu.Lock()
	refs := r.asked[questionID]
	delete(r.asked, questionID)
	r.mu.Unlock()
	for _, ref := range refs {
		ch := r.channel(ref.channel)
		if ch == nil || !ch.Caps().Edit {
			continue
		}
		text := fmt.Sprintf("%s\n\n✓ %s by %s", ref.text, label, who)
		if err := ch.Edit(ctx, ref.Ref, channel.Outbound{Conv: ref.Conv, Text: text}); err != nil {
			slog.Debug("updating answered question", "error", err)
		}
	}
}

// ask sends a question to users on their direct conversations, or to one
// preferred conversation if given.
func (r *Router) ask(ctx context.Context, userIDs []string, prefChannel string, prefConv channel.ConvKey, header string, rec store.Question) {
	q := questionFromRecord(rec)
	text := q.Text
	if text == "" {
		text = q.Title
	}
	text = header + ":\n" + text
	type target struct {
		ch   channel.Channel
		conv channel.ConvKey
	}
	var targets []target
	if ch := r.channel(prefChannel); ch != nil && prefConv != "" {
		targets = append(targets, target{ch, prefConv})
	} else {
		for _, id := range userIDs {
			links, err := r.s.store.LinksOf(ctx, id)
			if err != nil {
				slog.Error("listing links", "error", err)
				continue
			}
			for _, l := range links {
				if ch := r.channel(l.Channel); ch != nil && l.Conv != "" {
					targets = append(targets, target{ch, channel.ConvKey(l.Conv)})
				}
			}
		}
	}
	for _, t := range targets {
		out := channel.Outbound{Conv: t.conv, Text: text, Subject: header}
		if t.ch.Caps().Buttons {
			out.Question = &channel.Question{ID: rec.ID, Options: q.labels()}
		} else {
			out.Text += fmt.Sprintf("\n\nReply %s, or open %s", optionWords(q), r.s.hitlURL(rec))
		}
		ref, err := t.ch.Send(ctx, out)
		if err != nil {
			slog.Warn("sending question", "channel", t.ch.Name(), "error", err)
			continue
		}
		r.remember(rec.ID, t.ch.Name(), ref, text)
		if err := r.s.store.Set(ctx, pendingKey(t.ch.Name(), t.conv), rec.ID); err != nil {
			slog.Warn("recording pending question", "error", err)
		}
	}
}

func optionWords(q *hitlQuestion) string {
	var words []string
	for _, o := range q.Options {
		switch o.ID {
		case "once":
			words = append(words, "approve")
		case "always":
			words = append(words, "always")
		case "reject":
			words = append(words, "deny")
		default:
			words = append(words, o.ID)
		}
	}
	return strings.Join(words, ", ")
}

type askedRef struct {
	channel.Ref
	channel string
	text    string
}

func (r *Router) remember(questionID, ch string, ref channel.Ref, text string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.asked[questionID] = append(r.asked[questionID], askedRef{Ref: ref, channel: ch, text: text})
}

// injectTurn posts text into a context as a new turn, rendered in the
// context's conversation if it has one.
func (r *Router) injectTurn(ctx context.Context, contextID string, p auth.Principal, text string) error {
	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart(text))
	msg.ContextID = contextID
	chName, conv, err := r.s.store.ConvOf(ctx, contextID)
	if err == nil {
		if ch := r.channel(chName); ch != nil {
			go func() {
				defer r.lockConv(chName, channel.ConvKey(conv))()
				if last, err := r.s.tasks.LatestInContext(ctx, contextID); err == nil && last.Status.State == a2a.TaskStateInputRequired {
					msg.TaskID = last.ID
				}
				r.stream(ctx, ch, channel.ConvKey(conv), channel.Ref{}, p, &a2a.SendMessageRequest{Message: msg})
			}()
			return nil
		}
	}
	_, err = r.s.handler.SendMessage(inProcess(ctx, p), &a2a.SendMessageRequest{Message: msg, Config: &a2a.SendMessageConfig{ReturnImmediately: true}})
	return err
}
