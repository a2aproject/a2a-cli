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

package a2aopencode

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// client talks to the HTTP API of `opencode serve`.
type client struct {
	baseURL string
	dir     string
	http    *http.Client
}

// event is an item of the /event SSE stream.
type event struct {
	Type       string          `json:"type"`
	Properties json.RawMessage `json:"properties"`
}

func (c *client) do(ctx context.Context, method, path string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, r)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.dir != "" {
		req.Header.Set("x-opencode-directory", url.PathEscape(c.dir))
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer closeBody(resp)
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(msg)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (c *client) waitHealthy(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		if err := c.do(ctx, http.MethodGet, "/global/health", nil, nil); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("opencode server at %s did not become healthy: %w", c.baseURL, ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// PermissionRule is an opencode permission rule, applied to sessions the executor creates.
type PermissionRule struct {
	Permission string `json:"permission"`
	Pattern    string `json:"pattern"`
	Action     string `json:"action"` // allow, ask or deny
}

func (c *client) createSession(ctx context.Context, title string, rules []PermissionRule) (string, error) {
	body := map[string]any{"title": title}
	if len(rules) > 0 {
		body["permission"] = rules
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := c.do(ctx, http.MethodPost, "/session", body, &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

func (c *client) sessionExists(ctx context.Context, id string) bool {
	return c.do(ctx, http.MethodGet, "/session/"+id, nil, nil) == nil
}

type promptRequest struct {
	Parts []map[string]any `json:"parts"`
	Model *modelRef        `json:"model,omitempty"`
	Agent string           `json:"agent,omitempty"`
}

type modelRef struct {
	ProviderID string `json:"providerID"`
	ModelID    string `json:"modelID"`
}

func (c *client) promptAsync(ctx context.Context, sessionID string, req promptRequest) error {
	return c.do(ctx, http.MethodPost, "/session/"+sessionID+"/prompt_async", req, nil)
}

func (c *client) abort(ctx context.Context, sessionID string) error {
	return c.do(ctx, http.MethodPost, "/session/"+sessionID+"/abort", nil, nil)
}

func (c *client) replyPermission(ctx context.Context, id, reply, message string) error {
	body := map[string]any{"reply": reply}
	if message != "" {
		body["message"] = message
	}
	return c.do(ctx, http.MethodPost, "/permission/"+id+"/reply", body, nil)
}

func (c *client) replyQuestion(ctx context.Context, id string, answers [][]string) error {
	return c.do(ctx, http.MethodPost, "/question/"+id+"/reply", map[string]any{"answers": answers}, nil)
}

func (c *client) rejectQuestion(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/question/"+id+"/reject", nil, nil)
}

// subscribe streams /event until ctx is done, reconnecting on errors.
func (c *client) subscribe(ctx context.Context, handle func(event)) {
	backoff := 100 * time.Millisecond
	for ctx.Err() == nil {
		err := c.readEvents(ctx, handle, func() { backoff = 100 * time.Millisecond })
		if ctx.Err() != nil {
			return
		}
		slog.Warn("opencode event stream interrupted, reconnecting", "error", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 5*time.Second)
	}
}

func (c *client) readEvents(ctx context.Context, handle func(event), connected func()) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/event", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	if c.dir != "" {
		req.Header.Set("x-opencode-directory", url.PathEscape(c.dir))
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer closeBody(resp)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET /event: %s", resp.Status)
	}
	connected()
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 32*1024*1024)
	var data strings.Builder
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "data:"):
			data.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		case line == "" && data.Len() > 0:
			var e event
			if err := json.Unmarshal([]byte(data.String()), &e); err == nil {
				handle(e)
			}
			data.Reset()
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return io.EOF
}

// queue is an unbounded FIFO of events for one session, so the shared event
// stream never blocks on a slow consumer.
type queue struct {
	mu     sync.Mutex
	items  []event
	notify chan struct{}
}

func newQueue() *queue { return &queue{notify: make(chan struct{}, 1)} }

func (q *queue) push(e event) {
	q.mu.Lock()
	q.items = append(q.items, e)
	q.mu.Unlock()
	select {
	case q.notify <- struct{}{}:
	default:
	}
}

func (q *queue) pop(ctx context.Context) (event, error) {
	for {
		q.mu.Lock()
		if len(q.items) > 0 {
			e := q.items[0]
			q.items = q.items[1:]
			q.mu.Unlock()
			return e, nil
		}
		q.mu.Unlock()
		select {
		case <-ctx.Done():
			return event{}, ctx.Err()
		case <-q.notify:
		}
	}
}

func closeBody(resp *http.Response) {
	if err := resp.Body.Close(); err != nil {
		slog.Debug("closing opencode response body", "error", err)
	}
}
