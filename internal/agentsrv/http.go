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
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"github.com/a2aproject/a2a-cli/internal/auth"
	"github.com/a2aproject/a2a-cli/internal/network"
	"github.com/a2aproject/a2a-cli/internal/store"
	"github.com/a2aproject/a2a-cli/skills"
)

// routes returns the HTTP handler of one listener. baseURL is the URL the
// listener is reached at; empty derives it from the request.
func (s *Server) routes(kind, baseURL string) http.Handler {
	mux := http.NewServeMux()
	if kind == network.ListenerHarness {
		agents := s.peers.agentsHandler()
		mux.Handle("/{handle}/{agent}", agents)
		mux.Handle("/{handle}/{agent}/{rest...}", agents)
		return mux
	}
	card := func(w http.ResponseWriter, r *http.Request) {
		a2asrv.NewStaticAgentCardHandler(s.card(kind, base(baseURL, r))).ServeHTTP(w, r)
	}
	jsonrpc := a2asrv.NewJSONRPCHandler(s.handler, a2asrv.WithTransportKeepAlive(15*time.Second))
	rest := a2asrv.NewRESTHandler(s.handler, a2asrv.WithTransportKeepAlive(15*time.Second))

	mux.HandleFunc("GET /{$}", card)
	mux.HandleFunc("GET "+a2asrv.WellKnownAgentCardPath, card)
	mux.HandleFunc("GET /.well-known/agent.json", card)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /skill.md", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		writeBody(w, skills.A2ACLI)
	})
	mux.HandleFunc("GET /relay/skill.md", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		writeBody(w, strings.ReplaceAll(skills.A2ARelay, "{{BASE}}", base(baseURL, r)))
	})
	mux.HandleFunc("GET /download/a2a", s.handleDownload)
	mux.HandleFunc("POST /pair", func(w http.ResponseWriter, r *http.Request) { s.handlePair(w, r, base(baseURL, r)) })
	mux.HandleFunc("/hitl/{token}", s.handleHitl)
	s.nodes.register(mux, kind, func(r *http.Request) string { return base(baseURL, r) })
	mux.Handle("POST /{$}", s.authenticate(kind, jsonrpc))
	mux.Handle("/", s.authenticate(kind, rest))
	return mux
}

// base returns the configured base URL or derives it from the request.
func base(baseURL string, r *http.Request) string {
	if baseURL != "" {
		return baseURL
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// authenticate resolves the principal of a request with the auth chain.
func (s *Server) authenticate(kind string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok, err := s.chain.Authenticate(r.Context(), auth.Request{Listener: kind, RemoteAddr: r.RemoteAddr, Header: r.Header})
		if err != nil {
			slog.Error("authentication failed", "error", err)
			writeError(w, http.StatusInternalServerError, "authentication failed")
			return
		}
		if !ok {
			w.Header().Set("WWW-Authenticate", `Bearer realm="a2a"`)
			writeError(w, http.StatusUnauthorized, "this agent needs a token: ask its owner for a pairing code and run `a2a pair add <agent-url> <code>`")
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), p)))
	})
}

// card builds the agent card served at baseURL.
func (s *Server) card(kind, baseURL string) *a2a.AgentCard {
	cfg := s.config()
	desc := cfg.Description
	if desc == "" {
		desc = cfg.Name + " is a " + cfg.Harness + " coding agent reached through `a2a server`."
	}
	card := &a2a.AgentCard{
		Name:        cfg.Name,
		Description: desc,
		Version:     "1.0.0",
		SupportedInterfaces: []*a2a.AgentInterface{
			a2a.NewAgentInterface(baseURL, a2a.TransportProtocolJSONRPC),
			a2a.NewAgentInterface(baseURL, a2a.TransportProtocolHTTPJSON),
		},
		Capabilities:       a2a.AgentCapabilities{Streaming: true},
		DefaultInputModes:  []string{"text/plain", "application/octet-stream"},
		DefaultOutputModes: []string{"text/plain", "application/json"},
		Skills: []a2a.AgentSkill{{
			ID:          "coding",
			Name:        "Coding agent",
			Description: "Works in its project workspace with " + cfg.Harness + ". Tool calls may pause the task in input-required until an approver answers.",
			Tags:        []string{"coding", cfg.Harness},
		}},
	}
	if kind != network.ListenerPrivate {
		card.SecuritySchemes = a2a.NamedSecuritySchemes{
			"pairing": a2a.HTTPAuthSecurityScheme{Scheme: "Bearer", Description: "A token from pairing: `a2a pair add <agent-url> <code>`, with a code from the owner."},
		}
		card.SecurityRequirements = a2a.SecurityRequirementsOptions{{"pairing": {}}}
	}
	return card
}

// PairRequest is the body of POST /pair.
type PairRequest struct {
	Code string `json:"code"`
}

// Kinds of tokens POST /pair returns.
const (
	PairKindCaller = "caller"
	PairKindNode   = "node"
)

// PairResponse is returned by POST /pair. The token is shown once. URL is
// where the token applies: the agent for callers, the node for nodes.
type PairResponse struct {
	Kind      string `json:"kind"`
	Token     string `json:"token"`
	Principal string `json:"principal"`
	Agent     string `json:"agent"`
	URL       string `json:"url"`
}

// handlePair exchanges a pair code for a caller token, or a node code for a
// node token.
func (s *Server) handlePair(w http.ResponseWriter, r *http.Request, baseURL string) {
	host := remoteHost(r.RemoteAddr)
	if !s.pairFailures.allow(host) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "too many invalid pairing codes; try again in a minute")
		return
	}
	var req PairRequest
	if !readJSON(w, r, &req) {
		return
	}
	c, err := s.store.TakeCode(r.Context(), auth.NormalizeCode(req.Code), "")
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrCodeExpired) {
		s.pairFailures.fail(host)
		writeError(w, http.StatusForbidden, "invalid or expired pairing code")
		return
	}
	if err != nil {
		slog.Error("pairing failed", "error", err)
		writeError(w, http.StatusInternalServerError, "pairing failed")
		return
	}
	if c.Purpose == auth.PurposeNode {
		s.nodes.pair(w, r, baseURL, c.Name)
		return
	}
	tok, p, err := auth.IssueToken(r.Context(), s.store, c, baseURL)
	if err != nil {
		slog.Error("pairing failed", "error", err)
		writeError(w, http.StatusInternalServerError, "pairing failed")
		return
	}
	slog.Info("paired", "principal", p.ID, "url", baseURL)
	writeJSON(w, http.StatusOK, PairResponse{Kind: PairKindCaller, Token: tok, Principal: p.ID, Agent: s.config().Name, URL: baseURL})
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
		slog.Debug("writing response", "error", err)
	}
}

// APIError is the body of an error response.
type APIError struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, APIError{Error: msg})
}

func writeBody(w io.Writer, body string) {
	if _, err := io.WriteString(w, body); err != nil {
		slog.Debug("writing response", "error", err)
	}
}

func trimSlash(u string) string { return strings.TrimSuffix(u, "/") }
