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

// Package network defines how an agent server is reached beyond localhost:
// a private network of the owner's machines and team, and the public
// internet. Each [Provider] implements what it supports.
package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

// ErrUnsupported is returned by providers for capabilities they don't have.
var ErrUnsupported = errors.New("not supported by this network provider")

// ErrNoIdentity is returned by WhoIs when the caller can't be identified.
var ErrNoIdentity = errors.New("caller has no verified network identity")

// Peer is a verified caller on the private network.
type Peer struct {
	// ID is the login of the user who owns the calling machine.
	ID      string
	Machine string
	// Caps are the caller's application capabilities granted by the
	// network's policy, as raw JSON values per capability name.
	Caps map[string][]json.RawMessage
	// Self is true when the caller belongs to the same user as this server.
	Self bool
}

// PeerInfo is another agent server found on the private network.
type PeerInfo struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
	Online      bool   `json:"online"`
}

// Provider is one way of reaching the agent server. It implements what it
// supports and returns [ErrUnsupported] for the rest.
type Provider interface {
	// Name is the provider name, such as tailscale.
	Name() string
	// ListenPrivate listens on a network limited to the owner's machines or team.
	ListenPrivate(ctx context.Context) (net.Listener, *url.URL, error)
	// ListenPublic listens on the internet, with TLS terminated.
	ListenPublic(ctx context.Context) (net.Listener, *url.URL, error)
	// WhoIs identifies a caller on the private listener, or returns ErrNoIdentity.
	WhoIs(ctx context.Context, remoteAddr string) (*Peer, error)
	// Peers lists other agent servers reachable on the private network.
	Peers(ctx context.Context) ([]PeerInfo, error)
	// HTTPClient makes outbound calls to peers.
	HTTPClient() *http.Client
	// Close stops the provider.
	Close() error
}

// Listener kinds, which tell authenticators how a request arrived.
const (
	ListenerInProcess = "inprocess"
	ListenerLocal     = "local"
	ListenerPrivate   = "private"
	ListenerPublic    = "public"
	// ListenerHarness serves the agent's own harness its calls to other agents.
	ListenerHarness = "harness"
)

// ProbeAgents keeps the candidates which serve an A2A agent card, filling in
// their name and description from it.
func ProbeAgents(ctx context.Context, client *http.Client, candidates []PeerInfo) []PeerInfo {
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		out []PeerInfo
	)
	for _, c := range candidates {
		wg.Go(func() {
			card, err := FetchCard(ctx, client, c.URL)
			if err != nil {
				return
			}
			c.Description = card.Description
			if card.Name != "" {
				c.Name = card.Name
			}
			mu.Lock()
			out = append(out, c)
			mu.Unlock()
		})
	}
	wg.Wait()
	return out
}

// FetchCard fetches the agent card served at base.
func FetchCard(ctx context.Context, client *http.Client, base string) (*a2a.AgentCard, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(base, "/")+a2asrv.WellKnownAgentCardPath, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Debug("closing card response", "error", err)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", base, resp.Status)
	}
	var card a2a.AgentCard
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&card); err != nil {
		return nil, err
	}
	return &card, nil
}

// splitListener is a net.Listener fed from another listener's accept loop.
type splitListener struct {
	addr   net.Addr
	conns  chan net.Conn
	done   chan struct{}
	once   sync.Once
	closer func() error
}

func newSplitListener(addr net.Addr, closer func() error) *splitListener {
	return &splitListener{addr: addr, conns: make(chan net.Conn), done: make(chan struct{}), closer: closer}
}

func (l *splitListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *splitListener) Close() error {
	var err error
	l.once.Do(func() {
		close(l.done)
		if l.closer != nil {
			err = l.closer()
		}
	})
	return err
}

func (l *splitListener) Addr() net.Addr { return l.addr }

// deliver hands c to the listener, or closes it if the listener is closed.
func (l *splitListener) deliver(c net.Conn) {
	select {
	case l.conns <- c:
	case <-l.done:
		if err := c.Close(); err != nil {
			slog.Debug("closing connection", "error", err)
		}
	}
}
