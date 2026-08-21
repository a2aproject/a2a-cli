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
	msgJSON := fmt.Sprintf(`{"role":"ROLE_USER","parts":[{"text":"%s"}]}`, msgText)

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
				return []string{"send", "-a", url, "-o", "json", "--text", "part one", "--text", "part two"}
			},
			wantText: "part one part two",
		},
		{
			name: "message json",
			args: func(url string) []string {
				return []string{"send", "-a", url, "-o", "json", "--json", msgJSON}
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
			name: "fails on bad --json",
			args: func(url string) []string {
				return []string{"send", "-a", url, "--json", "{bad"}
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
				if text := allArtifactText(&task); text != tt.wantText {
					t.Fatalf("allArtifactText() = %q, want %q", text, tt.wantText)
				}
			})
		}
	}
}

func TestSendDataPart(t *testing.T) {
	t.Parallel()
	url := startTestServer(t)

	path := filepath.Join(t.TempDir(), "data.json")
	if err := os.WriteFile(path, []byte(`{"hello":"world"}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	out := mustRunCMD(t, "send", "-a", url, "-o", "json", "--data", path)
	var task a2a.Task
	if err := json.Unmarshal([]byte(out), &task); err != nil {
		t.Fatalf("json.Unmarshal(send --data output) error = %v", err)
	}
	if got := allArtifactText(&task); got != `{"hello":"world"}` {
		t.Fatalf("allArtifactText() = %q, want %q", got, `{"hello":"world"}`)
	}
}

func TestBuildMessage(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	binPath := filepath.Join(dir, "report.bin")
	if err := os.WriteFile(binPath, []byte("rawbytes"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	t.Run("part flags preserve command-line order", func(t *testing.T) {
		t.Parallel()
		b := &partsBuilder{}
		b.add(kindText, "first")
		b.add(kindFile, "https://example.com/pic.png")
		b.add(kindText, "third")

		msg, err := buildMessage(nil, "", b)
		if err != nil {
			t.Fatalf("buildMessage() error = %v", err)
		}
		if len(msg.Parts) != 3 {
			t.Fatalf("len(parts) = %d, want 3", len(msg.Parts))
		}
		if msg.Parts[0].Text() != "first" {
			t.Errorf("parts[0].Text() = %q, want %q", msg.Parts[0].Text(), "first")
		}
		if msg.Parts[1].URL() != "https://example.com/pic.png" {
			t.Errorf("parts[1].URL() = %q, want the image URL", msg.Parts[1].URL())
		}
		if msg.Parts[2].Text() != "third" {
			t.Errorf("parts[2].Text() = %q, want %q", msg.Parts[2].Text(), "third")
		}
	})

	t.Run("media-type binds to the preceding part and local file is inlined", func(t *testing.T) {
		t.Parallel()
		b := &partsBuilder{}
		b.add(kindFile, binPath)
		if err := b.setMediaType("application/pdf"); err != nil {
			t.Fatalf("setMediaType() error = %v", err)
		}

		msg, err := buildMessage(nil, "", b)
		if err != nil {
			t.Fatalf("buildMessage() error = %v", err)
		}
		if len(msg.Parts) != 1 {
			t.Fatalf("len(parts) = %d, want 1", len(msg.Parts))
		}
		p := msg.Parts[0]
		if string(p.Raw()) != "rawbytes" {
			t.Errorf("part raw bytes = %q, want %q", string(p.Raw()), "rawbytes")
		}
		if p.MediaType != "application/pdf" {
			t.Errorf("part.MediaType = %q, want %q", p.MediaType, "application/pdf")
		}
		if p.Filename != "report.bin" {
			t.Errorf("part.Filename = %q, want %q", p.Filename, "report.bin")
		}
	})

	t.Run("media-type without a preceding part flag is an error", func(t *testing.T) {
		t.Parallel()
		b := &partsBuilder{}
		if err := b.setMediaType("text/plain"); err == nil {
			t.Fatal("setMediaType() with no preceding part should fail")
		}
	})

	t.Run("positional text is appended after part flags", func(t *testing.T) {
		t.Parallel()
		b := &partsBuilder{}
		b.add(kindText, "flagpart")

		msg, err := buildMessage([]string{"positional", "tail"}, "", b)
		if err != nil {
			t.Fatalf("buildMessage() error = %v", err)
		}
		if len(msg.Parts) != 2 {
			t.Fatalf("len(parts) = %d, want 2", len(msg.Parts))
		}
		if msg.Parts[0].Text() != "flagpart" {
			t.Errorf("parts[0].Text() = %q, want %q", msg.Parts[0].Text(), "flagpart")
		}
		if msg.Parts[1].Text() != "positional tail" {
			t.Errorf("parts[1].Text() = %q, want %q", msg.Parts[1].Text(), "positional tail")
		}
	})

	t.Run("--json is mutually exclusive with part flags", func(t *testing.T) {
		t.Parallel()
		b := &partsBuilder{}
		b.add(kindText, "hi")
		if _, err := buildMessage(nil, `{"role":"ROLE_USER"}`, b); err == nil {
			t.Fatal("buildMessage() with --json and part flags should fail")
		}
	})

	t.Run("no content is an error", func(t *testing.T) {
		t.Parallel()
		if _, err := buildMessage(nil, "", &partsBuilder{}); err == nil {
			t.Fatal("buildMessage() with no content should fail")
		}
	})
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
			command:            []string{"send", "-u", nonStreamingServerURL, "--transport", "rest", "-o", "json", "--stream", "stream me"},
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
		out, err := runCMDWithPoller(t, poller, tc.command...)
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

func startTestServer(t *testing.T) string {
	t.Helper()
	return startTestServerWith(t, a2a.AgentCapabilities{Streaming: true})
}

func startTestServerWith(t *testing.T, capabilities a2a.AgentCapabilities) string {
	t.Helper()

	handler := a2asrv.NewHandler(&echoExecutor{}, a2asrv.WithCapabilityChecks(&capabilities))

	mux := http.NewServeMux()
	mux.Handle("/", a2asrv.NewRESTHandler(handler))

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(&a2a.AgentCard{
		Name:                "Test Echo",
		Version:             "1.0.0",
		Capabilities:        capabilities,
		SupportedInterfaces: []*a2a.AgentInterface{a2a.NewAgentInterface(server.URL, a2a.TransportProtocolHTTPJSON)},
	}))

	return server.URL
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
	return runCMDWithPoller(t, handlePolling, args...)
}

func runCMDWithPoller(t *testing.T, poller pollerFunc, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	cfg := &globalConfig{}
	root := newRootCmd(cfg, &buf, poller)
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
	echo := a2acorev0.TextPart{Text: messageText(msg)}
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
