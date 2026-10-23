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

package approvals

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"maps"
	"net/http"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"github.com/a2aproject/a2a-cli/internal/auth"
	"github.com/a2aproject/a2a-cli/internal/channel"
	"github.com/a2aproject/a2a-cli/internal/cryptoutil"
	"github.com/a2aproject/a2a-cli/internal/store"
)

// Notifier shows questions to people and updates them once answered.
type Notifier interface {
	// Ask sends a question to users on their direct conversations, or to
	// one preferred conversation if given.
	Ask(ctx context.Context, userIDs []string, prefChannel string, prefConv channel.ConvKey, header string, rec store.Question)
	// MarkAnswered shows who answered a question where it was asked.
	MarkAnswered(ctx context.Context, questionID, label, who string)
}

// RemoteAnswerer forwards answers to the questions of other agents' tasks.
type RemoteAnswerer interface {
	// AnswerRemote sends by's answer to the task of the agent rec.Contact.
	AnswerRemote(ctx context.Context, rec store.Question, q *Question, opt Option, by auth.Principal) error
}

// Service handles the questions of the agent and of agents it calls. Its
// fields are set once, before the server serves.
type Service struct {
	Store *store.Store
	// Tasks is the agent's task store.
	Tasks *store.TaskStore
	// Handler is the agent's A2A handler, where answers to its own
	// questions are sent.
	Handler  a2asrv.RequestHandler
	Notifier Notifier
	Remote   RemoteAnswerer
	// Approvers returns the users who may approve besides owners.
	Approvers func() []string
	// BaseURL returns the URL people reach the agent at.
	BaseURL func() string
	// AgentName returns the name of the agent.
	AgentName func() string
}

// ErrNotApprover is returned when someone who may not approve answers an approval.
var ErrNotApprover = errors.New("only the owner or an approver can answer this")

// NewRecord stores a question so that buttons and pages can answer it.
// contact is the agent whose task asked, or empty for this agent. It
// returns [store.ErrExists] for a remote question stored before.
func (s *Service) NewRecord(ctx context.Context, taskID a2a.TaskID, contextID, contact string, q *Question) (store.Question, error) {
	rec := store.Question{
		ID:        cryptoutil.RandomString(8, cryptoutil.Alphanumeric),
		Token:     cryptoutil.RandomString(32, cryptoutil.Alphanumeric),
		TaskID:    string(taskID),
		ContextID: contextID,
		Contact:   contact,
		HitlID:    q.ID,
		Data:      q.Data,
	}
	if q.Text != "" {
		rec.Data = maps.Clone(q.Data)
		rec.Data["text"] = q.Text
	}
	return rec, s.Store.PutQuestion(ctx, rec)
}

// CanApprove reports whether p may answer permission requests.
func (s *Service) CanApprove(ctx context.Context, p auth.Principal) bool {
	if auth.CanApprove(p, s.Approvers()) {
		return true
	}
	u, err := s.Store.User(ctx, p.ID)
	return err == nil && auth.CanApprove(auth.Principal{ID: u.ID, Roles: u.Roles}, nil)
}

// approverIDs are the users who receive approval requests.
func (s *Service) approverIDs() []string {
	return append([]string{auth.OwnerID}, s.Approvers()...)
}

// PageURL is the page where an approver can answer a question.
func (s *Service) PageURL(rec store.Question) string {
	return s.BaseURL() + "/hitl/" + rec.Token
}

// OnQuestion relays a permission question of the agent to the approvers
// when the caller who started the task can't answer it.
func (s *Service) OnQuestion(ctx context.Context, execCtx *a2asrv.ExecutorContext, q *Question) {
	if q.Type != TypePermission {
		return
	}
	ctx = context.WithoutCancel(ctx)
	stored, err := s.Tasks.Get(auth.WithPrincipal(ctx, auth.Owner()), execCtx.TaskID)
	starter := ""
	if err == nil {
		starter = stored.User
	} else if p, ok := auth.Caller(ctx); ok {
		starter = p.ID
	}
	if starter != "" && s.CanApprove(ctx, auth.Principal{ID: starter}) {
		return
	}
	go func() {
		rec, err := s.NewRecord(ctx, execCtx.TaskID, execCtx.ContextID, "", q)
		if err != nil {
			slog.Error("storing question", "error", err)
			return
		}
		slog.Info("approval needed", "task", execCtx.TaskID, "from", starter, "question", rec.ID)
		header := fmt.Sprintf("%s needs your approval", DisplayPrincipal(starter))
		s.Notifier.Ask(ctx, s.approverIDs(), "", "", header, rec)
	}()
}

// CheckAnswer rejects a reply to the agent's pending permission question
// from a caller who may not approve.
func (s *Service) CheckAnswer(ctx context.Context, p auth.Principal, taskID a2a.TaskID) error {
	stored, err := s.Tasks.Get(auth.WithPrincipal(ctx, auth.Owner()), taskID)
	if errors.Is(err, a2a.ErrTaskNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("checking task %s: %w", taskID, err)
	}
	task := stored.Task
	if task.Status.State != a2a.TaskStateInputRequired {
		return nil
	}
	q := FromStatus(task.Status)
	if q == nil || q.Type != TypePermission {
		return nil
	}
	if s.CanApprove(ctx, p) {
		return nil
	}
	return fmt.Errorf("%w: only the owner or an approver can answer this approval; it was sent to them", a2a.ErrUnauthorized)
}

// Answer answers a stored question with option on behalf of p. Questions of
// remote agents are forwarded to them.
func (s *Service) Answer(ctx context.Context, rec store.Question, p auth.Principal, option Option) error {
	q := FromRecord(rec)
	if q.Type == TypePermission && !s.CanApprove(ctx, p) {
		return ErrNotApprover
	}
	if err := s.Store.AnswerQuestion(ctx, rec.ID, p.ID, option.ID); err != nil {
		return err
	}
	if rec.Contact != "" {
		return s.Remote.AnswerRemote(ctx, rec, q, option, p)
	}
	msg := AnswerMessage(a2a.TaskID(rec.TaskID), rec.ContextID, q.ID, option.ID)
	_, err := s.Handler.SendMessage(auth.InProcess(ctx, p), &a2a.SendMessageRequest{
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

// HandlePage serves the page of a question at /hitl/{token}. Its token is the credential:
// it is sent only to approvers, who answer as the owner.
func (s *Service) HandlePage(w http.ResponseWriter, r *http.Request) {
	rec, err := s.Store.QuestionByToken(r.Context(), r.PathValue("token"))
	if err != nil {
		http.Error(w, "This question does not exist.", http.StatusNotFound)
		return
	}
	q := FromRecord(rec)
	if r.Method == http.MethodPost && rec.AnsweredBy == "" {
		opt, ok := q.Match(r.FormValue("option"))
		if !ok {
			http.Error(w, "Unknown option.", http.StatusBadRequest)
			return
		}
		if err := s.Answer(r.Context(), rec, auth.Owner(), opt); err != nil && !errors.Is(err, store.ErrAnswered) {
			slog.Error("answering question from the web", "question", rec.ID, "error", err)
			http.Error(w, "Couldn't send the answer. Try again.", http.StatusInternalServerError)
			return
		}
		s.Notifier.MarkAnswered(r.Context(), rec.ID, opt.Label, "web")
		http.Redirect(w, r, r.URL.Path, http.StatusSeeOther)
		return
	}
	if rec, err = s.Store.QuestionByToken(r.Context(), r.PathValue("token")); err != nil {
		slog.Error("reloading question", "error", err)
		http.Error(w, "Couldn't load the question. Try again.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := hitlPage.Execute(w, map[string]any{
		"Agent": s.AgentName(), "Text": q.Display(), "Options": q.Options,
		"Answered": rec.Answer, "By": DisplayPrincipal(rec.AnsweredBy),
	}); err != nil {
		slog.Warn("rendering hitl page", "error", err)
	}
}
