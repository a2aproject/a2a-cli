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
	"crypto/subtle"
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"github.com/a2aproject/a2a-cli/internal/auth"
	"github.com/a2aproject/a2a-cli/internal/credstore"
	"github.com/a2aproject/a2a-cli/internal/localsrv"
	"github.com/a2aproject/a2a-cli/internal/store"
)

// harnessHost is the host of agents URLs when the harness reaches the server
// through a Unix socket; the CLI sends requests for it through the socket.
const (
	harnessHost = "a2a"
	socketHost  = "http://" + harnessHost
)

// harnessKeyAgent marks the harness key in the harness's a2a credentials.
const harnessKeyAgent = "a2a server (harness key)"

// saveHarnessKey stores the key the harness sends over TCP in the a2a
// credentials file of the harness home, where the CLI attaches it to
// requests for agentsBase only. Keys of earlier runs are removed.
func saveHarnessKey(home, agentsBase, key string) error {
	cs := &credstore.Store{Path: filepath.Join(home, ".config", "a2a-cli", "credentials.json")}
	creds, err := cs.List()
	if err != nil {
		return err
	}
	for _, c := range creds {
		if c.Agent != harnessKeyAgent {
			continue
		}
		if _, err := cs.Delete(c.URL); err != nil {
			return err
		}
	}
	return cs.Put(credstore.Credential{URL: agentsBase, Token: key, Agent: harnessKeyAgent, Created: time.Now().UTC()})
}

// agentsURL is where the harness calls other agents in one conversation:
// <agents URL>/<name> is an A2A agent. The handle in the path stands for the
// conversation, the user and the hop count.
func (p *Peers) agentsURL(ctx context.Context, c callClaims) (string, error) {
	p.s.urlMu.Lock()
	base, socket := p.s.harnessURL, p.s.harnessSocket
	p.s.urlMu.Unlock()
	if socket != "" {
		base = socketHost
	}
	if base == "" {
		return "", errors.New("the harness listener is not running")
	}
	h, err := p.handle(ctx, c)
	if err != nil {
		return "", err
	}
	return base + "/" + h, nil
}

// agentsHandler serves the agents the harness may call, at
// /{handle}/{agent}/..., as A2A agents proxied by this server: contacts are
// called with this agent's identity, nodes in-process. Over a Unix socket
// only the harness can connect; over TCP it sends the harness key.
func (p *Peers) agentsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.s.urlMu.Lock()
		key := p.s.harnessKey
		p.s.urlMu.Unlock()
		if key != "" {
			tok, ok := auth.BearerToken(r.Header.Get("Authorization"))
			if !ok || subtle.ConstantTimeCompare([]byte(tok), []byte(key)) != 1 {
				writeError(w, http.StatusUnauthorized, "harness key required")
				return
			}
		}
		handle, name := r.PathValue("handle"), r.PathValue("agent")
		claims, ok := p.claimsOf(r.Context(), handle)
		if !ok {
			writeError(w, http.StatusNotFound, "unknown agents URL: use the one from the latest [a2a agents] line")
			return
		}
		if claims.Hops+1 > auth.MaxHops {
			writeError(w, http.StatusForbidden, fmt.Sprintf("too many agents in this chain (%d); answer without calling another agent", claims.Hops))
			return
		}
		rest := "/" + r.PathValue("rest")
		base := "http://" + r.Host + "/" + handle + "/" + name
		if !p.callable(r.Context(), name) {
			writeError(w, http.StatusNotFound, fmt.Sprintf("%q is not an agent you can call; the [a2a agents] line lists them", name))
			return
		}
		if r.Method == http.MethodGet && (rest == "/" || rest == "/.well-known/agent-card.json") {
			a2asrv.NewStaticAgentCardHandler(proxyCard(name, base)).ServeHTTP(w, r)
			return
		}
		h, principal, closeFn, err := p.agentHandler(r.Context(), claims, name)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		defer closeFn()
		guarded := &guardedHandler{RequestHandler: h, p: p, claims: claims, agent: name}
		r2 := r.Clone(r.Context())
		r2.URL.Path, r2.URL.RawPath = rest, ""
		if principal != nil {
			r2 = r2.WithContext(auth.WithPrincipal(r2.Context(), *principal))
		}
		var inner http.Handler = a2asrv.NewRESTHandler(guarded, a2asrv.WithTransportKeepAlive(15*time.Second))
		if r.Method == http.MethodPost && rest == "/" {
			inner = a2asrv.NewJSONRPCHandler(guarded, a2asrv.WithTransportKeepAlive(15*time.Second))
		}
		inner.ServeHTTP(w, r2)
	})
}

// callable reports whether name is a contact or a hosted node.
func (p *Peers) callable(ctx context.Context, name string) bool {
	if _, err := p.s.store.Node(ctx, name); err == nil {
		return true
	}
	_, err := p.s.store.Contact(ctx, name)
	return err == nil
}

func proxyCard(name, base string) *a2a.AgentCard {
	return &a2a.AgentCard{
		Name:        name,
		Description: name + ", called through the agent server hosting you.",
		Version:     "1.0.0",
		SupportedInterfaces: []*a2a.AgentInterface{
			a2a.NewAgentInterface(base, a2a.TransportProtocolJSONRPC),
			a2a.NewAgentInterface(base, a2a.TransportProtocolHTTPJSON),
		},
		Capabilities:       a2a.AgentCapabilities{Streaming: true},
		DefaultInputModes:  []string{"text/plain", "application/json"},
		DefaultOutputModes: []string{"text/plain", "application/json"},
	}
}

// agentHandler returns the A2A handler of a node or a contact. A node is
// served in-process, as principal.
func (p *Peers) agentHandler(ctx context.Context, c callClaims, name string) (a2asrv.RequestHandler, *auth.Principal, func(), error) {
	if _, err := p.s.store.Node(ctx, name); err == nil {
		pr := p.nodeCaller(c.OnBehalfOf, c.Hops)
		return p.s.nodes.handler(name), &pr, func() {}, nil
	}
	client, err := p.contactClient(ctx, name, c.OnBehalfOf, c.Hops)
	if err != nil {
		return nil, nil, nil, err
	}
	return &localsrv.ClientHandler{Client: client}, nil, func() {
		if err := client.Destroy(); err != nil {
			slog.Debug("closing a2a client", "error", err)
		}
	}, nil
}

// guardedHandler keeps approvals with the user: the harness can't answer
// another agent's permission question, which is relayed to the user's chat
// instead. Tasks the harness doesn't wait for are watched, and their result
// is posted into the conversation.
type guardedHandler struct {
	a2asrv.RequestHandler
	p      *Peers
	claims callClaims
	agent  string
}

func (g *guardedHandler) SendMessage(ctx context.Context, req *a2a.SendMessageRequest) (a2a.SendMessageResult, error) {
	if err := g.checkAnswer(ctx, req); err != nil {
		return nil, err
	}
	res, err := g.RequestHandler.SendMessage(ctx, req)
	if task, ok := res.(*a2a.Task); ok && err == nil {
		g.after(ctx, task)
	}
	return res, err
}

func (g *guardedHandler) SendStreamingMessage(ctx context.Context, req *a2a.SendMessageRequest) iter.Seq2[a2a.Event, error] {
	if err := g.checkAnswer(ctx, req); err != nil {
		return func(yield func(a2a.Event, error) bool) { yield(nil, err) }
	}
	return func(yield func(a2a.Event, error) bool) {
		var last *a2a.Task
		for ev, err := range g.RequestHandler.SendStreamingMessage(ctx, req) {
			switch e := ev.(type) {
			case *a2a.Task:
				last = &a2a.Task{ID: e.ID, ContextID: e.ContextID, Status: e.Status}
			case *a2a.TaskStatusUpdateEvent:
				last = &a2a.Task{ID: e.TaskID, ContextID: e.ContextID, Status: e.Status}
			}
			if !yield(ev, err) {
				break
			}
		}
		if last != nil {
			g.after(ctx, last)
		}
	}
}

// checkAnswer rejects a reply to a pending permission question.
func (g *guardedHandler) checkAnswer(ctx context.Context, req *a2a.SendMessageRequest) error {
	if req.Message == nil || req.Message.TaskID == "" {
		return nil
	}
	task, err := g.RequestHandler.GetTask(ctx, &a2a.GetTaskRequest{ID: req.Message.TaskID})
	if err != nil || task.Status.State != a2a.TaskStateInputRequired {
		return nil
	}
	if q := questionOfStatus(task.Status); q != nil && q.Type == questionPermission {
		g.after(ctx, task)
		return fmt.Errorf("%w: this is an approval for the user, who was asked in their chat; don't answer it, the result will be posted into this conversation", a2a.ErrUnauthorized)
	}
	return nil
}

// after relays a permission question to the user and watches a task the
// harness won't see finish: one waiting for an approval, or one it didn't wait for.
func (g *guardedHandler) after(ctx context.Context, task *a2a.Task) {
	state := task.Status.State
	if state.Terminal() || state == a2a.TaskStateAuthRequired {
		return
	}
	if state == a2a.TaskStateInputRequired {
		q := questionOfStatus(task.Status)
		if q == nil || q.Type != questionPermission {
			return
		}
		g.p.relay(ctx, g.claims, g.agent, task, q)
	}
	w := store.Watch{Contact: g.agent, TaskID: string(task.ID), RemoteContextID: task.ContextID, OriginContextID: g.claims.ContextID, OnBehalfOf: g.claims.OnBehalfOf}
	if err := g.p.s.store.PutWatch(ctx, w); err != nil {
		slog.Error("storing watch", "agent", g.agent, "task", task.ID, "error", err)
		return
	}
	g.p.watch(context.WithoutCancel(ctx), w)
}
