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
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"github.com/a2aproject/a2a-cli/internal/agentsrv/approvals"
	"github.com/a2aproject/a2a-cli/internal/agentsrv/nodes"
	"github.com/a2aproject/a2a-cli/internal/auth"
	"github.com/a2aproject/a2a-cli/internal/channel"
	"github.com/a2aproject/a2a-cli/internal/controlapi"
	"github.com/a2aproject/a2a-cli/internal/cryptoutil"
	"github.com/a2aproject/a2a-cli/internal/network"
	"github.com/a2aproject/a2a-cli/internal/pairapi"
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
	store     *store.Store
	private   network.Provider
	nodes     *nodes.Host
	approvals *approvals.Service
	chat      chat
	agentName func() string
	bridge    harnessBridge
	// ctx ends when the server stops; watches run in it.
	ctx context.Context

	handleMu sync.Mutex

	mu       sync.Mutex
	watching map[string]bool
	cache    []network.PeerInfo
	cachedAt time.Time
	clients  map[string]*contactClient
}

// chat is where peers show relayed questions and post late results: the
// router.
type chat interface {
	approvals.Notifier
	injectTurn(ctx context.Context, contextID string, p auth.Principal, text string) error
}

// harnessBridge is how the harness reaches the server: through the Unix
// socket, or over TCP at url with key.
type harnessBridge struct {
	socket string
	url    string
	key    string
}

// contactClient is the A2A client of a contact, built for the contact as
// stored when it was made.
type contactClient struct {
	contact store.Contact
	client  *a2aclient.Client
}

// callClaims is what an agents URL stands for: calls from the harness in one
// context, on behalf of the user who started the turn.
type callClaims struct {
	ContextID  string
	OnBehalfOf string
	Hops       int
}

// handleAlphabet avoids characters that are easy to confuse.
const handleAlphabet = "abcdefghijkmnpqrstuvwxyz23456789"

// handle returns the short handle of the agents URL for c. The same claims
// keep their handle, also across restarts. A handle is not a secret: only
// the harness reaches the listener it is used on.
func (p *Peers) handle(ctx context.Context, c callClaims) (string, error) {
	want := store.Handle{ContextID: c.ContextID, OnBehalfOf: c.OnBehalfOf, Hops: c.Hops}
	p.handleMu.Lock()
	defer p.handleMu.Unlock()
	h, err := p.store.HandleFor(ctx, want)
	if err == nil {
		return h.Handle, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return "", err
	}
	for n := 2; ; n++ {
		for range 8 {
			want.Handle = cryptoutil.RandomString(n, handleAlphabet)
			err := p.store.PutHandle(ctx, want)
			if errors.Is(err, store.ErrExists) {
				continue
			}
			return want.Handle, err
		}
	}
}

// claimsOf returns the claims of a handle.
func (p *Peers) claimsOf(ctx context.Context, name string) (callClaims, bool) {
	h, err := p.store.HandleByName(ctx, name)
	if errors.Is(err, store.ErrNotFound) {
		return callClaims{}, false
	}
	if err != nil {
		slog.Error("loading agents URL handle", "error", err)
		return callClaims{}, false
	}
	return callClaims{ContextID: h.ContextID, OnBehalfOf: h.OnBehalfOf, Hops: h.Hops}, true
}

// discover lists agents on the private network, cached briefly.
func (p *Peers) discover(ctx context.Context) ([]network.PeerInfo, error) {
	if p.private == nil {
		return nil, errors.New("this agent has no private network; start it with --private tailscale or --private lan")
	}
	p.mu.Lock()
	if time.Since(p.cachedAt) < 30*time.Second {
		out := p.cache
		p.mu.Unlock()
		return out, nil
	}
	p.mu.Unlock()
	peers, err := p.private.Peers(ctx)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	p.cache, p.cachedAt = peers, time.Now()
	p.mu.Unlock()
	return peers, nil
}

// addContact stores a contact, pairing with it if a code is given.
func (p *Peers) addContact(ctx context.Context, req controlapi.ContactRequest) (store.Contact, error) {
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
	} else if p.private != nil && p.onPrivateNetwork(ctx, c.URL) {
		c.Via = ViaPrivate
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	hc := p.httpClient(c)
	if req.Code != "" {
		tok, err := pairWith(ctx, hc, c.URL, req.Code)
		if err != nil {
			return store.Contact{}, err
		}
		c.Token = tok
		hc = p.httpClient(c)
	}
	if _, err := network.FetchCard(ctx, hc, c.URL); err != nil {
		return store.Contact{}, fmt.Errorf("can't reach %s: %w", c.URL, err)
	}
	return c, p.store.PutContact(ctx, c)
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

// pairWith exchanges a pairing code at an agent for a caller token.
func pairWith(ctx context.Context, hc *http.Client, agentURL, code string) (string, error) {
	resp, err := pairapi.Pair(ctx, hc, agentURL, code)
	if err != nil {
		return "", err
	}
	if resp.Kind == pairapi.KindNode {
		return "", fmt.Errorf("pairing with %s: the code is for a webhook node, not a caller", agentURL)
	}
	return resp.Token, nil
}

// headerTransport adds the caller's headers to requests for the contact's
// host. Requests redirected elsewhere go without them, so that the token
// doesn't leak to another host.
type headerTransport struct {
	base   http.RoundTripper
	host   string
	header http.Header
}

func (t *headerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if !strings.EqualFold(r.URL.Host, t.host) {
		return t.base.RoundTrip(r)
	}
	r = r.Clone(r.Context())
	maps.Copy(r.Header, t.header)
	return t.base.RoundTrip(r)
}

// httpClient returns the HTTP client for a contact, which sends its token
// to the contact's host only. It has no overall timeout, because streams to
// the contact last as long as its task; callers bound requests with their
// context.
func (p *Peers) httpClient(c store.Contact) *http.Client {
	base := http.DefaultClient
	if c.Via == ViaPrivate && p.private != nil {
		base = p.private.HTTPClient()
	}
	rt := base.Transport
	if rt == nil {
		rt = http.DefaultTransport
	}
	h := http.Header{}
	if c.Token != "" {
		h.Set("Authorization", "Bearer "+c.Token)
	}
	host := ""
	if u, err := url.Parse(c.URL); err == nil {
		host = u.Host
	}
	return &http.Client{Transport: &headerTransport{base: rt, host: host, header: h}}
}

// onBehalfOf returns ctx for calling another agent on behalf of a user, at
// hops agents deep.
func onBehalfOf(ctx context.Context, user string, hops int) context.Context {
	params := a2aclient.ServiceParams{auth.HeaderHops: {strconv.Itoa(hops + 1)}}
	if user != "" {
		params[auth.HeaderOnBehalfOf] = []string{user}
	}
	return a2aclient.AttachServiceParams(ctx, params)
}

// target is an agent the server calls on behalf of a user.
type target interface {
	send(ctx context.Context, msg *a2a.Message) (a2a.SendMessageResult, error)
	get(ctx context.Context, id a2a.TaskID) (*a2a.Task, error)
}

type remoteTarget struct {
	c    *a2aclient.Client
	user string
	hops int
}

func (t remoteTarget) send(ctx context.Context, msg *a2a.Message) (a2a.SendMessageResult, error) {
	return t.c.SendMessage(onBehalfOf(ctx, t.user, t.hops), &a2a.SendMessageRequest{Message: msg, Config: &a2a.SendMessageConfig{ReturnImmediately: true}})
}

func (t remoteTarget) get(ctx context.Context, id a2a.TaskID) (*a2a.Task, error) {
	return t.c.GetTask(onBehalfOf(ctx, t.user, t.hops), &a2a.GetTaskRequest{ID: id})
}

type localTarget struct {
	h a2asrv.RequestHandler
	p auth.Principal
}

func (t localTarget) send(ctx context.Context, msg *a2a.Message) (a2a.SendMessageResult, error) {
	return t.h.SendMessage(auth.InProcess(ctx, t.p), &a2a.SendMessageRequest{Message: msg, Config: &a2a.SendMessageConfig{ReturnImmediately: true}})
}

func (t localTarget) get(ctx context.Context, id a2a.TaskID) (*a2a.Task, error) {
	return t.h.GetTask(auth.InProcess(ctx, t.p), &a2a.GetTaskRequest{ID: id})
}

// ErrOffline is returned when a contact can't be reached.
var ErrOffline = errors.New("offline")

// target resolves an agent name to a contact or a hosted node.
func (p *Peers) target(ctx context.Context, name, user string, hops int) (target, error) {
	node, err := p.isNode(ctx, name)
	if err != nil {
		return nil, err
	}
	if node {
		return localTarget{h: p.nodes.Handler(name), p: p.nodeCaller(user, hops)}, nil
	}
	client, err := p.contactClient(ctx, name)
	if err != nil {
		return nil, err
	}
	return remoteTarget{c: client, user: user, hops: hops}, nil
}

// isNode reports whether name is a webhook node hosted here.
func (p *Peers) isNode(ctx context.Context, name string) (bool, error) {
	_, err := p.store.Node(ctx, name)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// nodeCaller is the principal this agent calls its nodes as.
func (p *Peers) nodeCaller(user string, hops int) auth.Principal {
	return auth.Principal{ID: "agent:" + p.agentName(), Roles: []string{auth.RoleOwner}, OnBehalfOf: user, Hops: hops + 1}
}

// contactClient returns the client of a contact, one per contact: the user
// and hop count go with each request. A client is rebuilt when the contact
// changed, and dropped with the contact. It returns [store.ErrNotFound] for
// a name which isn't a contact.
func (p *Peers) contactClient(ctx context.Context, name string) (*a2aclient.Client, error) {
	c, err := p.store.Contact(ctx, name)
	if errors.Is(err, store.ErrNotFound) {
		p.forgetClient(name)
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	cached := p.clients[name]
	p.mu.Unlock()
	if cached != nil && sameContact(cached.contact, c) {
		return cached.client, nil
	}
	hc := p.httpClient(c)
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
	p.mu.Lock()
	old := p.clients[name]
	p.clients[name] = &contactClient{contact: c, client: client}
	p.mu.Unlock()
	if old != nil {
		destroyClient(old.client)
	}
	return client, nil
}

func sameContact(a, b store.Contact) bool {
	return a.URL == b.URL && a.Token == b.Token && a.Via == b.Via
}

// forgetClient drops the client of a contact, so that the next call
// resolves its card again.
func (p *Peers) forgetClient(name string) {
	p.mu.Lock()
	old := p.clients[name]
	delete(p.clients, name)
	p.mu.Unlock()
	if old != nil {
		destroyClient(old.client)
	}
}

// close drops the clients of all contacts.
func (p *Peers) close(context.Context) error {
	p.mu.Lock()
	clients := p.clients
	p.clients = map[string]*contactClient{}
	p.mu.Unlock()
	for _, c := range clients {
		destroyClient(c.client)
	}
	return nil
}

func destroyClient(c *a2aclient.Client) {
	if err := c.Destroy(); err != nil {
		slog.Debug("closing a2a client", "error", err)
	}
}

// relay asks the user on whose behalf a remote task runs to answer its
// permission question, once per question, also across restarts.
func (p *Peers) relay(ctx context.Context, claims callClaims, agent string, task *a2a.Task, q *approvals.Question) {
	rec, err := p.approvals.NewRecord(ctx, task.ID, task.ContextID, agent, q)
	if errors.Is(err, store.ErrExists) {
		return
	}
	if err != nil {
		slog.Error("storing remote question", "error", err)
		return
	}
	chName, conv, err := p.store.ConvOf(ctx, claims.ContextID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		slog.Warn("finding the conversation of a relayed question", "context", claims.ContextID, "error", err)
	}
	user := claims.OnBehalfOf
	if user == "" {
		user = auth.OwnerID
	}
	header := fmt.Sprintf("%s wants to", agent)
	if q.Title != "" {
		header = fmt.Sprintf("%s wants to run %s", agent, q.Title)
	}
	p.chat.Ask(ctx, []string{user}, chName, channel.ConvKey(conv), header, rec)
}

// AnswerRemote forwards an answer to a remote agent's question.
func (p *Peers) AnswerRemote(ctx context.Context, rec store.Question, q *approvals.Question, opt approvals.Option, by auth.Principal) error {
	t, err := p.target(ctx, rec.Contact, by.ID, 0)
	if err != nil {
		return err
	}
	if _, err = t.send(ctx, approvals.AnswerMessage(a2a.TaskID(rec.TaskID), rec.ContextID, q.ID, opt.ID)); err != nil {
		p.forgetClient(rec.Contact)
	}
	return err
}

// resumeWatches restarts watches persisted by a previous run.
func (p *Peers) resumeWatches(ctx context.Context) {
	ws, err := p.store.Watches(ctx)
	if err != nil {
		slog.Error("loading watches", "error", err)
		return
	}
	for _, w := range ws {
		p.watch(w)
	}
}

// watch polls a remote task until it finishes or the server stops, and posts
// the result into the originating context as a new turn.
func (p *Peers) watch(w store.Watch) {
	ctx := p.ctx
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
				if q := approvals.FromStatus(task.Status); q != nil && q.Type == approvals.TypePermission {
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
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	t, err := p.target(ctx, w.Contact, w.OnBehalfOf, 0)
	if err != nil {
		return nil, err
	}
	task, err := t.get(ctx, a2a.TaskID(w.TaskID))
	if err != nil {
		p.forgetClient(w.Contact)
	}
	return task, err
}

func (p *Peers) finishWatch(ctx context.Context, w store.Watch, text string) {
	if err := p.store.DeleteWatch(ctx, w.Contact, w.TaskID); err != nil {
		slog.Error("deleting watch", "error", err)
	}
	if err := p.chat.injectTurn(ctx, w.OriginContextID, p.watchPrincipal(ctx, w), text); err != nil {
		slog.Error("posting watched result", "contact", w.Contact, "task", w.TaskID, "error", err)
	}
}

// watchPrincipal is the user a watched task's result is posted as.
func (p *Peers) watchPrincipal(ctx context.Context, w store.Watch) auth.Principal {
	if w.OnBehalfOf == "" || w.OnBehalfOf == auth.OwnerID {
		return auth.Owner()
	}
	who := auth.Principal{ID: w.OnBehalfOf}
	u, err := p.store.User(ctx, who.ID)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			slog.Warn("loading the user of a watch", "user", who.ID, "error", err)
		}
		return who
	}
	who.Roles = u.Roles
	return who
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
