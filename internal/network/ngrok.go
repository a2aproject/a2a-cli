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

package network

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"

	"golang.ngrok.com/ngrok/v2"
)

// Ngrok is a [Provider] for a public URL through ngrok, embedded. A free
// account includes one static domain.
type Ngrok struct {
	AuthToken string
	// URL is a reserved domain, such as https://coder.ngrok.app. Empty gets
	// a random one.
	URL         string
	Description string

	mu    sync.Mutex
	agent ngrok.Agent
	ln    ngrok.EndpointListener
}

var _ Provider = (*Ngrok)(nil)

// Name implements [Provider].
func (n *Ngrok) Name() string { return "ngrok" }

// ListenPrivate implements [Provider].
func (n *Ngrok) ListenPrivate(context.Context) (net.Listener, *url.URL, error) {
	return nil, nil, ErrUnsupported
}

// ListenPublic implements [Provider].
func (n *Ngrok) ListenPublic(ctx context.Context) (net.Listener, *url.URL, error) {
	if n.AuthToken == "" {
		return nil, nil, errors.New("ngrok needs an authtoken: pass --secret NGROK_AUTHTOKEN (https://dashboard.ngrok.com/get-started/your-authtoken)")
	}
	agent, err := ngrok.NewAgent(ngrok.WithAuthtoken(n.AuthToken))
	if err != nil {
		return nil, nil, fmt.Errorf("ngrok: %w", err)
	}
	var opts []ngrok.EndpointOption
	if n.URL != "" {
		opts = append(opts, ngrok.WithURL(n.URL))
	}
	if n.Description != "" {
		opts = append(opts, ngrok.WithDescription(n.Description))
	}
	ln, err := agent.Listen(ctx, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("ngrok: %w", err)
	}
	n.mu.Lock()
	n.agent, n.ln = agent, ln
	n.mu.Unlock()
	return ln, ln.URL(), nil
}

// WhoIs implements [Provider].
func (n *Ngrok) WhoIs(context.Context, string) (*Peer, error) { return nil, ErrNoIdentity }

// Peers implements [Provider].
func (n *Ngrok) Peers(context.Context) ([]PeerInfo, error) { return nil, ErrUnsupported }

// HTTPClient implements [Provider].
func (n *Ngrok) HTTPClient() *http.Client { return http.DefaultClient }

// Close implements [Provider].
func (n *Ngrok) Close() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.agent == nil {
		return nil
	}
	return n.agent.Disconnect()
}
