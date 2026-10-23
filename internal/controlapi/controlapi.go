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

// Package controlapi is the API of an agent server's control socket, through
// which management commands change the running server without a restart.
package controlapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"time"

	"github.com/a2aproject/a2a-cli/internal/agentdir"
	"github.com/a2aproject/a2a-cli/internal/fsutil"
	"github.com/a2aproject/a2a-cli/internal/jsonhttp"
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
	// Kind is [pairapi.KindCaller] or [pairapi.KindNode].
	Kind string `json:"kind"`
}

// Peer is an agent found on the private network.
type Peer struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
	Online      bool   `json:"online"`
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

// PairCommand is the command a caller or a node runs to pair with code.
func PairCommand(agentURL, code string) string {
	return fmt.Sprintf("a2a pair add %s %s", agentURL, code)
}

// ChannelRequestFromConfig is the request which starts the channel of kind
// as configured in conf.
func ChannelRequestFromConfig(conf *agentdir.Config, kind string) ChannelRequest {
	return ChannelRequest{Kind: kind, Token: conf.ChannelToken(kind), Email: conf.Channels.Email}
}

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
			if err := fsutil.CheckPrivate(filepath.Dir(path)); err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return nil, &net.OpError{Op: "dial", Net: "unix", Err: err}
				}
				return nil, fmt.Errorf("not trusting the control socket %s: %w", path, err)
			}
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
		return errors.New(jsonhttp.ErrorMessage(resp.Status, data))
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
func (c *Client) Peers(ctx context.Context) ([]Peer, error) {
	var out []Peer
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
