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

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"

	"github.com/a2aproject/a2a-cli/internal/clicfg"
	"github.com/a2aproject/a2a-cli/internal/flagparse"
	"github.com/a2aproject/a2a-cli/internal/localsrv"
	"github.com/a2aproject/a2a-cli/internal/output"
	"github.com/a2aproject/a2a-cli/internal/polling"
	"github.com/a2aproject/a2a-cli/internal/testutil"
	a2acorev0 "github.com/a2aproject/a2a-go/a2a"
	a2asrvv0 "github.com/a2aproject/a2a-go/a2asrv"
	"github.com/a2aproject/a2a-go/a2asrv/eventqueue"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2acompat/a2av0"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

func TestCardGet(t *testing.T) {
	t.Parallel()
	url := startTestServer(t)
	legacyURL := startLegacyTestServer(t)

	modes := []struct {
		suffix string
		url    string
	}{{suffix: " v1", url: url}, {suffix: " v0", url: legacyURL}}

	for _, mode := range modes {
		t.Run("returns agent card"+mode.suffix, func(t *testing.T) {
			t.Parallel()
			out := mustRunCMD(t, "card", "get", mode.url, "-o", "json")
			var card a2a.AgentCard
			if err := json.Unmarshal([]byte(out), &card); err != nil {
				t.Fatalf("json.Unmarshal(card get output) error = %v", err)
			}
			if card.Name != "Test Echo" {
				t.Errorf("a2a card get card.Name = %q, want %q", card.Name, "Test Echo")
			}
			if !card.Capabilities.Streaming {
				t.Errorf("a2a card get card.Capabilities.Streaming = false, want true")
			}
			if len(card.SupportedInterfaces) == 0 {
				t.Errorf("a2a card get supported interfaces is empty")
			}
		})

		t.Run("returns agent card with complete card url"+mode.suffix, func(t *testing.T) {
			t.Parallel()
			out := mustRunCMD(t, "card", "get", mode.url+"/.well-known/agent-card.json", "-o", "json")
			var card a2a.AgentCard
			if err := json.Unmarshal([]byte(out), &card); err != nil {
				t.Fatalf("json.Unmarshal(card get output) error = %v", err)
			}
			if card.Name != "Test Echo" {
				t.Fatalf("a2a card get card.Name = %q, want %q", card.Name, "Test Echo")
			}
		})

		t.Run("accepts --agent-card flag"+mode.suffix, func(t *testing.T) {
			t.Parallel()
			out := mustRunCMD(t, "card", "get", "-a", mode.url, "-o", "json")
			var card a2a.AgentCard
			if err := json.Unmarshal([]byte(out), &card); err != nil {
				t.Fatalf("json.Unmarshal(card get output) error = %v", err)
			}
			if card.Name != "Test Echo" {
				t.Fatalf("a2a card get -a card.Name = %q, want %q", card.Name, "Test Echo")
			}
		})
	}

	t.Run("missing agent fails", func(t *testing.T) {
		t.Parallel()
		if _, err := runCMD(t, "card", "get"); err == nil {
			t.Fatal("a2a card get (no url) should fail")
		}
	})

	t.Run("extended with positional url", func(t *testing.T) {
		t.Parallel()
		_, err := runCMD(t, "card", "get", url, "--extended")
		if err == nil {
			t.Fatal("card get <url> --extended against a server without an extended card should fail")
		}
		if strings.Contains(err.Error(), "must be provided") {
			t.Fatalf("card get <url> --extended ignored the positional url: %v", err)
		}
	})

	agentCardFilePath := testutil.MustWriteTmpCardFile(t, newAgentCard(url, a2a.AgentCapabilities{
		Streaming: true,
	}))
	t.Run("with positional file path", func(t *testing.T) {
		out := mustRunCMD(t, "card", "get", agentCardFilePath, "-o", "json")
		var card a2a.AgentCard
		if err := json.Unmarshal([]byte(out), &card); err != nil {
			t.Fatalf("json.Unmarshal(card get output) error = %v", err)
		}
		if card.Name != "Test Echo" {
			t.Fatalf("a2a card get -a card.Name = %q, want %q", card.Name, "Test Echo")
		}
	})
}

func TestVersion(t *testing.T) {
	t.Parallel()

	t.Run("text output", func(t *testing.T) {
		t.Parallel()
		out := mustRunCMD(t, "version")
		if !strings.HasPrefix(out, "a2a ") {
			t.Fatalf("a2a version output = %q, want it to start with %q", out, "a2a ")
		}
	})

	t.Run("json output", func(t *testing.T) {
		t.Parallel()
		out := mustRunCMD(t, "version", "-o", "json")
		var info versionInfo
		if err := json.Unmarshal([]byte(out), &info); err != nil {
			t.Fatalf("json.Unmarshal(version output) error = %v", err)
		}
		if info.Version == "" {
			t.Fatalf("a2a version info.Version = %q, want non-empty", info.Version)
		}
		if info.GoVersion == "" {
			t.Fatalf("a2a version info.GoVersion = %q, want non-empty", info.GoVersion)
		}
	})
}

func TestSend(t *testing.T) {
	t.Parallel()
	url := startTestServer(t)
	legacyURL := startLegacyTestServer(t)

	msgText := "hello hello!"
	reqJSON := fmt.Sprintf(`{"message":{"role":"ROLE_USER","parts":[{"text":"%s"}]}}`, msgText)

	sendTests := []struct {
		name     string
		args     func(url string) []string
		wantText string
		wantErr  bool
	}{
		{
			name: "text",
			args: func(url string) []string {
				return []string{"send", "-a", url, "-o", "json", msgText}
			},
			wantText: msgText,
		},
		{
			name: "text parts preserve order",
			args: func(url string) []string {
				return []string{"send", "-a", url, "-o", "json", "--text-part", "part one", "--text-part", "part two"}
			},
			wantText: "part one part two",
		},
		{
			name: "request payload json",
			args: func(url string) []string {
				return []string{"send", "-a", url, "-o", "json", "--request-payload", reqJSON}
			},
			wantText: msgText,
		},
		{
			name: "fails when no message",
			args: func(url string) []string {
				return []string{"send", "-a", url}
			},
			wantErr: true,
		},
		{
			name: "fails when no agent",
			args: func(url string) []string {
				return []string{"send", "hello"}
			},
			wantErr: true,
		},
		{
			name: "fails on bad --request-payload",
			args: func(url string) []string {
				return []string{"send", "-a", url, "--request-payload", "{bad"}
			},
			wantErr: true,
		},
		{
			name: "fails when --request-payload combined with a part flag",
			args: func(url string) []string {
				return []string{"send", "-a", url, "--request-payload", reqJSON, "--text-part", "extra"}
			},
			wantErr: true,
		},
	}

	modes := []struct {
		suffix string
		url    string
	}{{suffix: "_v1", url: url}, {suffix: "_v0", url: legacyURL}}

	for _, mode := range modes {
		for _, tt := range sendTests {
			t.Run(tt.name+mode.suffix, func(t *testing.T) {
				t.Parallel()
				out, err := runCMD(t, tt.args(mode.url)...)
				if err != nil && tt.wantErr {
					return
				}
				if err != nil {
					t.Fatalf("runCMD(%q) error = %v", strings.Join(tt.args(mode.url), " "), err)
				}
				var task a2a.Task
				if err := json.Unmarshal([]byte(out), &task); err != nil {
					t.Fatalf("json.Unmarshal() error = %v", err)
				}
				if text := testutil.AllArtifactText(&task); text != tt.wantText {
					t.Fatalf("allArtifactText() = %q, want %q", text, tt.wantText)
				}
			})
		}
	}
}

func TestSend_AgentCardFromFile(t *testing.T) {
	t.Parallel()
	url := startTestServer(t)
	agentCardFilePath := testutil.MustWriteTmpCardFile(t, newAgentCard(url, a2a.AgentCapabilities{
		Streaming: true,
	}))

	msgText := "hello hello!"
	sendTests := []struct {
		name     string
		args     []string
		wantText string
	}{
		{
			name:     "file path",
			args:     []string{"send", "-a", agentCardFilePath, "-o", "json", msgText},
			wantText: msgText,
		},
		{
			name:     "file URL",
			args:     []string{"send", "-a", "file://" + agentCardFilePath, "-o", "json", msgText},
			wantText: msgText,
		},
	}

	for _, tt := range sendTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out, err := runCMD(t, tt.args...)
			if err != nil {
				t.Fatalf("runCMD(%q) error = %v", strings.Join(tt.args, " "), err)
			}
			var task a2a.Task
			if err := json.Unmarshal([]byte(out), &task); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			if text := testutil.AllArtifactText(&task); text != tt.wantText {
				t.Fatalf("allArtifactText() = %q, want %q", text, tt.wantText)
			}
		})
	}
}

func TestSendDataPart(t *testing.T) {
	t.Parallel()
	url := startTestServer(t)

	path := filepath.Join(t.TempDir(), "data.json")
	if err := os.WriteFile(path, []byte(`{"hello":"world"}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	out := mustRunCMD(t, "send", "-a", url, "-o", "json", "--data-part", path)
	var task a2a.Task
	if err := json.Unmarshal([]byte(out), &task); err != nil {
		t.Fatalf("json.Unmarshal(send --data-part output) error = %v", err)
	}
	if got := testutil.AllArtifactText(&task); got != `{"hello":"world"}` {
		t.Fatalf("allArtifactText() = %q, want %q", got, `{"hello":"world"}`)
	}
}

func TestSendRequestPayloadFile(t *testing.T) {
	t.Parallel()
	url := startTestServer(t)

	path := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(path, []byte(`{"message":{"role":"ROLE_USER","parts":[{"text":"from file"}]}}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	out := mustRunCMD(t, "send", "-a", url, "-o", "json", "--request-payload", path)
	var task a2a.Task
	if err := json.Unmarshal([]byte(out), &task); err != nil {
		t.Fatalf("json.Unmarshal(send --request-payload output) error = %v", err)
	}
	if got := testutil.AllArtifactText(&task); got != "from file" {
		t.Fatalf("allArtifactText() = %q, want %q", got, "from file")
	}
}

func TestBuildMessage(t *testing.T) {
	t.Parallel()

	t.Run("positional text is prepended before part flags", func(t *testing.T) {
		t.Parallel()
		msg, err := buildMessage([]string{"lead"}, partsFromArgs(t, "--text-part", "flagpart"))
		if err != nil {
			t.Fatalf("buildMessage() error = %v", err)
		}
		if len(msg.Parts) != 2 {
			t.Fatalf("len(parts) = %d, want 2", len(msg.Parts))
		}
		if msg.Parts[0].Text() != "lead" {
			t.Errorf("parts[0].Text() = %q, want %q", msg.Parts[0].Text(), "lead")
		}
		if msg.Parts[1].Text() != "flagpart" {
			t.Errorf("parts[1].Text() = %q, want %q", msg.Parts[1].Text(), "flagpart")
		}
	})

	t.Run("more than one positional is an error", func(t *testing.T) {
		t.Parallel()
		if _, err := buildMessage([]string{"one", "two"}, &sendFlags{}); err == nil {
			t.Fatal("buildMessage() with multiple positional args should fail")
		}
	})

	t.Run("no content is an error", func(t *testing.T) {
		t.Parallel()
		if _, err := buildMessage(nil, &sendFlags{}); err == nil {
			t.Fatal("buildMessage() with no content should fail")
		}
	})
}

func TestParseRequestPayload(t *testing.T) {
	t.Parallel()

	t.Run("inline JSON request", func(t *testing.T) {
		t.Parallel()
		req, err := parseRequestPayload(`{"message":{"role":"ROLE_USER","parts":[{"text":"hi"}]}}`)
		if err != nil {
			t.Fatalf("parseRequestPayload() error = %v", err)
		}
		if len(req.Message.Parts) != 1 || req.Message.Parts[0].Text() != "hi" {
			t.Fatalf("parseRequestPayload() parts = %+v, want a single text part %q", req.Message.Parts, "hi")
		}
		if req.Message.ID == "" {
			t.Fatal("parseRequestPayload() did not assign a message ID")
		}
	})

	t.Run("reads a file path", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "req.json")
		if err := os.WriteFile(path, []byte(`{"message":{"role":"ROLE_USER","parts":[{"text":"file"}]}}`), 0o644); err != nil {
			t.Fatalf("os.WriteFile() error = %v", err)
		}
		req, err := parseRequestPayload(path)
		if err != nil {
			t.Fatalf("parseRequestPayload() error = %v", err)
		}
		if req.Message.Parts[0].Text() != "file" {
			t.Errorf("parseRequestPayload() text = %q, want %q", req.Message.Parts[0].Text(), "file")
		}
	})

	t.Run("missing message is an error", func(t *testing.T) {
		t.Parallel()
		if _, err := parseRequestPayload(`{"tenant":"acme"}`); err == nil {
			t.Fatal("parseRequestPayload() without a message should fail")
		}
	})

	t.Run("neither a file nor valid JSON is an error", func(t *testing.T) {
		t.Parallel()
		if _, err := parseRequestPayload("not json and not a file"); err == nil {
			t.Fatal("parseRequestPayload() with invalid input should fail")
		}
	})
}

func partsFromArgs(t *testing.T, args ...string) *sendFlags {
	t.Helper()
	var p flagparse.Parts
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	p.Attach(fs)
	if err := fs.Parse(args); err != nil {
		t.Fatalf("fs.Parse(%v) error = %v", args, err)
	}
	return &sendFlags{parts: p}
}

func TestSendStreaming(t *testing.T) {
	t.Parallel()
	url := startTestServer(t)
	nonStreamingServerURL := startTestServerWith(t, a2a.AgentCapabilities{Streaming: false})

	testCases := []struct {
		name               string
		command            []string
		wantPollerFallback bool
	}{
		{
			name:               "streaming supported",
			command:            []string{"send", "-a", url, "-o", "json", "--stream", "stream me"},
			wantPollerFallback: false,
		},
		{
			name:               "poller fallback with create from card",
			command:            []string{"send", "-a", nonStreamingServerURL, "-o", "json", "--stream", "stream me"},
			wantPollerFallback: true,
		},
		{
			name:               "poller fallback with create from interface",
			command:            []string{"send", "-e", nonStreamingServerURL, "--transport", "rest", "-o", "json", "--stream", "stream me"},
			wantPollerFallback: true,
		},
	}
	for _, tc := range testCases {
		fallbackToPoller := false
		poller := pollerFunc(func(ctx context.Context, client *a2aclient.Client, req *a2a.SendMessageRequest, interval time.Duration) iter.Seq2[a2a.Event, error] {
			return func(yield func(a2a.Event, error) bool) {
				fallbackToPoller = true
				task := &a2a.Task{ID: a2a.NewTaskID(), ContextID: a2a.NewContextID(), Status: a2a.TaskStatus{State: a2a.TaskStateSubmitted}}
				if !yield(task, nil) {
					return
				}
				yield(a2a.NewStatusUpdateEvent(task, a2a.TaskStateCompleted, nil), nil)
			}
		})
		out, err := runCMDWithPoller(t, deps{poller: poller}, tc.command...)
		if err != nil {
			t.Fatalf("runCMDWithPoller() error = %v", err)
		}
		dec := json.NewDecoder(strings.NewReader(out))
		var events []a2a.StreamResponse
		for dec.More() {
			var sr a2a.StreamResponse
			if err := dec.Decode(&sr); err != nil {
				t.Fatalf("json.Decode(event %d) error = %v", len(events), err)
			}
			if sr.Event == nil {
				t.Fatalf("json.Decode(event %d) produced nil Event", len(events))
			}
			events = append(events, sr)
		}
		if len(events) <= 1 {
			t.Fatalf("a2a send --stream produced %d events, want > 1", len(events))
		}
		if fallbackToPoller != tc.wantPollerFallback {
			t.Fatalf("fallback to poller = %v, want the opposite", fallbackToPoller)
		}
	}
}

func TestSendStreamingFallbackUsesDefaultPoller(t *testing.T) {
	t.Parallel()
	nonStreamingURL := startTestServerWith(t, a2a.AgentCapabilities{Streaming: false})

	out, err := runCMDWithConfig(t, deps{cfgLoader: clicfg.LoadEmpty},
		"send", "-a", nonStreamingURL, "-o", "json", "--stream", "stream me", "--polling-interval", "5ms")
	if err != nil {
		t.Fatalf("runCMDWithConfig() error = %v", err)
	}

	dec := json.NewDecoder(strings.NewReader(out))
	events := 0
	for dec.More() {
		var sr a2a.StreamResponse
		if err := dec.Decode(&sr); err != nil {
			t.Fatalf("json.Decode(event %d) error = %v", events, err)
		}
		events++
	}
	if events == 0 {
		t.Fatalf("send --stream via default poller produced %d events, want > 0", events)
	}
}

func TestGetTask(t *testing.T) {
	t.Parallel()
	url := startTestServer(t)

	taskID := sendTestMessage(t, url, "setup")

	t.Run("get task by id", func(t *testing.T) {
		t.Parallel()
		out := mustRunCMD(t, "task", "get", "-a", url, string(taskID), "-o", "json")
		var task a2a.Task
		if err := json.Unmarshal([]byte(out), &task); err != nil {
			t.Fatalf("json.Unmarshal(task get output) error = %v", err)
		}
		if task.ID != taskID {
			t.Fatalf("a2a task get ID = %q, want %q", task.ID, taskID)
		}
		if task.Status.State != a2a.TaskStateCompleted {
			t.Fatalf("a2a task get Status.State = %q, want %q", task.Status.State, a2a.TaskStateCompleted)
		}
	})

	t.Run("get task with --history", func(t *testing.T) {
		t.Parallel()
		out := mustRunCMD(t, "task", "get", "-a", url, string(taskID), "--history", "10", "-o", "json")
		var task a2a.Task
		if err := json.Unmarshal([]byte(out), &task); err != nil {
			t.Fatalf("json.Unmarshal(task get --history output) error = %v", err)
		}
		if task.ID != taskID {
			t.Fatalf("a2a task get --history ID = %q, want %q", task.ID, taskID)
		}
		if len(task.History) == 0 {
			t.Fatal("a2a task get --history returned no history")
		}
	})

	t.Run("missing args fails", func(t *testing.T) {
		t.Parallel()
		if _, err := runCMD(t, "task", "get", "-a", url); err == nil {
			t.Fatal("a2a task get (missing id) should fail")
		}
	})
}

func TestServe_ModeValidation(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name           string
		args           []string
		wantErrContain string
	}{
		{"no mode", []string{"server"}, "specify --echo, --proxy <url>, or --exec"},
		{"multiple modes", []string{"server", "--echo", "--exec", "cat"}, "mutually exclusive"},
	} {
		t.Run(tt.name+" fails", func(t *testing.T) {
			t.Parallel()
			_, err := runCMD(t, tt.args...)
			if err == nil {
				t.Fatalf("runCMD(%v) error = nil, want error containing %q", tt.args, tt.wantErrContain)
			}
			if !strings.Contains(err.Error(), tt.wantErrContain) {
				t.Fatalf("runCMD(%v) error = %v, want error containing %q", tt.args, err, tt.wantErrContain)
			}
		})
	}
}

func TestConfigApplied(t *testing.T) {
	t.Parallel()
	url := startTestServer(t)

	testCases := []struct {
		name           string
		command        []string
		env            map[string]string
		wantErrContain string
	}{
		{
			name:    "loaded from env",
			env:     map[string]string{"A2ACLI_AGENT_CARD": url},
			command: []string{"card", "get", "-o", "json"},
		},
		{
			name:    "global flag override",
			env:     map[string]string{"A2ACLI_AGENT_CARD": "https://unreachable.invalid"},
			command: []string{"card", "get", "-o", "json", "-a", url},
		},
		{
			name:           "local flag override",
			env:            map[string]string{"A2ACLI_EXTENDED": "true"},
			command:        []string{"card", "get", "-o", "json", "-a", url},
			wantErrContain: "extended card not configured",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			loader := cfgLoaderFunc(func(clicfg.LoadOpts) (*clicfg.Store, error) {
				return clicfg.Load(clicfg.LoadOpts{
					LookupEnv: func(k string) (string, bool) {
						v, ok := tc.env[k]
						return v, ok
					},
				})
			})
			out, err := runCMDWithConfig(t, deps{cfgLoader: loader}, tc.command...)
			if err != nil && tc.wantErrContain == "" {
				t.Fatalf("runCMDWithConfig(%v) error = %v", strings.Join(tc.command, " "), err)
			}
			if err == nil && tc.wantErrContain != "" {
				t.Fatalf("runCMDWithConfig(%v) error = nil, want containing %q", strings.Join(tc.command, " "), tc.wantErrContain)
			}
			if err != nil && !strings.Contains(err.Error(), tc.wantErrContain) {
				t.Fatalf("runCMDWithConfig(%v) error = %v, want containing %q", strings.Join(tc.command, " "), err, tc.wantErrContain)
			}
			if err != nil {
				return
			}
			var card a2a.AgentCard
			if err := json.Unmarshal([]byte(out), &card); err != nil {
				t.Fatalf("json.Unmarshal(card get output) error = %v", err)
			}
			if card.Name != "Test Echo" {
				t.Fatalf("card.Name = %q, want %q", card.Name, "Test Echo")
			}
		})
	}
}

func startTestServer(t *testing.T) string {
	t.Helper()
	return startTestServerWith(t, a2a.AgentCapabilities{Streaming: true})
}

func startTestServerWith(t *testing.T, capabilities a2a.AgentCapabilities) string {
	t.Helper()

	handler := a2asrv.NewHandler(localsrv.NewEchoExecutor(), a2asrv.WithCapabilityChecks(&capabilities))

	mux := http.NewServeMux()
	mux.Handle("/", a2asrv.NewRESTHandler(handler))

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	agentCard := newAgentCard(server.URL, capabilities)
	mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(agentCard))

	return server.URL
}

func newAgentCard(url string, cap a2a.AgentCapabilities) *a2a.AgentCard {
	return &a2a.AgentCard{
		Name:                "Test Echo",
		Version:             "1.0.0",
		Capabilities:        cap,
		SupportedInterfaces: []*a2a.AgentInterface{a2a.NewAgentInterface(url, a2a.TransportProtocolHTTPJSON)},
	}
}

func startLegacyTestServer(t *testing.T) string {
	t.Helper()

	handler := a2asrvv0.NewHandler(&legacyExecutor{})

	mux := http.NewServeMux()
	mux.Handle("/", a2asrvv0.NewJSONRPCHandler(handler))

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrvv0.NewStaticAgentCardHandler(&a2acorev0.AgentCard{
		Name:               "Test Echo",
		Version:            "1.0.0",
		URL:                server.URL,
		PreferredTransport: a2acorev0.TransportProtocol(a2a.TransportProtocolJSONRPC),
		Capabilities:       a2acorev0.AgentCapabilities{Streaming: true},
	}))

	return server.URL
}

func sendTestMessage(t *testing.T, url, text string) a2a.TaskID {
	t.Helper()
	ctx := t.Context()

	client, err := a2aclient.NewFromEndpoints(ctx, []*a2a.AgentInterface{
		a2a.NewAgentInterface(url, a2a.TransportProtocolHTTPJSON),
	})
	if err != nil {
		t.Fatalf("a2aclient.NewFromEndpoints() error = %v", err)
	}
	defer func() { _ = client.Destroy() }()

	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart(text))
	result, err := client.SendMessage(ctx, &a2a.SendMessageRequest{Message: msg})
	if err != nil {
		t.Fatalf("client.SendMessage() error = %v", err)
	}
	task, ok := result.(*a2a.Task)
	if !ok {
		t.Fatalf("SendMessage() result type = %T, want *a2a.Task", result)
	}
	return task.ID
}

func mustRunCMD(t *testing.T, args ...string) string {
	t.Helper()
	r, err := runCMD(t, args...)
	if err != nil {
		t.Fatalf("runCMD(%q) error = %v", strings.Join(args, " "), err)
	}
	return r
}

func runCMD(t *testing.T, args ...string) (string, error) {
	t.Helper()
	return runCMDWithPoller(t, deps{poller: polling.Stream, cfgLoader: clicfg.LoadEmpty}, args...)
}

func runCMDWithPoller(t *testing.T, deps deps, args ...string) (string, error) {
	t.Helper()
	return runCMDWithConfig(t, deps, args...)
}

func runCMDWithConfig(t *testing.T, deps deps, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	cfg := &globalConfig{
		Printer:   output.NewPrinter(&buf, output.ModeText),
		svcParams: &flagparse.ServiceParams{},
	}
	root := newRootCmd(cfg, deps)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

type legacyExecutor struct{}

func (e *legacyExecutor) Execute(ctx context.Context, reqCtx *a2asrvv0.RequestContext, queue eventqueue.Queue) error {
	if err := queue.Write(ctx, a2acorev0.NewStatusUpdateEvent(reqCtx, a2acorev0.TaskState(a2a.TaskStateSubmitted), nil)); err != nil {
		return err
	}
	msg, err := a2av0.ToV1Message(reqCtx.Message)
	if err != nil {
		return err
	}
	echo := a2acorev0.TextPart{Text: output.MessageText(msg)}
	if err := queue.Write(ctx, a2acorev0.NewArtifactEvent(reqCtx, echo)); err != nil {
		return err
	}
	finalEvent := a2acorev0.NewStatusUpdateEvent(reqCtx, a2acorev0.TaskState(a2a.TaskStateCompleted), nil)
	finalEvent.Final = true
	return queue.Write(ctx, finalEvent)
}

func (e *legacyExecutor) Cancel(ctx context.Context, reqCtx *a2asrvv0.RequestContext, queue eventqueue.Queue) error {
	return fmt.Errorf("not implemented")
}
