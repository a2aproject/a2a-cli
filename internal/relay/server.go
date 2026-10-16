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

package relay

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"github.com/a2aproject/a2a-go/v2/a2asrv/push"
	"github.com/a2aproject/a2a-go/v2/a2asrv/taskstore"
)

// Config configures a relay [Server].
type Config struct {
	// PublicURL is the relay's public base URL. If empty, it is derived from
	// the Host and X-Forwarded-Proto headers of incoming requests.
	PublicURL string
	// AdminToken authorizes creating nodes and acting on behalf of any node.
	AdminToken string
	// StateDir keeps attached nodes across restarts. Tasks are kept in memory.
	// Empty means nothing is persisted.
	StateDir string
	// TaskTimeout fails a task when its node posts no event for this long.
	TaskTimeout time.Duration
	// PairingTTL is how long a pairing code is valid.
	PairingTTL time.Duration
	// HTTPClient is used for webhooks and peers.
	HTTPClient *http.Client
	// WebhookRetries are the delays before each webhook delivery attempt.
	WebhookRetries []time.Duration
	// PollInterval is how often the relay checks a peer task that it can't
	// stream.
	PollInterval time.Duration
}

// Server is a relay. It is an [http.Handler].
type Server struct {
	cfg   Config
	store *store
	tasks taskstore.Store
	push  push.ConfigStore
	// pushSender delivers A2A push notifications to callers.
	pushSender push.Sender
	mux        *http.ServeMux
	baseURL    atomic.Pointer[string]

	mu       sync.Mutex
	handlers map[string]http.Handler
	open     map[a2a.TaskID]*waiter
	taskInfo map[a2a.TaskID]*taskInfo
	inbox    map[string][]*InboxEntry
}

const inboxSize = 200

// New creates a relay.
func New(cfg Config) (*Server, error) {
	if cfg.TaskTimeout <= 0 {
		cfg.TaskTimeout = time.Hour
	}
	if cfg.PairingTTL <= 0 {
		cfg.PairingTTL = 15 * time.Minute
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if cfg.WebhookRetries == nil {
		cfg.WebhookRetries = []time.Duration{0, 2 * time.Second, 10 * time.Second, 30 * time.Second}
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 5 * time.Second
	}
	cfg.PublicURL = strings.TrimSuffix(cfg.PublicURL, "/")
	var path string
	if cfg.StateDir != "" {
		path = filepath.Join(cfg.StateDir, "nodes.json")
	}
	st, err := openStore(path)
	if err != nil {
		return nil, err
	}
	s := &Server{
		cfg:   cfg,
		store: st,
		tasks: taskstore.NewInMemory(&taskstore.InMemoryStoreConfig{Authenticator: taskUser}),
		push:  push.NewInMemoryStore(),
		pushSender: push.NewHTTPPushSender(&push.HTTPSenderConfig{
			Timeout: 15 * time.Second,
		}),
		handlers: map[string]http.Handler{},
		open:     map[a2a.TaskID]*waiter{},
		taskInfo: map[a2a.TaskID]*taskInfo{},
		inbox:    map[string][]*InboxEntry{},
	}
	if cfg.PublicURL != "" {
		s.baseURL.Store(&cfg.PublicURL)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleSkill)
	mux.HandleFunc("GET /skill.md", s.handleSkill)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /download/a2a", s.handleDownload)
	mux.HandleFunc("/n/{node}", s.handleNode)
	mux.HandleFunc("/n/{node}/{rest...}", s.handleNode)

	mux.HandleFunc("POST /api/nodes", s.admin(s.handleCreateNode))
	mux.HandleFunc("POST /api/nodes/{id}/pairing", s.admin(s.handleNewPairing))
	mux.HandleFunc("POST /api/pair", s.handlePair)
	mux.HandleFunc("GET /api/node", s.nodeAPI(s.handleGetNode))
	mux.HandleFunc("PATCH /api/node", s.nodeAPI(s.handleUpdateNode))
	mux.HandleFunc("POST /api/node/ping", s.nodeAPI(s.handlePing))
	mux.HandleFunc("POST /api/tasks/{id}/events", s.nodeAPI(s.handleEvent))
	mux.HandleFunc("GET /api/tasks/{id}", s.nodeAPI(s.handleGetTask))
	mux.HandleFunc("GET /api/inbox", s.nodeAPI(s.handleInbox))
	mux.HandleFunc("POST /api/send", s.nodeAPI(s.handleSend))
	mux.HandleFunc("GET /api/grants", s.nodeAPI(s.handleListGrants))
	mux.HandleFunc("POST /api/grants", s.nodeAPI(s.handleGrant))
	mux.HandleFunc("DELETE /api/grants/{name}", s.nodeAPI(s.handleRevoke))
	s.mux = mux
	return s, nil
}

// ServeHTTP implements [http.Handler].
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.cfg.PublicURL == "" && s.baseURL.Load() == nil && r.Host != "" {
		base := requestBase(r)
		s.baseURL.CompareAndSwap(nil, &base)
	}
	s.mux.ServeHTTP(w, r)
}

// base returns the public URL of the relay for links in cards and envelopes.
func (s *Server) base(r *http.Request) string {
	if s.cfg.PublicURL != "" {
		return s.cfg.PublicURL
	}
	if r != nil && r.Host != "" {
		return requestBase(r)
	}
	if b := s.baseURL.Load(); b != nil {
		return *b
	}
	return ""
}

func requestBase(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if p := r.Header.Get("X-Forwarded-Proto"); p != "" {
		scheme = strings.TrimSpace(strings.Split(p, ",")[0])
	}
	return scheme + "://" + r.Host
}

func (s *Server) nodeURL(r *http.Request, id string) string { return s.base(r) + "/n/" + id }

// A2A serving.

// handleNode serves the A2A endpoints of a node under /n/{node}.
func (s *Server) handleNode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("node")
	n, ok := s.store.get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "no such agent")
		return
	}
	rest := "/" + r.PathValue("rest")
	// a2a-go resolves a card from a URL with a path by fetching the URL
	// itself; other clients append the well-known path.
	if r.Method == http.MethodGet && (rest == "/" || rest == a2asrv.WellKnownAgentCardPath || rest == "/.well-known/agent.json") {
		a2asrv.NewStaticAgentCardHandler(s.card(r, n)).ServeHTTP(w, r)
		return
	}
	r2 := r.Clone(r.Context())
	r2.URL.Path, r2.URL.RawPath = rest, ""
	s.a2aHandler(id).ServeHTTP(w, r2)
}

func (s *Server) a2aHandler(id string) http.Handler {
	s.mu.Lock()
	defer s.mu.Unlock()
	if h, ok := s.handlers[id]; ok {
		return h
	}
	caps := a2a.AgentCapabilities{Streaming: true, PushNotifications: true}
	handler := a2asrv.NewHandler(&executor{s: s, node: id},
		a2asrv.WithTaskStore(s.tasks),
		a2asrv.WithPushNotifications(s.push, s.pushSender),
		a2asrv.WithCallInterceptors(&callerAuth{s: s, node: id}),
		a2asrv.WithCapabilityChecks(&caps),
		a2asrv.WithLogger(slog.Default()),
	)
	mux := http.NewServeMux()
	mux.Handle("POST /{$}", a2asrv.NewJSONRPCHandler(handler, a2asrv.WithTransportKeepAlive(15*time.Second)))
	mux.Handle("/", a2asrv.NewRESTHandler(handler, a2asrv.WithTransportKeepAlive(15*time.Second)))
	s.handlers[id] = mux
	return mux
}

func (s *Server) card(r *http.Request, n node) *a2a.AgentCard {
	u := s.nodeURL(r, n.ID)
	desc := n.Description
	if desc == "" {
		desc = n.Name + " is reachable through an A2A relay."
	}
	card := &a2a.AgentCard{
		Name:        n.Name,
		Description: desc,
		Version:     "0.1.0",
		SupportedInterfaces: []*a2a.AgentInterface{
			a2a.NewAgentInterface(u, a2a.TransportProtocolJSONRPC),
			a2a.NewAgentInterface(u, a2a.TransportProtocolHTTPJSON),
		},
		Capabilities:       a2a.AgentCapabilities{Streaming: true, PushNotifications: true},
		DefaultInputModes:  []string{"text/plain", "application/json", "*/*"},
		DefaultOutputModes: []string{"text/plain", "application/json", "*/*"},
		Skills: []a2a.AgentSkill{{
			ID: "relay", Name: n.Name, Tags: []string{"async"},
			Description: "Works asynchronously: a task stays submitted until the agent wakes up, and can stay working for a long time. " +
				"Use returnImmediately with tasks/get, a push notification config, or streaming to get the result.",
		}},
	}
	if !n.Public {
		card.SecuritySchemes = a2a.NamedSecuritySchemes{
			"bearer": a2a.HTTPAuthSecurityScheme{Scheme: "Bearer", Description: "A caller token issued by the agent (" + CLI + " grant)."},
		}
		card.SecurityRequirements = a2a.SecurityRequirementsOptions{{"bearer": {}}}
	}
	return card
}

// callerAuth resolves the caller of a node from its bearer token and rejects
// callers the node doesn't allow.
type callerAuth struct {
	a2asrv.PassthroughCallInterceptor
	s    *Server
	node string
}

func (a *callerAuth) Before(ctx context.Context, callCtx *a2asrv.CallContext, req *a2asrv.Request) (context.Context, any, error) {
	n, ok := a.s.store.get(a.node)
	if !ok {
		return ctx, nil, a2a.ErrUnauthenticated
	}
	var token string
	if v, ok := callCtx.ServiceParams().Get("authorization"); ok && len(v) > 0 {
		token = bearer(v[0])
	}
	sender, err := a.s.authenticateCaller(n, token)
	if err != nil {
		return ctx, nil, err
	}
	callCtx.User = &a2asrv.User{
		Name:          a.node + "/" + sender.String(),
		Authenticated: sender.Kind != SenderAnonymous,
		Attributes:    map[string]any{"sender": sender},
	}
	return ctx, nil, nil
}

func (s *Server) authenticateCaller(n node, token string) (Sender, error) {
	if token == "" {
		if n.Public {
			return Sender{Kind: SenderAnonymous}, nil
		}
		return Sender{}, fmt.Errorf("%w: this agent needs a caller token, ask its owner for one", a2a.ErrUnauthenticated)
	}
	if s.isAdmin(token) {
		return Sender{Kind: SenderOwner}, nil
	}
	if g, ok := n.grantByToken(token); ok {
		return Sender{Kind: SenderGrant, Name: g.Name}, nil
	}
	if m, ok := s.nodeByToken(token); ok {
		if m.ID == n.ID || n.Public || n.allows("node:"+m.ID) {
			return Sender{Kind: SenderNode, Name: m.ID}, nil
		}
		return Sender{}, fmt.Errorf("%w: node %s is not allowed to call %s", a2a.ErrUnauthorized, m.ID, n.ID)
	}
	return Sender{}, fmt.Errorf("%w: invalid token", a2a.ErrUnauthenticated)
}

// The task store scopes tasks to the user set by callerAuth, which includes
// the node ID, so callers only see their own tasks on that node. The relay
// reads tasks on behalf of nodes with asTaskUser.

type taskUserKey struct{}

func asTaskUser(ctx context.Context, user string) context.Context {
	return context.WithValue(ctx, taskUserKey{}, user)
}

func taskUser(ctx context.Context) (string, error) {
	if u, ok := ctx.Value(taskUserKey{}).(string); ok {
		return u, nil
	}
	if callCtx, ok := a2asrv.CallContextFrom(ctx); ok && callCtx.User != nil {
		return callCtx.User.Name, nil
	}
	return "", a2a.ErrUnauthenticated
}

// Relay API.

type apiCall struct {
	node   node
	caller Sender
	token  string
}

func (s *Server) isAdmin(token string) bool {
	return s.cfg.AdminToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(s.cfg.AdminToken)) == 1
}

func (s *Server) nodeByToken(token string) (node, bool) {
	_, id, ok := ParseNodeToken(token)
	if !ok {
		return node{}, false
	}
	n, ok := s.store.get(id)
	if !ok || n.TokenHash == "" || subtle.ConstantTimeCompare([]byte(n.TokenHash), []byte(hash(token))) != 1 {
		return node{}, false
	}
	return n, true
}

func (s *Server) admin(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.isAdmin(bearer(r.Header.Get("Authorization"))) {
			writeError(w, http.StatusUnauthorized, "admin token required")
			return
		}
		h(w, r)
	}
}

// nodeAPI authenticates a node by its node token, or the admin acting on a
// node named in the X-A2A-Relay-Node header.
func (s *Server) nodeAPI(h func(http.ResponseWriter, *http.Request, apiCall)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := bearer(r.Header.Get("Authorization"))
		if s.isAdmin(token) {
			id := r.Header.Get("X-A2A-Relay-Node")
			n, ok := s.store.get(id)
			if !ok {
				writeError(w, http.StatusNotFound, "set X-A2A-Relay-Node to an existing node ID")
				return
			}
			h(w, r, apiCall{node: n, caller: Sender{Kind: SenderOwner}, token: token})
			return
		}
		n, ok := s.nodeByToken(token)
		if !ok {
			writeError(w, http.StatusUnauthorized, "node token required")
			return
		}
		h(w, r, apiCall{node: n, caller: Sender{Kind: SenderNode, Name: n.ID}, token: token})
	}
}

func (s *Server) info(r *http.Request, n node) NodeInfo {
	return NodeInfo{
		ID: n.ID, Name: n.Name, Description: n.Description, URL: s.nodeURL(r, n.ID),
		Webhook: redactURL(n.Webhook), Public: n.Public, Paired: n.TokenHash != "",
		Grants: n.grantNames(), CreatedAt: n.CreatedAt,
	}
}

func (s *Server) handleCreateNode(w http.ResponseWriter, r *http.Request) {
	var req CreateNodeRequest
	if !readJSON(w, r, &req) {
		return
	}
	if err := validateWebhook(req.Webhook); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	n := &node{
		ID: newNodeID(), Name: req.Name, Description: req.Description,
		Webhook: req.Webhook, WebhookHeaders: req.WebhookHeaders, Public: req.Public,
		CreatedAt: time.Now().UTC(),
	}
	if n.Name == "" {
		n.Name = "agent-" + n.ID
	}
	code := newPairingCode()
	n.PairingHash, n.PairingExpires = hash(normalizeCode(code)), time.Now().Add(s.cfg.PairingTTL).UTC()
	if err := s.store.create(n); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	slog.Info("node attached", "node", n.ID, "name", n.Name)
	writeJSON(w, http.StatusCreated, PairingResponse{Node: s.info(r, *n), PairingCode: code, ExpiresAt: n.PairingExpires})
}

func (s *Server) handleNewPairing(w http.ResponseWriter, r *http.Request) {
	code := newPairingCode()
	expires := time.Now().Add(s.cfg.PairingTTL).UTC()
	n, err := s.store.update(r.PathValue("id"), func(n *node) error {
		n.PairingHash, n.PairingExpires = hash(normalizeCode(code)), expires
		return nil
	})
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, PairingResponse{Node: s.info(r, n), PairingCode: code, ExpiresAt: expires})
}

func (s *Server) handlePair(w http.ResponseWriter, r *http.Request) {
	var req PairRequest
	if !readJSON(w, r, &req) {
		return
	}
	var token string
	n, err := s.store.pairWith(req.Code, time.Now(), func(n *node) string {
		token = newNodeToken(s.base(r), n.ID)
		return hash(token)
	})
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	slog.Info("node paired", "node", n.ID)
	writeJSON(w, http.StatusOK, PairResponse{Node: s.info(r, n), Token: token})
}

func (s *Server) handleGetNode(w http.ResponseWriter, r *http.Request, c apiCall) {
	writeJSON(w, http.StatusOK, s.info(r, c.node))
}

// UpdateNodeRequest is the body of PATCH /api/node. Unset fields are kept.
type UpdateNodeRequest struct {
	Name           *string           `json:"name,omitempty"`
	Description    *string           `json:"description,omitempty"`
	Webhook        *string           `json:"webhook,omitempty"`
	WebhookHeaders map[string]string `json:"webhookHeaders,omitempty"`
	Public         *bool             `json:"public,omitempty"`
}

func (s *Server) handleUpdateNode(w http.ResponseWriter, r *http.Request, c apiCall) {
	var req UpdateNodeRequest
	if !readJSON(w, r, &req) {
		return
	}
	n, err := s.store.update(c.node.ID, func(n *node) error {
		if req.Webhook != nil {
			if err := validateWebhook(*req.Webhook); err != nil {
				return err
			}
			n.Webhook = *req.Webhook
		}
		if req.WebhookHeaders != nil {
			n.WebhookHeaders = req.WebhookHeaders
		}
		if req.Name != nil {
			n.Name = *req.Name
		}
		if req.Description != nil {
			n.Description = *req.Description
		}
		if req.Public != nil {
			n.Public = *req.Public
		}
		return nil
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.info(r, n))
}

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request, c apiCall) {
	entry := s.wake(r, c.node.ID, Envelope{Kind: KindPing, Sender: Sender{Kind: SenderRelay}, Text: "Test wake from the relay."})
	// Report the first delivery attempt, which is what the caller is testing.
	deadline := time.After(20 * time.Second)
	for {
		s.mu.Lock()
		res := *entry
		s.mu.Unlock()
		if res.Delivered || res.Attempts > 0 {
			writeJSON(w, http.StatusOK, res)
			return
		}
		select {
		case <-deadline:
			writeJSON(w, http.StatusOK, res)
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (s *Server) handleListGrants(w http.ResponseWriter, r *http.Request, c apiCall) {
	writeJSON(w, http.StatusOK, map[string]any{"grants": c.node.grantNames()})
}

func (s *Server) handleGrant(w http.ResponseWriter, r *http.Request, c apiCall) {
	var req GrantRequest
	if !readJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || strings.ContainsAny(req.Name, "/ ") {
		writeError(w, http.StatusBadRequest, "grant name must be non-empty, without spaces or slashes")
		return
	}
	var token string
	g := grant{Name: req.Name, CreatedAt: time.Now().UTC()}
	if id, ok := strings.CutPrefix(req.Name, "node:"); ok {
		if _, exists := s.store.get(id); !exists {
			writeError(w, http.StatusNotFound, "no node "+id+" on this relay")
			return
		}
	} else {
		token = newGrantToken()
		g.TokenHash = hash(token)
	}
	_, err := s.store.update(c.node.ID, func(n *node) error {
		// Granting an existing name rotates its token.
		n.Grants = deleteGrant(n.Grants, req.Name)
		n.Grants = append(n.Grants, g)
		return nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, GrantResponse{Name: req.Name, Token: token})
}

func (s *Server) handleRevoke(w http.ResponseWriter, r *http.Request, c apiCall) {
	name := r.PathValue("name")
	if !c.node.allows(name) {
		writeError(w, http.StatusNotFound, "no grant "+name)
		return
	}
	if _, err := s.store.update(c.node.ID, func(n *node) error {
		n.Grants = deleteGrant(n.Grants, name)
		return nil
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"revoked": name})
}

func deleteGrant(gs []grant, name string) []grant {
	out := gs[:0:0]
	for _, g := range gs {
		if g.Name != name {
			out = append(out, g)
		}
	}
	return out
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	exe, err := os.Executable()
	if err != nil {
		writeError(w, http.StatusNotFound, "binary not available")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="a2a"`)
	http.ServeFile(w, r, exe)
}

// Helpers.

func bearer(h string) string {
	if v, ok := strings.CutPrefix(h, "Bearer "); ok {
		return strings.TrimSpace(v)
	}
	if v, ok := strings.CutPrefix(h, "bearer "); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func validateWebhook(u string) error {
	p, err := url.Parse(u)
	if err != nil || (p.Scheme != "https" && p.Scheme != "http") || p.Host == "" {
		return fmt.Errorf("webhook must be an http(s) URL, got %q", u)
	}
	return nil
}

// redactURL drops the query, which webhook providers often use for secrets.
func redactURL(u string) string {
	p, err := url.Parse(u)
	if err != nil {
		return ""
	}
	if p.RawQuery != "" {
		p.RawQuery = "…"
	}
	p.User = nil
	return p.String()
}

const maxBody = 32 << 20

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	if err := dec.Decode(v); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		slog.Warn("writing response", "error", err)
	}
}

// APIError is the body of an error response from the relay API.
type APIError struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, APIError{Error: msg})
}
