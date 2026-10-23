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
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"

	"github.com/a2aproject/a2a-cli/internal/agentdir"
	"github.com/a2aproject/a2a-cli/internal/auth"
	"github.com/a2aproject/a2a-cli/internal/channel"
	"github.com/a2aproject/a2a-cli/internal/fsutil"
	"github.com/a2aproject/a2a-cli/internal/store"
)

// Status describes a running agent server.
type Status struct {
	Name     string          `json:"name"`
	Harness  string          `json:"harness"`
	Dir      string          `json:"dir"`
	PID      int             `json:"pid"`
	Started  time.Time       `json:"started"`
	Local    string          `json:"local"`
	Private  string          `json:"private,omitempty"`
	Public   string          `json:"public,omitempty"`
	Channels []ChannelStatus `json:"channels,omitempty"`
	Tasks    map[string]int  `json:"tasks"`
	Owners   []string        `json:"owners,omitempty"`
	Contacts int             `json:"contacts"`
	Nodes    int             `json:"nodes"`
}

// ChannelStatus describes a channel of the agent.
type ChannelStatus struct {
	Name    string `json:"name"`
	Account string `json:"account,omitempty"`
	// Error is set while the channel is failing and waits to restart.
	Error string `json:"error,omitempty"`
}

// PairCodeRequest asks for a pairing code.
//
// With Webhook, Name is a webhook node, created or updated. A Name which
// already is a node gets a new node code.
type PairCodeRequest struct {
	Name        string            `json:"name"`
	Role        string            `json:"role,omitempty"`
	Webhook     string            `json:"webhook,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Description string            `json:"description,omitempty"`
}

// PairCodeResponse carries a pairing code and how to use it.
type PairCodeResponse struct {
	Code    string    `json:"code"`
	Expires time.Time `json:"expires"`
	URL     string    `json:"url"`
	// PairCommand is what the caller or the node runs to pair.
	PairCommand string `json:"pairCommand"`
	// Hints tell a person how to link a channel identity with the code.
	Hints []string `json:"hints,omitempty"`
	// Node is set for node codes.
	Node *NodeInfo `json:"node,omitempty"`
}

// RevokeResponse tells what `pair rm` removed.
type RevokeResponse struct {
	// Kind is caller or node.
	Kind string `json:"kind"`
}

// TokenInfo describes an issued token.
type TokenInfo struct {
	Name      string    `json:"name"`
	Principal string    `json:"principal"`
	Roles     []string  `json:"roles"`
	URL       string    `json:"url"`
	Created   time.Time `json:"created"`
	LastUsed  time.Time `json:"lastUsed,omitzero"`
}

// ContactRequest adds a contact. Without URL, the contact is looked up by
// name among the peers on the private network. Code pairs with the contact;
// Token sets the outbound token directly.
type ContactRequest struct {
	Name  string `json:"name"`
	URL   string `json:"url,omitempty"`
	Code  string `json:"code,omitempty"`
	Token string `json:"token,omitempty"`
}

// ContactInfo describes a contact.
type ContactInfo struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Via      string `json:"via"`
	HasToken bool   `json:"hasToken"`
}

// ChannelRequest adds or replaces a channel. Token is a secret reference
// already stored in the secrets file.
type ChannelRequest struct {
	Kind  string                `json:"kind"`
	Token string                `json:"token,omitempty"`
	Email *agentdir.EmailConfig `json:"email,omitempty"`
}

// NodeInfo describes a webhook node.
type NodeInfo struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	URL     string    `json:"url"`
	Webhook string    `json:"webhook"`
	Paired  bool      `json:"paired"`
	Created time.Time `json:"created"`
}

// serveControl serves the control socket, through which management commands
// change the running server without a restart.
func (s *Server) serveControl(ctx context.Context, errc chan<- error) error {
	path := s.dir.SocketPath()
	if err := fsutil.MkdirPrivate(filepath.Dir(path)); err != nil {
		return err
	}
	if c, err := net.DialTimeout("unix", path, time.Second); err == nil {
		return errors.Join(fmt.Errorf("an agent server is already running in %s (a2a server status)", s.dir.Root), c.Close())
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return fmt.Errorf("control socket: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return errors.Join(err, ln.Close())
	}
	if err := os.WriteFile(s.dir.PIDPath(), []byte(strconv.Itoa(pid())), 0o600); err != nil {
		return errors.Join(err, ln.Close())
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", s.ctlStatus)
	mux.HandleFunc("POST /stop", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"stopping": true})
		go s.Stop()
	})
	mux.HandleFunc("POST /pair", s.ctlPair)
	mux.HandleFunc("GET /tokens", s.ctlTokens)
	mux.HandleFunc("DELETE /pair/{name}", s.ctlRevoke)
	mux.HandleFunc("GET /peers", s.ctlPeers)
	mux.HandleFunc("GET /contacts", s.ctlContacts)
	mux.HandleFunc("POST /contacts", s.ctlAddContact)
	mux.HandleFunc("DELETE /contacts/{name}", s.ctlDeleteContact)
	mux.HandleFunc("POST /channels", s.ctlAddChannel)
	mux.HandleFunc("DELETE /channels/{kind}", s.ctlDeleteChannel)
	mux.HandleFunc("GET /nodes", s.ctlNodes)
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second, BaseContext: func(net.Listener) context.Context { return ctx }}
	s.closers = append(s.closers, func(ctx context.Context) error {
		err := srv.Shutdown(ctx)
		return errors.Join(err, removeIfExists(path), removeIfExists(s.dir.PIDPath()))
	})
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			reportFatal(errc, fmt.Errorf("control socket: %w", err))
		}
	}()
	return nil
}

func removeIfExists(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *Server) ctlStatus(w http.ResponseWriter, r *http.Request) {
	cfg := s.config()
	local, private, public := s.URLs()
	st := Status{
		Name: cfg.Name, Harness: cfg.Harness, Dir: s.dir.Root, PID: pid(), Started: s.started,
		Local: local, Private: private, Public: public, Tasks: map[string]int{},
	}
	st.Channels = s.router.status()
	counts, err := s.tasks.TaskCounts(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for state, n := range counts {
		st.Tasks[stateName(state)] = n
	}
	links, err := s.store.LinksOf(r.Context(), auth.OwnerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, l := range links {
		st.Owners = append(st.Owners, l.Channel+":"+l.Identity)
	}
	contacts, err := s.store.Contacts(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	nodes, err := s.store.Nodes(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	st.Contacts, st.Nodes = len(contacts), len(nodes)
	writeJSON(w, http.StatusOK, st)
}

func stateName(s a2a.TaskState) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(string(s), "TASK_STATE_")), "_", "-")
}

func (s *Server) ctlPair(w http.ResponseWriter, r *http.Request) {
	var req PairCodeRequest
	if !readJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || strings.ContainsAny(req.Name, " /:") {
		writeError(w, http.StatusBadRequest, "--name must be non-empty, without spaces, slashes or colons")
		return
	}
	_, err := s.store.Node(r.Context(), req.Name)
	isNode := err == nil
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if (req.Webhook != "" || isNode) && req.Role != "" {
		writeError(w, http.StatusBadRequest, "--role doesn't apply to webhook nodes")
		return
	}
	if req.Webhook != "" || isNode {
		s.pairNode(w, r, req, isNode)
		return
	}
	if req.Role == "" {
		req.Role = auth.RoleMember
	}
	if req.Role != auth.RoleMember && req.Role != auth.RoleOwner {
		writeError(w, http.StatusBadRequest, "--role must be member or owner")
		return
	}
	code, expires, err := auth.MintCode(r.Context(), s.store, auth.PurposePair, req.Name, req.Role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := PairCodeResponse{Code: code, Expires: expires, URL: s.bestURL(), PairCommand: pairCommand(s.bestURL(), code), Hints: s.router.linkHints(code)}
	writeJSON(w, http.StatusOK, resp)
}

// pairNode creates or updates a webhook node and mints a node code for it.
func (s *Server) pairNode(w http.ResponseWriter, r *http.Request, req PairCodeRequest, exists bool) {
	if req.Webhook != "" && !exists {
		toks, err := s.store.Tokens(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if slices.ContainsFunc(toks, func(t store.Token) bool { return t.Name == req.Name }) {
			writeError(w, http.StatusConflict, req.Name+" is already paired as a caller; pick another name, or run `a2a server pair rm "+req.Name+"` first")
			return
		}
	}
	var (
		resp *PairCodeResponse
		err  error
	)
	if req.Webhook != "" {
		resp, err = s.nodes.put(r.Context(), req)
	} else {
		resp, err = s.nodes.newCode(r.Context(), req.Name)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !exists {
		s.refreshInstructions(r.Context())
	}
	writeJSON(w, http.StatusOK, resp)
}

func pairCommand(agentURL, code string) string {
	return fmt.Sprintf("a2a pair add %s %s", agentURL, code)
}

func (s *Server) ctlTokens(w http.ResponseWriter, r *http.Request) {
	toks, err := s.store.Tokens(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := []TokenInfo{}
	for _, t := range toks {
		out = append(out, TokenInfo{Name: t.Name, Principal: t.Principal, Roles: t.Roles, URL: t.URL, Created: t.Created, LastUsed: t.LastUsed})
	}
	writeJSON(w, http.StatusOK, out)
}

// ctlRevoke removes the webhook node called name, or revokes the tokens and
// links issued to it.
func (s *Server) ctlRevoke(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	err := s.store.DeleteNode(r.Context(), name)
	if err == nil {
		s.refreshInstructions(r.Context())
		writeJSON(w, http.StatusOK, RevokeResponse{Kind: PairKindNode})
		return
	}
	if !errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	n, err := s.store.RevokeTokens(r.Context(), name, auth.PrincipalForCode(name, auth.RoleMember))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "no token or link named "+name)
		return
	}
	writeJSON(w, http.StatusOK, RevokeResponse{Kind: PairKindCaller})
}

func (s *Server) ctlPeers(w http.ResponseWriter, r *http.Request) {
	peers, err := s.peers.discover(r.Context())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, peers)
}

func (s *Server) ctlContacts(w http.ResponseWriter, r *http.Request) {
	cs, err := s.store.Contacts(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := []ContactInfo{}
	for _, c := range cs {
		out = append(out, ContactInfo{Name: c.Name, URL: c.URL, Via: c.Via, HasToken: c.Token != ""})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) ctlAddContact(w http.ResponseWriter, r *http.Request) {
	var req ContactRequest
	if !readJSON(w, r, &req) {
		return
	}
	c, err := s.peers.addContact(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.refreshInstructions(r.Context())
	writeJSON(w, http.StatusOK, ContactInfo{Name: c.Name, URL: c.URL, Via: c.Via, HasToken: c.Token != ""})
}

func (s *Server) ctlDeleteContact(w http.ResponseWriter, r *http.Request) {
	err := s.store.DeleteContact(r.Context(), r.PathValue("name"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "no contact "+r.PathValue("name"))
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.refreshInstructions(r.Context())
	writeJSON(w, http.StatusOK, map[string]string{"deleted": r.PathValue("name")})
}

func (s *Server) ctlAddChannel(w http.ResponseWriter, r *http.Request) {
	var req ChannelRequest
	if !readJSON(w, r, &req) {
		return
	}
	secrets, err := s.dir.LoadSecrets()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	next := s.config()
	if err := next.SetChannel(req.Kind, req.Token, req.Email); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ch, err := s.buildChannel(next, secrets, req.Kind)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if v, ok := ch.(channel.Validator); ok {
		if err := v.Validate(r.Context()); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	s.cfgMu.Lock()
	s.secrets = secrets
	s.cfgMu.Unlock()
	if err := s.updateConfig(func(c *agentdir.Config) error { return c.SetChannel(req.Kind, req.Token, req.Email) }); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.router.add(ch)
	writeJSON(w, http.StatusOK, s.router.channelStatus(ch, s.router.ownerHint(r.Context(), ch)))
}

func (s *Server) ctlDeleteChannel(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if err := (&agentdir.Config{}).SetChannel(kind, "", nil); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.updateConfig(func(c *agentdir.Config) error { return c.SetChannel(kind, "", nil) }); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.router.remove(kind)
	writeJSON(w, http.StatusOK, map[string]string{"removed": kind})
}

func (s *Server) ctlNodes(w http.ResponseWriter, r *http.Request) {
	nodes, err := s.store.Nodes(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := []NodeInfo{}
	for _, n := range nodes {
		out = append(out, s.nodes.info(n))
	}
	writeJSON(w, http.StatusOK, out)
}
