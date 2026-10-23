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
	"html/template"
	"log/slog"
	"maps"
	"net/http"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"github.com/a2aproject/a2a-cli/internal/auth"
	"github.com/a2aproject/a2a-cli/internal/store"
)

// Question types of the hitl data part.
const (
	questionPermission = "permission"
	questionForm       = "form"
)

// hitlKey is the key of the data part describing a question in an
// input-required status message.
const hitlKey = "hitl"

// hitlQuestion is a question from an input-required status message.
type hitlQuestion struct {
	ID      string
	Type    string
	Title   string
	Options []hitlOption
	// Text is the text of the status message.
	Text string
	// Data is the raw hitl data part.
	Data map[string]any
}

type hitlOption struct {
	ID    string
	Label string
}

func questionOf(ev a2a.Event) *hitlQuestion {
	su, ok := ev.(*a2a.TaskStatusUpdateEvent)
	if !ok {
		return nil
	}
	return questionOfStatus(su.Status)
}

func questionOfStatus(st a2a.TaskStatus) *hitlQuestion {
	if st.State != a2a.TaskStateInputRequired || st.Message == nil {
		return nil
	}
	return questionOfMessage(st.Message)
}

func questionOfMessage(msg *a2a.Message) *hitlQuestion {
	var q *hitlQuestion
	var texts []string
	for _, p := range msg.Parts {
		if t := p.Text(); t != "" {
			texts = append(texts, t)
		}
		m, ok := p.Data().(map[string]any)
		if !ok {
			continue
		}
		if data, ok := m[hitlKey].(map[string]any); ok {
			q = parseHitl(data)
		}
	}
	if q != nil {
		q.Text = strings.Join(texts, "\n")
	}
	return q
}

func parseHitl(data map[string]any) *hitlQuestion {
	q := &hitlQuestion{Data: data}
	q.ID, _ = data["id"].(string)
	q.Type, _ = data["type"].(string)
	q.Title, _ = data["title"].(string)
	opts, _ := data["options"].([]any)
	for _, o := range opts {
		m, _ := o.(map[string]any)
		id, _ := m["id"].(string)
		label, _ := m["label"].(string)
		if label == "" {
			label = id
		}
		if id != "" {
			q.Options = append(q.Options, hitlOption{ID: id, Label: label})
		}
	}
	return q
}

// labels returns the button labels of the question.
func (q *hitlQuestion) labels() []string {
	out := make([]string, len(q.Options))
	for i, o := range q.Options {
		out[i] = o.Label
	}
	return out
}

// matchOption maps a typed reply to an option, accepting option IDs and
// labels and the words of an email approval.
func (q *hitlQuestion) matchOption(text string) (hitlOption, bool) {
	reply := strings.ToLower(strings.Trim(strings.TrimSpace(text), ".!\"'`"))
	for _, o := range q.Options {
		if reply == strings.ToLower(o.ID) || reply == strings.ToLower(o.Label) {
			return o, true
		}
	}
	var id string
	switch reply {
	case "approve", "approved", "yes", "y", "ok", "allow":
		id = "once"
	case "always", "always allow":
		id = "always"
	case "deny", "denied", "no", "n", "reject":
		id = "reject"
	}
	for _, o := range q.Options {
		if o.ID == id {
			return o, true
		}
	}
	return hitlOption{}, false
}

// answerMessage is the reply which selects an option.
func answerMessage(taskID a2a.TaskID, contextID, questionID, option string) *a2a.Message {
	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewDataPart(map[string]any{
		hitlKey: map[string]any{"id": questionID, "answer": option},
	}))
	msg.TaskID, msg.ContextID = taskID, contextID
	return msg
}

// newQuestionRecord stores a question so that buttons and pages can answer it.
func (s *Server) newQuestionRecord(ctx context.Context, taskID a2a.TaskID, contextID, contact string, q *hitlQuestion) (store.Question, error) {
	rec := store.Question{
		ID:        store.RandomString(8, store.Alphanumeric),
		Token:     store.RandomString(32, store.Alphanumeric),
		TaskID:    string(taskID),
		ContextID: contextID,
		Contact:   contact,
		Data:      q.Data,
	}
	if q.Text != "" {
		rec.Data = maps.Clone(q.Data)
		rec.Data["text"] = q.Text
	}
	return rec, s.store.PutQuestion(ctx, rec)
}

// questionFromRecord rebuilds the question of a stored record.
func questionFromRecord(rec store.Question) *hitlQuestion {
	q := parseHitl(rec.Data)
	q.Text, _ = rec.Data["text"].(string)
	return q
}

// hitlURL is the page where an approver can answer a question.
func (s *Server) hitlURL(rec store.Question) string {
	return s.bestURL() + "/hitl/" + rec.Token
}

// onQuestion relays a permission question to the approvers when the caller
// who started the task can't answer it.
func (s *Server) onQuestion(ctx context.Context, execCtx *a2asrv.ExecutorContext, q *hitlQuestion) {
	if q.Type != questionPermission {
		return
	}
	ctx = context.WithoutCancel(ctx)
	stored, err := s.tasks.Get(auth.WithPrincipal(ctx, auth.Owner()), execCtx.TaskID)
	starter := ""
	if err == nil {
		starter = stored.User
	} else if p, ok := principalOf(ctx); ok {
		starter = p.ID
	}
	if starter != "" && s.canApprove(ctx, auth.Principal{ID: starter}) {
		return
	}
	go func() {
		rec, err := s.newQuestionRecord(ctx, execCtx.TaskID, execCtx.ContextID, "", q)
		if err != nil {
			slog.Error("storing question", "error", err)
			return
		}
		slog.Info("approval needed", "task", execCtx.TaskID, "from", starter, "page", s.hitlURL(rec))
		header := fmt.Sprintf("%s needs your approval", displayPrincipal(starter))
		if s.router != nil {
			s.router.ask(ctx, s.approverIDs(), "", "", header, rec)
		}
	}()
}

// approverIDs are the users who receive approval requests.
func (s *Server) approverIDs() []string {
	return append([]string{auth.OwnerID}, s.config().Approvals.Approvers...)
}

func displayPrincipal(id string) string {
	if id == "" {
		return "A caller"
	}
	_, name, ok := strings.Cut(id, ":")
	if !ok {
		return id
	}
	return name
}

// ErrNotApprover is returned when someone who may not approve answers an approval.
var ErrNotApprover = errors.New("only the owner or an approver can answer this")

// answer answers a stored question with option on behalf of p. Questions of
// remote agents are forwarded to them.
func (s *Server) answer(ctx context.Context, rec store.Question, p auth.Principal, option hitlOption) error {
	q := questionFromRecord(rec)
	if q.Type == questionPermission && !s.canApprove(ctx, p) {
		return ErrNotApprover
	}
	if err := s.store.AnswerQuestion(ctx, rec.ID, p.ID, option.ID); err != nil {
		return err
	}
	if rec.Contact != "" {
		return s.peers.answerRemote(ctx, rec, q, option, p)
	}
	msg := answerMessage(a2a.TaskID(rec.TaskID), rec.ContextID, q.ID, option.ID)
	_, err := s.handler.SendMessage(inProcess(ctx, p), &a2a.SendMessageRequest{
		Message: msg,
		Config:  &a2a.SendMessageConfig{ReturnImmediately: true},
	})
	return err
}

var hitlPage = template.Must(template.New("hitl").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Agent}}: approval</title>
<style>body{font-family:system-ui,sans-serif;max-width:40rem;margin:2rem auto;padding:0 1rem}
pre{white-space:pre-wrap;background:#f4f4f4;padding:1rem;border-radius:.5rem}
button{font-size:1rem;margin:.25rem;padding:.5rem 1rem}</style></head>
<body><h2>{{.Agent}}</h2>
{{if .Answered}}<p>Answered: <b>{{.Answered}}</b> by {{.By}}.</p>{{else}}
<pre>{{.Text}}</pre>
<form method="post">{{range .Options}}<button name="option" value="{{.ID}}">{{.Label}}</button>{{end}}</form>{{end}}
</body></html>`))

// handleHitl serves the page of a question. Its token is the credential:
// it is sent only to approvers, who answer as the owner.
func (s *Server) handleHitl(w http.ResponseWriter, r *http.Request) {
	rec, err := s.store.QuestionByToken(r.Context(), r.PathValue("token"))
	if err != nil {
		http.Error(w, "This question does not exist.", http.StatusNotFound)
		return
	}
	q := questionFromRecord(rec)
	if r.Method == http.MethodPost && rec.AnsweredBy == "" {
		opt, ok := q.matchOption(r.FormValue("option"))
		if !ok {
			http.Error(w, "Unknown option.", http.StatusBadRequest)
			return
		}
		if err := s.answer(r.Context(), rec, auth.Owner(), opt); err != nil && !errors.Is(err, store.ErrAnswered) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if s.router != nil {
			s.router.markAnswered(r.Context(), rec.ID, opt.Label, "web")
		}
		http.Redirect(w, r, r.URL.Path, http.StatusSeeOther)
		return
	}
	if rec, err = s.store.QuestionByToken(r.Context(), r.PathValue("token")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	text := q.Text
	if text == "" {
		text = q.Title
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := hitlPage.Execute(w, map[string]any{
		"Agent": s.config().Name, "Text": text, "Options": q.Options,
		"Answered": rec.Answer, "By": displayPrincipal(rec.AnsweredBy),
	}); err != nil {
		slog.Warn("rendering hitl page", "error", err)
	}
}
