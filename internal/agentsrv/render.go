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
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"

	"github.com/a2aproject/a2a-cli/internal/auth"
	"github.com/a2aproject/a2a-cli/internal/channel"
)

// editInterval throttles streaming edits; messengers rate-limit them.
const editInterval = time.Second

// renderer shows one task in a conversation. Channels which stream get one
// message edited as the agent writes; others get questions and the result.
type renderer struct {
	r       *Router
	ch      channel.Channel
	caps    channel.Caps
	conv    channel.ConvKey
	replyTo channel.Ref
	p       auth.Principal
	// following is set when rendering a task answered elsewhere.
	following bool

	mu        sync.Mutex
	taskID    a2a.TaskID
	contextID string
	artifacts []*artifactText
	files     []channel.File
	progress  string

	ref       channel.Ref
	segStart  int
	lastShown string
	lastEdit  time.Time
	timer     *time.Timer
	done      bool
}

type artifactText struct {
	id   a2a.ArtifactID
	text strings.Builder
}

func newRenderer(r *Router, ch channel.Channel, conv channel.ConvKey, replyTo channel.Ref, p auth.Principal) *renderer {
	caps := ch.Caps()
	if caps.MaxText <= 0 {
		caps.MaxText = 4000
	}
	return &renderer{r: r, ch: ch, caps: caps, conv: conv, replyTo: replyTo, p: p}
}

// onEvent renders an event and reports whether the task stopped: it
// finished or waits for input.
func (rd *renderer) onEvent(ctx context.Context, ev a2a.Event) bool {
	switch e := ev.(type) {
	case *a2a.Task:
		rd.setTask(e.ID, e.ContextID)
		for _, a := range e.Artifacts {
			rd.onArtifact(a, false)
		}
		return rd.onStatus(ctx, e.Status)
	case *a2a.TaskArtifactUpdateEvent:
		rd.setTask(e.TaskID, e.ContextID)
		rd.onArtifact(e.Artifact, e.Append)
		rd.flush(ctx, false)
	case *a2a.TaskStatusUpdateEvent:
		rd.setTask(e.TaskID, e.ContextID)
		return rd.onStatus(ctx, e.Status)
	case *a2a.Message:
		rd.send(ctx, textOf(e.Parts))
		return true
	}
	return false
}

func (rd *renderer) setTask(id a2a.TaskID, contextID string) {
	rd.mu.Lock()
	defer rd.mu.Unlock()
	rd.taskID, rd.contextID = id, contextID
}

func (rd *renderer) onArtifact(a *a2a.Artifact, appendTo bool) {
	if a == nil {
		return
	}
	rd.mu.Lock()
	defer rd.mu.Unlock()
	var at *artifactText
	for _, x := range rd.artifacts {
		if x.id == a.ID {
			at = x
		}
	}
	if at == nil {
		at = &artifactText{id: a.ID}
		rd.artifacts = append(rd.artifacts, at)
	} else if !appendTo {
		at.text.Reset()
	}
	for _, p := range a.Parts {
		if t := p.Text(); t != "" {
			at.text.WriteString(t)
			continue
		}
		if raw := p.Raw(); len(raw) > 0 {
			name := p.Filename
			if name == "" {
				name = "file"
			}
			rd.files = append(rd.files, channel.File{Name: name, MediaType: p.MediaType, Data: raw})
		}
	}
	rd.progress = ""
}

func (rd *renderer) onStatus(ctx context.Context, st a2a.TaskStatus) bool {
	switch st.State {
	case a2a.TaskStateSubmitted:
		return false
	case a2a.TaskStateWorking:
		if st.Message != nil {
			if text := textOf(st.Message.Parts); text != "" {
				rd.mu.Lock()
				rd.progress = text
				rd.mu.Unlock()
				rd.flush(ctx, false)
			}
		}
		return false
	case a2a.TaskStateInputRequired:
		rd.flush(ctx, true)
		rd.question(ctx, st.Message)
		return true
	case a2a.TaskStateAuthRequired:
		rd.flush(ctx, true)
		rd.send(ctx, "The agent needs authorization: "+messageText(st.Message))
		return true
	case a2a.TaskStateCompleted, a2a.TaskStateFailed, a2a.TaskStateCanceled, a2a.TaskStateRejected:
		rd.final(ctx, st)
		return true
	}
	return false
}

func (rd *renderer) text() string {
	var sb strings.Builder
	for i, a := range rd.artifacts {
		if i > 0 && sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(a.text.String())
	}
	return sb.String()
}

// flush shows the current text. Unless force, edits are throttled and the
// latest text is shown when the interval passes.
func (rd *renderer) flush(ctx context.Context, force bool) {
	if !rd.caps.Stream {
		return
	}
	rd.mu.Lock()
	defer rd.mu.Unlock()
	if rd.done {
		return
	}
	if !force {
		if wait := editInterval - time.Since(rd.lastEdit); wait > 0 {
			if rd.timer == nil {
				rd.timer = time.AfterFunc(wait, func() {
					rd.mu.Lock()
					rd.timer = nil
					rd.mu.Unlock()
					rd.flush(ctx, false)
				})
			}
			return
		}
	}
	rd.showLocked(ctx, rd.progress)
}

// showLocked edits or sends the streaming message, starting a new message
// when the text outgrows the channel's limit.
func (rd *renderer) showLocked(ctx context.Context, progress string) {
	text := rd.text()
	limit := rd.caps.MaxText - 200
	for len(text)-rd.segStart > limit {
		seg := text[rd.segStart:]
		cut := strings.LastIndex(seg[:limit], "\n")
		if cut < limit/2 {
			cut = len(channel.Truncate(seg, limit))
		}
		rd.showSegmentLocked(ctx, seg[:cut])
		rd.segStart += cut
		rd.ref, rd.lastShown = channel.Ref{}, ""
	}
	display := text[rd.segStart:]
	if progress != "" {
		if display != "" {
			display += "\n\n"
		}
		display += "⚙ " + progress
	}
	if display != "" {
		rd.showSegmentLocked(ctx, display)
	}
}

func (rd *renderer) showSegmentLocked(ctx context.Context, display string) {
	if display == rd.lastShown {
		return
	}
	rd.lastEdit = time.Now()
	out := channel.Outbound{Conv: rd.conv, Text: display, ReplyTo: rd.replyTo}
	if rd.ref.ID != "" && rd.caps.Edit {
		if err := rd.ch.Edit(ctx, rd.ref, out); err != nil {
			slog.Debug("editing message", "channel", rd.ch.Name(), "error", err)
			return
		}
		rd.lastShown = display
		return
	}
	ref, err := rd.ch.Send(ctx, out)
	if err != nil {
		slog.Warn("sending message", "channel", rd.ch.Name(), "error", err)
		return
	}
	rd.ref, rd.lastShown = ref, display
}

// send sends a standalone message, split to the channel's limit.
func (rd *renderer) send(ctx context.Context, text string) channel.Ref {
	var ref channel.Ref
	for _, piece := range channel.Split(text, rd.caps.MaxText) {
		var err error
		ref, err = rd.ch.Send(ctx, channel.Outbound{Conv: rd.conv, Text: piece, ReplyTo: rd.replyTo, Subject: rd.subject()})
		if err != nil {
			slog.Warn("sending message", "channel", rd.ch.Name(), "error", err)
		}
	}
	return ref
}

func (rd *renderer) subject() string { return rd.r.s.config().Name }

func (rd *renderer) final(ctx context.Context, st a2a.TaskStatus) {
	rd.mu.Lock()
	if rd.timer != nil {
		rd.timer.Stop()
		rd.timer = nil
	}
	text := rd.text()
	status := messageText(st.Message)
	switch st.State {
	case a2a.TaskStateCompleted:
		if text == "" {
			text = status
		}
		if text == "" && len(rd.files) == 0 {
			text = "Done."
		}
	case a2a.TaskStateCanceled:
		text = joinText(text, "Stopped.")
	default:
		if status == "" {
			status = "The task failed."
		}
		text = joinText(text, "⚠ "+status)
	}
	files := rd.files
	streamed := rd.caps.Stream
	if streamed {
		rd.artifacts = []*artifactText{{id: "final"}}
		rd.artifacts[0].text.WriteString(text)
		rd.showLocked(ctx, "")
	}
	rd.done = true
	rd.mu.Unlock()

	if !streamed && text != "" {
		rd.send(ctx, text)
	}
	if len(files) > 0 {
		if _, err := rd.ch.Send(ctx, channel.Outbound{Conv: rd.conv, Files: files, ReplyTo: rd.replyTo, Subject: rd.subject()}); err != nil {
			slog.Warn("sending files", "channel", rd.ch.Name(), "error", err)
		}
	}
}

// question shows a question the agent asked.
func (rd *renderer) question(ctx context.Context, msg *a2a.Message) {
	rd.mu.Lock()
	taskID, contextID := rd.taskID, rd.contextID
	pending := ""
	if !rd.caps.Stream {
		pending = rd.text()
	}
	rd.done = true
	rd.mu.Unlock()
	if pending != "" {
		rd.send(ctx, pending)
	}
	if msg == nil {
		rd.send(ctx, "The agent needs more input.")
		return
	}
	q := questionOfMessage(msg)
	if q == nil || q.Type != questionPermission {
		rd.send(ctx, messageText(msg))
		return
	}
	if !rd.r.s.canApprove(ctx, rd.p) {
		rd.send(ctx, fmt.Sprintf("⏳ Waiting for the owner to approve: %s", q.Title))
		rd.r.follow(ctx, rd.ch, rd.conv, rd.p, taskID)
		return
	}
	rec, err := rd.r.s.newQuestionRecord(ctx, taskID, contextID, "", q)
	if err != nil {
		slog.Error("storing question", "error", err)
		rd.send(ctx, messageText(msg))
		return
	}
	out := channel.Outbound{Conv: rd.conv, Text: q.Text, ReplyTo: rd.replyTo, Subject: rd.subject()}
	if rd.caps.Buttons {
		out.Question = &channel.Question{ID: rec.ID, Options: q.labels()}
	} else {
		out.Text += fmt.Sprintf("\n\nReply %s, or open %s", optionWords(q), rd.r.s.hitlURL(rec))
	}
	ref, err := rd.ch.Send(ctx, out)
	if err != nil {
		slog.Warn("sending question", "channel", rd.ch.Name(), "error", err)
		return
	}
	rd.r.remember(rec.ID, rd.ch.Name(), ref, q.Text)
	if err := rd.r.s.store.Set(ctx, pendingKey(rd.ch.Name(), rd.conv), rec.ID); err != nil {
		slog.Warn("recording pending question", "error", err)
	}
}

func (rd *renderer) fail(ctx context.Context, err error) {
	rd.mu.Lock()
	rd.done = true
	rd.mu.Unlock()
	rd.send(ctx, "⚠ "+err.Error())
}

// finish handles a stream which ended without a final event: the task's
// stored state tells whether it finished or the stream was cut off.
func (rd *renderer) finish(ctx context.Context) {
	rd.mu.Lock()
	done, taskID := rd.done, rd.taskID
	rd.mu.Unlock()
	if done {
		return
	}
	interrupted := a2a.TaskStatus{State: a2a.TaskStateFailed, Message: a2a.NewMessage(a2a.MessageRoleAgent,
		a2a.NewTextPart("The task was interrupted before it finished."))}
	if taskID == "" {
		rd.final(ctx, interrupted)
		return
	}
	stored, err := rd.r.s.tasks.Get(auth.WithPrincipal(context.WithoutCancel(ctx), auth.Owner()), taskID)
	if err != nil {
		slog.Warn("loading the task of an ended stream", "task", taskID, "error", err)
		rd.final(ctx, interrupted)
		return
	}
	if !stored.Task.Status.State.Terminal() {
		rd.final(ctx, interrupted)
		return
	}
	rd.final(ctx, stored.Task.Status)
}

func joinText(a, b string) string {
	if a == "" {
		return b
	}
	return a + "\n\n" + b
}

func textOf(parts []*a2a.Part) string {
	var out []string
	for _, p := range parts {
		if t := p.Text(); t != "" {
			out = append(out, t)
		}
	}
	return strings.Join(out, "\n")
}

func messageText(m *a2a.Message) string {
	if m == nil {
		return ""
	}
	return textOf(m.Parts)
}
