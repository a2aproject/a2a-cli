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

// Command fakeacp is a scripted ACP agent for the agent server's conformance
// tests. The prompt text selects the behavior:
//
//	hello            two message chunks
//	tool             a tool call with progress
//	ask              a permission request; the answer decides the reply
//	slow             chunks until canceled
//	remember <x>     stores x in the session
//	recall           replies with the stored value
//	crash            exits mid-turn
//	call <agent> <t> sends t to an agent through the agents URL of the prompt
//	acall <agent> <t> the same, without waiting for the result
//
// Sessions are kept in .fakeacp in the working directory, so session/load
// works after a restart.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
)

// agentsLinePrefix starts the line the agent server adds to every prompt.
const agentsLinePrefix = "[a2a agents]"

type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type session struct {
	Memory string `json:"memory"`
}

type agent struct {
	out     *json.Encoder
	outMu   sync.Mutex
	mu      sync.Mutex
	nextID  int
	pending map[string]chan message
	cancels map[string]chan struct{}
	dir     string
}

func main() {
	a := &agent{out: json.NewEncoder(os.Stdout), pending: map[string]chan message{}, cancels: map[string]chan struct{}{}}
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var m message
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			fmt.Fprintln(os.Stderr, "fakeacp: bad message:", err)
			continue
		}
		if m.Method == "" {
			a.mu.Lock()
			ch := a.pending[string(m.ID)]
			delete(a.pending, string(m.ID))
			a.mu.Unlock()
			if ch != nil {
				ch <- m
			}
			continue
		}
		go a.handle(m)
	}
}

func (a *agent) write(v any) {
	a.outMu.Lock()
	defer a.outMu.Unlock()
	if err := a.out.Encode(v); err != nil {
		fmt.Fprintln(os.Stderr, "fakeacp: write:", err)
		os.Exit(1)
	}
}

func (a *agent) respond(id json.RawMessage, result any) {
	a.write(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (a *agent) fail(id json.RawMessage, msg string) {
	a.write(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": -32603, "message": msg}})
}

func (a *agent) notify(sessionID string, update map[string]any) {
	a.write(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": sessionID, "update": update}})
}

func (a *agent) call(method string, params any) message {
	a.mu.Lock()
	a.nextID++
	id := fmt.Sprintf("%d", a.nextID)
	ch := make(chan message, 1)
	a.pending[id] = ch
	a.mu.Unlock()
	a.write(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "method": method, "params": params})
	return <-ch
}

func (a *agent) chunk(sessionID, text string) {
	a.notify(sessionID, map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": text}})
}

func (a *agent) sessionPath(id string) string { return filepath.Join(a.dir, ".fakeacp", id+".json") }

func (a *agent) load(id string) (*session, error) {
	data, err := os.ReadFile(a.sessionPath(id))
	if err != nil {
		return nil, err
	}
	var s session
	return &s, json.Unmarshal(data, &s)
}

func (a *agent) save(id string, s *session) error {
	if err := os.MkdirAll(filepath.Dir(a.sessionPath(id)), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(a.sessionPath(id), data, 0o600)
}

func (a *agent) handle(m message) {
	switch m.Method {
	case "initialize":
		a.respond(m.ID, map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]any{"loadSession": true}})
	case "session/new":
		var p struct {
			Cwd string `json:"cwd"`
		}
		if err := json.Unmarshal(m.Params, &p); err != nil {
			a.fail(m.ID, err.Error())
			return
		}
		a.dir = p.Cwd
		id := fmt.Sprintf("s%d", time.Now().UnixNano())
		if err := a.save(id, &session{}); err != nil {
			a.fail(m.ID, err.Error())
			return
		}
		a.respond(m.ID, map[string]any{"sessionId": id})
	case "session/load":
		var p struct {
			SessionID string `json:"sessionId"`
			Cwd       string `json:"cwd"`
		}
		if err := json.Unmarshal(m.Params, &p); err != nil {
			a.fail(m.ID, err.Error())
			return
		}
		a.dir = p.Cwd
		if _, err := a.load(p.SessionID); err != nil {
			a.fail(m.ID, "unknown session")
			return
		}
		a.respond(m.ID, map[string]any{})
	case "session/cancel":
		var p struct {
			SessionID string `json:"sessionId"`
		}
		if json.Unmarshal(m.Params, &p) == nil {
			a.mu.Lock()
			if ch := a.cancels[p.SessionID]; ch != nil {
				close(ch)
				delete(a.cancels, p.SessionID)
			}
			a.mu.Unlock()
		}
	case "session/prompt":
		a.prompt(m)
	default:
		if len(m.ID) > 0 {
			a.fail(m.ID, "method not found: "+m.Method)
		}
	}
}

func (a *agent) prompt(m message) {
	var p struct {
		SessionID string `json:"sessionId"`
		Prompt    []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"prompt"`
	}
	if err := json.Unmarshal(m.Params, &p); err != nil {
		a.fail(m.ID, err.Error())
		return
	}
	var texts []string
	agentsURL := ""
	for _, b := range p.Prompt {
		if b.Type != "text" {
			continue
		}
		if strings.HasPrefix(b.Text, agentsLinePrefix) {
			agentsURL = parseAgentsURL(b.Text)
			continue
		}
		texts = append(texts, b.Text)
	}
	text := strings.TrimSpace(strings.Join(texts, "\n"))
	sid := p.SessionID
	s, err := a.load(sid)
	if err != nil {
		a.fail(m.ID, "unknown session")
		return
	}
	canceled := make(chan struct{})
	a.mu.Lock()
	a.cancels[sid] = canceled
	a.mu.Unlock()
	end := func(reason string) { a.respond(m.ID, map[string]any{"stopReason": reason}) }

	cmd, arg, _ := strings.Cut(text, " ")
	switch strings.ToLower(cmd) {
	case "tool":
		a.notify(sid, map[string]any{"sessionUpdate": "tool_call", "toolCallId": "t1", "title": "list files", "kind": "read", "status": "pending"})
		a.notify(sid, map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "t1", "status": "in_progress"})
		a.notify(sid, map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "t1", "status": "completed"})
		a.chunk(sid, "Listed the files.")
		end("end_turn")
	case "ask":
		a.notify(sid, map[string]any{"sessionUpdate": "tool_call", "toolCallId": "t2", "title": "git push", "kind": "execute", "status": "pending", "rawInput": map[string]any{"command": "git push origin main"}})
		resp := a.call("session/request_permission", map[string]any{
			"sessionId": sid,
			"toolCall":  map[string]any{"toolCallId": "t2"},
			"options": []map[string]any{
				{"optionId": "allow", "name": "Allow", "kind": "allow_once"},
				{"optionId": "always", "name": "Always", "kind": "allow_always"},
				{"optionId": "deny", "name": "Deny", "kind": "reject_once"},
			},
		})
		var r struct {
			Outcome struct {
				Outcome  string `json:"outcome"`
				OptionID string `json:"optionId"`
			} `json:"outcome"`
		}
		if err := json.Unmarshal(resp.Result, &r); err != nil || r.Outcome.Outcome == "cancelled" {
			end("cancelled")
			return
		}
		if r.Outcome.OptionID == "deny" {
			a.notify(sid, map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "t2", "status": "failed"})
			a.chunk(sid, "Push denied.")
		} else {
			a.notify(sid, map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "t2", "status": "completed"})
			a.chunk(sid, "Pushed ("+r.Outcome.OptionID+").")
		}
		end("end_turn")
	case "slow":
		for i := 0; ; i++ {
			select {
			case <-canceled:
				end("cancelled")
				return
			case <-time.After(50 * time.Millisecond):
				a.chunk(sid, ".")
			}
		}
	case "remember":
		s.Memory = arg
		if err := a.save(sid, s); err != nil {
			a.fail(m.ID, err.Error())
			return
		}
		a.chunk(sid, "OK.")
		end("end_turn")
	case "recall":
		a.chunk(sid, "You said: "+s.Memory)
		end("end_turn")
	case "crash":
		a.chunk(sid, "about to crash")
		os.Exit(3)
	case "call", "acall":
		agentName, msg, _ := strings.Cut(arg, " ")
		out, err := callAgent(agentsURL, agentName, msg, strings.ToLower(cmd) == "acall")
		if err != nil {
			a.chunk(sid, "tool error: "+err.Error())
		} else {
			a.chunk(sid, "tool result: "+out)
		}
		end("end_turn")
	default:
		a.chunk(sid, "Hello ")
		a.chunk(sid, "from fakeacp.")
		end("end_turn")
	}
}

// parseAgentsURL returns the agents URL of the line the agent server adds
// to prompts.
func parseAgentsURL(line string) string {
	_, rest, ok := strings.Cut(line, "Agents URL for this conversation: ")
	if !ok {
		return ""
	}
	u, _, _ := strings.Cut(rest, " ")
	return u
}

// callAgent sends text to an agent the way the a2a CLI does: it resolves
// the agent's card under the agents URL and sends a message.
func callAgent(agentsURL, name, text string, async bool) (string, error) {
	if agentsURL == "" {
		return "", fmt.Errorf("no agents URL in the prompt")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	hc := httpClient()
	card, err := agentcard.NewResolver(hc).Resolve(ctx, agentsURL+"/"+name)
	if err != nil {
		return "", err
	}
	client, err := a2aclient.NewFromCard(ctx, card, a2aclient.WithJSONRPCTransport(hc), a2aclient.WithRESTTransport(hc))
	if err != nil {
		return "", err
	}
	defer func() {
		if err := client.Destroy(); err != nil {
			fmt.Fprintln(os.Stderr, "closing client:", err)
		}
	}()
	req := &a2a.SendMessageRequest{Message: a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart(text))}
	if async {
		req.Config = &a2a.SendMessageConfig{ReturnImmediately: true}
	}
	res, err := client.SendMessage(ctx, req)
	if err != nil {
		return "", err
	}
	switch r := res.(type) {
	case *a2a.Message:
		return partsText(r.Parts), nil
	case *a2a.Task:
		var parts []*a2a.Part
		for _, art := range r.Artifacts {
			parts = append(parts, art.Parts...)
		}
		if len(parts) == 0 && r.Status.Message != nil {
			parts = r.Status.Message.Parts
		}
		return fmt.Sprintf("task %s %s: %s", r.ID, r.Status.State, partsText(parts)), nil
	}
	return "", fmt.Errorf("unexpected result %T", res)
}

// httpClient honors A2ACLI_UNIX_SOCKET=<host>=<path> like the a2a CLI.
func httpClient() *http.Client {
	host, path, ok := strings.Cut(os.Getenv("A2ACLI_UNIX_SOCKET"), "=")
	if !ok {
		return http.DefaultClient
	}
	var d net.Dialer
	return &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		if h, _, err := net.SplitHostPort(addr); err == nil && h == host {
			return d.DialContext(ctx, "unix", path)
		}
		return d.DialContext(ctx, network, addr)
	}}}
}

func partsText(parts []*a2a.Part) string {
	var out []string
	for _, p := range parts {
		if t := p.Text(); t != "" {
			out = append(out, t)
		}
	}
	return strings.Join(out, "")
}
