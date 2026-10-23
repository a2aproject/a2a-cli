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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"github.com/a2aproject/a2a-cli/internal/auth"
	"github.com/a2aproject/a2a-cli/internal/channel"
	"github.com/a2aproject/a2a-cli/internal/network"
	"github.com/a2aproject/a2a-cli/internal/store"
)

// Contact routes.
const (
	ViaPrivate = "private"
	ViaHTTPS   = "https"
)

// watchInterval is the first delay between checks of a watched task; it
// grows to watchMaxInterval.
var (
	watchInterval    = 5 * time.Second
	watchMaxInterval = 30 * time.Second
)

// Peers handles calls from this agent to other agents: contacts, agents on
// the private network and webhook nodes hosted here. The harness reaches
// them with the a2a CLI through the agents URL of its conversation.
type Peers struct {
	s *Server

	handleMu sync.Mutex

	mu       sync.Mutex
	relayed  map[string]bool
	watching map[string]bool
	cache    []network.PeerInfo
	cachedAt time.Time
}

func newPeers(s *Server) *Peers {
	return &Peers{s: s, relayed: map[string]bool{}, watching: map[string]bool{}}
}

// callClaims is what an agents URL stands for: calls from the harness in one
// context, on behalf of the user who started the turn.
type callClaims struct {
	ContextID  string `json:"c"`
	OnBehalfOf string `json:"u"`
	Hops       int    `json:"h,omitempty"`
}

// handleAlphabet avoids characters that are easy to confuse.
const handleAlphabet = "abcdefghijkmnpqrstuvwxyz23456789"

// handle returns the short handle of the agents URL for c. The same claims
// keep their handle, also across restarts. A handle is not a secret: only
// the harness reaches the listener it is used on.
func (p *Peers) handle(ctx context.Context, c callClaims) (string, error) {
	data, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	byClaims := "handle-of:" + string(data)
	p.handleMu.Lock()
	defer p.handleMu.Unlock()
	if h, ok, err := p.s.store.Get(ctx, byClaims); err != nil || ok {
		return h, err
	}
	for n := 2; ; n++ {
		for range 8 {
			h := store.RandomString(n, handleAlphabet)
			_, taken, err := p.s.store.Get(ctx, "handle:"+h)
			if err != nil {
				return "", err
			}
			if taken {
				continue
			}
			if err := p.s.store.Set(ctx, "handle:"+h, string(data)); err != nil {
				return "", err
			}
			return h, p.s.store.Set(ctx, byClaims, h)
		}
	}
}

// claimsOf returns the claims of a handle.
func (p *Peers) claimsOf(ctx context.Context, h string) (callClaims, bool) {
	v, ok, err := p.s.store.Get(ctx, "handle:"+h)
	if err != nil || !ok {
		return callClaims{}, false
	}
	var c callClaims
	return c, json.Unmarshal([]byte(v), &c) == nil
}

// discover lists agents on the private network, cached briefly.
func (p *Peers) discover(ctx context.Context) ([]network.PeerInfo, error) {
	if p.s.private == nil {
		return nil, errors.New("this agent has no private network; start it with --private tailscale or --private lan")
	}
	p.mu.Lock()
	if time.Since(p.cachedAt) < 30*time.Second {
		out := p.cache
		p.mu.Unlock()
		return out, nil
	}
	p.mu.Unlock()
	peers, err := p.s.private.Peers(ctx)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	p.cache, p.cachedAt = peers, time.Now()
	p.mu.Unlock()
	return peers, nil
}

// addContact stores a contact, pairing with it if a code is given.
func (p *Peers) addContact(ctx context.Context, req ContactRequest) (store.Contact, error) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || strings.ContainsAny(req.Name, " /") {
		return store.Contact{}, errors.New("contact name must be non-empty, without spaces or slashes")
	}
	c := store.Contact{Name: req.Name, URL: trimSlash(req.URL), Token: req.Token, Via: ViaHTTPS}
	if c.URL == "" {
		peers, err := p.discover(ctx)
		if err != nil {
			return store.Contact{}, err
		}
		for _, pi := range peers {
			if strings.EqualFold(pi.Name, req.Name) {
				c.URL = pi.URL
			}
		}
		if c.URL == "" {
			return store.Contact{}, fmt.Errorf("no agent named %q on the private network; pass its URL with --url", req.Name)
		}
		c.Via = ViaPrivate
	} else if p.s.private != nil && p.onPrivateNetwork(ctx, c.URL) {
		c.Via = ViaPrivate
	}
	hc := p.httpClient(c, "", 0)
	if req.Code != "" {
		tok, err := pairWith(ctx, hc, c.URL, req.Code)
		if err != nil {
			return store.Contact{}, err
		}
		c.Token = tok
		hc = p.httpClient(c, "", 0)
	}
	if _, err := network.FetchCard(ctx, hc, c.URL); err != nil {
		return store.Contact{}, fmt.Errorf("can't reach %s: %w", c.URL, err)
	}
	return c, p.s.store.PutContact(ctx, c)
}

func (p *Peers) onPrivateNetwork(ctx context.Context, u string) bool {
	peers, err := p.discover(ctx)
	if err != nil {
		return false
	}
	for _, pi := range peers {
		if trimSlash(pi.URL) == trimSlash(u) {
			return true
		}
	}
	return false
}

// pairWith exchanges a pairing code at an agent for a token.
func pairWith(ctx context.Context, hc *http.Client, agentURL, code string) (string, error) {
	body, err := json.Marshal(PairRequest{Code: code})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, trimSlash(agentURL)+"/pair", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Debug("closing pair response", "error", err)
		}
	}()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		var apiErr APIError
		if json.Unmarshal(data, &apiErr) == nil && apiErr.Error != "" {
			return "", fmt.Errorf("pairing with %s: %s", agentURL, apiErr.Error)
		}
		return "", fmt.Errorf("pairing with %s: %s", agentURL, resp.Status)
	}
	var out PairResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	if out.Kind == PairKindNode {
		return "", fmt.Errorf("pairing with %s: the code is for a webhook node, not a caller", agentURL)
	}
	return out.Token, nil
}

// headerTransport adds the caller's headers to every request.
type headerTransport struct {
	base   http.RoundTripper
	header http.Header
}

func (t *headerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	maps.Copy(r.Header, t.header)
	return t.base.RoundTrip(r)
}

// httpClient returns the client for calls to a contact on behalf of a user.
func (p *Peers) httpClient(c store.Contact, onBehalfOf string, hops int) *http.Client {
	base := http.DefaultClient
	if c.Via == ViaPrivate && p.s.private != nil {
		base = p.s.private.HTTPClient()
	}
	rt := base.Transport
	if rt == nil {
		rt = http.DefaultTransport
	}
	h := http.Header{}
	if c.Token != "" {
		h.Set("Authorization", "Bearer "+c.Token)
	}
	if onBehalfOf != "" {
		h.Set(auth.HeaderOnBehalfOf, onBehalfOf)
	}
	h.Set(auth.HeaderHops, strconv.Itoa(hops+1))
	return &http.Client{Transport: &headerTransport{base: rt, header: h}, Timeout: 30 * time.Second}
}

// target is an agent a2a.send talks to.
type target interface {
	send(ctx context.Context, msg *a2a.Message) (a2a.SendMessageResult, error)
	get(ctx context.Context, id a2a.TaskID) (*a2a.Task, error)
	close()
}

type remoteTarget struct{ c *a2aclient.Client }

func (t remoteTarget) send(ctx context.Context, msg *a2a.Message) (a2a.SendMessageResult, error) {
	return t.c.SendMessage(ctx, &a2a.SendMessageRequest{Message: msg, Config: &a2a.SendMessageConfig{ReturnImmediately: true}})
}

func (t remoteTarget) get(ctx context.Context, id a2a.TaskID) (*a2a.Task, error) {
	return t.c.GetTask(ctx, &a2a.GetTaskRequest{ID: id})
}

func (t remoteTarget) close() {
	if err := t.c.Destroy(); err != nil {
		slog.Debug("closing a2a client", "error", err)
	}
}

type localTarget struct {
	h a2asrv.RequestHandler
	p auth.Principal
}

func (t localTarget) send(ctx context.Context, msg *a2a.Message) (a2a.SendMessageResult, error) {
	return t.h.SendMessage(inProcess(ctx, t.p), &a2a.SendMessageRequest{Message: msg, Config: &a2a.SendMessageConfig{ReturnImmediately: true}})
}

func (t localTarget) get(ctx context.Context, id a2a.TaskID) (*a2a.Task, error) {
	return t.h.GetTask(inProcess(ctx, t.p), &a2a.GetTaskRequest{ID: id})
}

func (t localTarget) close() {}

// ErrOffline is returned when a contact can't be reached.
var ErrOffline = errors.New("offline")

// target resolves an agent name to a contact or a hosted node.
func (p *Peers) target(ctx context.Context, name, onBehalfOf string, hops int) (target, error) {
	if _, err := p.s.store.Node(ctx, name); err == nil {
		return localTarget{h: p.s.nodes.handler(name), p: p.nodeCaller(onBehalfOf, hops)}, nil
	}
	client, err := p.contactClient(ctx, name, onBehalfOf, hops)
	if err != nil {
		return nil, err
	}
	return remoteTarget{c: client}, nil
}

// nodeCaller is the principal this agent calls its nodes as.
func (p *Peers) nodeCaller(onBehalfOf string, hops int) auth.Principal {
	return auth.Principal{ID: "agent:" + p.s.config().Name, Roles: []string{auth.RoleOwner}, OnBehalfOf: onBehalfOf, Hops: hops + 1}
}

// contactClient connects to a contact on behalf of a user. It returns
// [store.ErrNotFound] for a name which isn't a contact.
func (p *Peers) contactClient(ctx context.Context, name, onBehalfOf string, hops int) (*a2aclient.Client, error) {
	c, err := p.s.store.Contact(ctx, name)
	if err != nil {
		return nil, err
	}
	hc := p.httpClient(c, onBehalfOf, hops)
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	card, err := agentcard.NewResolver(hc).Resolve(cctx, c.URL)
	if err != nil {
		return nil, fmt.Errorf("%s is %w (%v)", name, ErrOffline, err)
	}
	client, err := a2aclient.NewFromCard(ctx, card, a2aclient.WithJSONRPCTransport(hc), a2aclient.WithRESTTransport(hc))
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", name, err)
	}
	return client, nil
}

// relay asks the user on whose behalf a remote task runs to answer its
// permission question, once per question.
func (p *Peers) relay(ctx context.Context, claims callClaims, agent string, task *a2a.Task, q *hitlQuestion) {
	key := agent + "|" + string(task.ID) + "|" + q.ID
	p.mu.Lock()
	seen := p.relayed[key]
	p.relayed[key] = true
	p.mu.Unlock()
	if seen || p.s.router == nil {
		return
	}
	rec, err := p.s.newQuestionRecord(ctx, task.ID, task.ContextID, agent, q)
	if err != nil {
		slog.Error("storing remote question", "error", err)
		return
	}
	chName, conv, _ := p.s.store.ConvOf(ctx, claims.ContextID)
	user := claims.OnBehalfOf
	if user == "" {
		user = auth.OwnerID
	}
	header := fmt.Sprintf("%s wants to", agent)
	if q.Title != "" {
		header = fmt.Sprintf("%s wants to run %s", agent, q.Title)
	}
	p.s.router.ask(ctx, []string{user}, chName, channel.ConvKey(conv), header, rec)
}

// answerRemote forwards an answer to a remote agent's question.
func (p *Peers) answerRemote(ctx context.Context, rec store.Question, q *hitlQuestion, opt hitlOption, by auth.Principal) error {
	t, err := p.target(ctx, rec.Contact, by.ID, 0)
	if err != nil {
		return err
	}
	defer t.close()
	_, err = t.send(ctx, answerMessage(a2a.TaskID(rec.TaskID), rec.ContextID, q.ID, opt.ID))
	return err
}

// resumeWatches restarts watches persisted by a previous run.
func (p *Peers) resumeWatches(ctx context.Context) {
	ws, err := p.s.store.Watches(ctx)
	if err != nil {
		slog.Error("loading watches", "error", err)
		return
	}
	for _, w := range ws {
		p.watch(ctx, w)
	}
}

// watch polls a remote task until it finishes and posts the result into the
// originating context as a new turn.
func (p *Peers) watch(ctx context.Context, w store.Watch) {
	key := w.Contact + "|" + w.TaskID
	p.mu.Lock()
	if p.watching[key] {
		p.mu.Unlock()
		return
	}
	p.watching[key] = true
	p.mu.Unlock()
	go func() {
		defer func() {
			p.mu.Lock()
			delete(p.watching, key)
			p.mu.Unlock()
		}()
		claims := callClaims{ContextID: w.OriginContextID, OnBehalfOf: w.OnBehalfOf}
		interval := watchInterval
		failingSince := time.Time{}
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(interval):
			}
			if interval < watchMaxInterval {
				interval += watchInterval
			}
			task, err := p.poll(ctx, w)
			if err != nil {
				if failingSince.IsZero() {
					failingSince = time.Now()
				}
				if time.Since(failingSince) < 24*time.Hour {
					continue
				}
				p.finishWatch(ctx, w, fmt.Sprintf("%s has been unreachable for a day; giving up on task %s: %v", w.Contact, w.TaskID, err))
				return
			}
			failingSince = time.Time{}
			if task.Status.State == a2a.TaskStateInputRequired {
				if q := questionOfStatus(task.Status); q != nil && q.Type == questionPermission {
					p.relay(ctx, claims, w.Contact, task, q)
					continue
				}
			}
			if !task.Status.State.Terminal() && task.Status.State != a2a.TaskStateInputRequired {
				continue
			}
			text := fmt.Sprintf("[%s finished task %s: %s]\n%s", w.Contact, w.TaskID, stateName(task.Status.State), resultText(task))
			if task.Status.State == a2a.TaskStateInputRequired {
				text = fmt.Sprintf("[%s asks, on task %s]\n%s\nAnswer with: a2a send -a <agents URL>/%s --task-id %s \"...\"",
					w.Contact, w.TaskID, messageText(task.Status.Message), w.Contact, w.TaskID)
			}
			p.finishWatch(ctx, w, text)
			return
		}
	}()
}

func (p *Peers) poll(ctx context.Context, w store.Watch) (*a2a.Task, error) {
	t, err := p.target(ctx, w.Contact, w.OnBehalfOf, 0)
	if err != nil {
		return nil, err
	}
	defer t.close()
	return t.get(ctx, a2a.TaskID(w.TaskID))
}

func (p *Peers) finishWatch(ctx context.Context, w store.Watch, text string) {
	if err := p.s.store.DeleteWatch(ctx, w.Contact, w.TaskID); err != nil {
		slog.Error("deleting watch", "error", err)
	}
	if p.s.router == nil {
		return
	}
	who := auth.Principal{ID: w.OnBehalfOf}
	if who.ID == "" || who.ID == auth.OwnerID {
		who = auth.Owner()
	} else if u, err := p.s.store.User(ctx, who.ID); err == nil {
		who.Roles = u.Roles
	}
	if err := p.s.router.injectTurn(ctx, w.OriginContextID, who, text); err != nil {
		slog.Error("posting watched result", "contact", w.Contact, "task", w.TaskID, "error", err)
	}
}

// resultText is the text of a finished task: its artifacts, or its status message.
func resultText(t *a2a.Task) string {
	var parts []string
	for _, a := range t.Artifacts {
		var sb strings.Builder
		for _, p := range a.Parts {
			sb.WriteString(p.Text())
		}
		if sb.Len() > 0 {
			parts = append(parts, sb.String())
		}
	}
	if len(parts) == 0 {
		return messageText(t.Status.Message)
	}
	return strings.Join(parts, "\n\n")
}
