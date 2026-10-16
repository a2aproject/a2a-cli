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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

// Client calls the relay API as a node (with a node token) or as the owner
// (with the admin token, setting NodeID to act on a node).
type Client struct {
	BaseURL string
	Token   string
	NodeID  string
	HTTP    *http.Client
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
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(c.BaseURL, "/")+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if c.NodeID != "" {
		req.Header.Set("X-A2A-Relay-Node", c.NodeID)
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 60 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer closeBody(resp)
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		var apiErr APIError
		if json.Unmarshal(data, &apiErr) == nil && apiErr.Error != "" {
			return fmt.Errorf("relay: %s", apiErr.Error)
		}
		return fmt.Errorf("relay: %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

// CreateNode attaches a node. It needs the admin token.
func (c *Client) CreateNode(ctx context.Context, req CreateNodeRequest) (*PairingResponse, error) {
	var out PairingResponse
	return &out, c.do(ctx, http.MethodPost, "/api/nodes", req, &out)
}

// NewPairing issues a new pairing code for a node. It needs the admin token.
func (c *Client) NewPairing(ctx context.Context, nodeID string) (*PairingResponse, error) {
	var out PairingResponse
	return &out, c.do(ctx, http.MethodPost, "/api/nodes/"+url.PathEscape(nodeID)+"/pairing", nil, &out)
}

// Pair exchanges a pairing code for a node token.
func (c *Client) Pair(ctx context.Context, code string) (*PairResponse, error) {
	var out PairResponse
	return &out, c.do(ctx, http.MethodPost, "/api/pair", PairRequest{Code: code}, &out)
}

// Node describes the node.
func (c *Client) Node(ctx context.Context) (*NodeInfo, error) {
	var out NodeInfo
	return &out, c.do(ctx, http.MethodGet, "/api/node", nil, &out)
}

// UpdateNode changes the node's webhook, name, description or access.
func (c *Client) UpdateNode(ctx context.Context, req UpdateNodeRequest) (*NodeInfo, error) {
	var out NodeInfo
	return &out, c.do(ctx, http.MethodPatch, "/api/node", req, &out)
}

// Ping sends the node a test wake and reports the first delivery attempt.
func (c *Client) Ping(ctx context.Context) (*InboxEntry, error) {
	var out InboxEntry
	return &out, c.do(ctx, http.MethodPost, "/api/node/ping", nil, &out)
}

// PostEvent posts an event for a task addressed to the node.
func (c *Client) PostEvent(ctx context.Context, taskID string, ev Event) (*EventResult, error) {
	var out EventResult
	return &out, c.do(ctx, http.MethodPost, "/api/tasks/"+url.PathEscape(taskID)+"/events", ev, &out)
}

// Task returns a task addressed to the node.
func (c *Client) Task(ctx context.Context, taskID string) (*a2a.Task, error) {
	var out a2a.Task
	return &out, c.do(ctx, http.MethodGet, "/api/tasks/"+url.PathEscape(taskID), nil, &out)
}

// Inbox lists wakes that weren't delivered or whose task waits for the node,
// or all recent wakes.
func (c *Client) Inbox(ctx context.Context, all bool) (*Inbox, error) {
	path := "/api/inbox"
	if all {
		path += "?all=1"
	}
	var out Inbox
	return &out, c.do(ctx, http.MethodGet, path, nil, &out)
}

// Send sends a message to a peer. The result arrives as a result wake.
func (c *Client) Send(ctx context.Context, req SendRequest) (*SendResponse, error) {
	var out SendResponse
	return &out, c.do(ctx, http.MethodPost, "/api/send", req, &out)
}

// Grants lists the names allowed to call the node.
func (c *Client) Grants(ctx context.Context) ([]string, error) {
	var out struct {
		Grants []string `json:"grants"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/grants", nil, &out); err != nil {
		return nil, err
	}
	return out.Grants, nil
}

// Grant allows a caller and returns its token, or allows another node.
func (c *Client) Grant(ctx context.Context, name string) (*GrantResponse, error) {
	var out GrantResponse
	return &out, c.do(ctx, http.MethodPost, "/api/grants", GrantRequest{Name: name}, &out)
}

// Revoke removes a grant.
func (c *Client) Revoke(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/api/grants/"+url.PathEscape(name), nil, nil)
}

func closeBody(resp *http.Response) {
	if err := resp.Body.Close(); err != nil {
		slog.Debug("closing response body", "error", err)
	}
}
