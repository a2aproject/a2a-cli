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
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/a2aproject/a2a-cli/internal/agentdir"
	"github.com/a2aproject/a2a-cli/internal/network"
)

// ErrNotRunning is returned when no agent server answers on the control socket.
var ErrNotRunning = errors.New("the agent server is not running; start it with `a2a server`")

// Client talks to a running agent server over its control socket.
type Client struct {
	dir agentdir.Dir
	hc  *http.Client
}

// NewClient returns a client for the agent in dir.
func NewClient(dir agentdir.Dir) *Client {
	path := dir.SocketPath()
	return &Client{dir: dir, hc: &http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", path)
		}},
	}}
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://agent"+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		var opErr *net.OpError
		if errors.As(err, &opErr) {
			return ErrNotRunning
		}
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Debug("closing response", "error", err)
		}
	}()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		var apiErr APIError
		if json.Unmarshal(data, &apiErr) == nil && apiErr.Error != "" {
			return errors.New(apiErr.Error)
		}
		return fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

// Status returns the status of the server.
func (c *Client) Status(ctx context.Context) (*Status, error) {
	var out Status
	return &out, c.do(ctx, http.MethodGet, "/status", nil, &out)
}

// Stop stops the server.
func (c *Client) Stop(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/stop", nil, nil)
}

// Pair mints a pairing code.
func (c *Client) Pair(ctx context.Context, req PairCodeRequest) (*PairCodeResponse, error) {
	var out PairCodeResponse
	return &out, c.do(ctx, http.MethodPost, "/pair", req, &out)
}

// Tokens lists issued tokens.
func (c *Client) Tokens(ctx context.Context) ([]TokenInfo, error) {
	var out []TokenInfo
	return out, c.do(ctx, http.MethodGet, "/tokens", nil, &out)
}

// Revoke removes the webhook node called name, or revokes the tokens and
// links issued under it.
func (c *Client) Revoke(ctx context.Context, name string) (*RevokeResponse, error) {
	var out RevokeResponse
	return &out, c.do(ctx, http.MethodDelete, "/pair/"+url.PathEscape(name), nil, &out)
}

// Peers lists agents found on the private network.
func (c *Client) Peers(ctx context.Context) ([]network.PeerInfo, error) {
	var out []network.PeerInfo
	return out, c.do(ctx, http.MethodGet, "/peers", nil, &out)
}

// Contacts lists contacts.
func (c *Client) Contacts(ctx context.Context) ([]ContactInfo, error) {
	var out []ContactInfo
	return out, c.do(ctx, http.MethodGet, "/contacts", nil, &out)
}

// AddContact adds a contact.
func (c *Client) AddContact(ctx context.Context, req ContactRequest) (*ContactInfo, error) {
	var out ContactInfo
	return &out, c.do(ctx, http.MethodPost, "/contacts", req, &out)
}

// DeleteContact removes a contact.
func (c *Client) DeleteContact(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/contacts/"+url.PathEscape(name), nil, nil)
}

// AddChannel adds or replaces a channel.
func (c *Client) AddChannel(ctx context.Context, req ChannelRequest) (*ChannelStatus, error) {
	var out ChannelStatus
	return &out, c.do(ctx, http.MethodPost, "/channels", req, &out)
}

// DeleteChannel removes a channel.
func (c *Client) DeleteChannel(ctx context.Context, kind string) error {
	return c.do(ctx, http.MethodDelete, "/channels/"+url.PathEscape(kind), nil, nil)
}

// Nodes lists webhook nodes.
func (c *Client) Nodes(ctx context.Context) ([]NodeInfo, error) {
	var out []NodeInfo
	return out, c.do(ctx, http.MethodGet, "/nodes", nil, &out)
}
