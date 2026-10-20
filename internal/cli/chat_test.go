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
	"errors"
	"io"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/a2aproject/a2a-cli/internal/clicfg"
	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-cli/internal/flagparse"
	"github.com/a2aproject/a2a-cli/internal/output"
	"github.com/a2aproject/a2a-cli/internal/polling"
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

// chatRequest is the message the agent received on one turn.
type chatRequest struct {
	Text      string
	TaskID    string
	ContextID string
}

const (
	// chatTurnFails scripts a turn that the agent answers with an error.
	chatTurnFails a2a.TaskState = "fail"
	// chatTurnInterrupted scripts a turn that the user interrupts while the agent works.
	chatTurnInterrupted a2a.TaskState = "interrupt"
	// chatTurnInterruptedWorking scripts a turn that reports the task as working and then
	// waits; the test interrupts it once chat has printed the task.
	chatTurnInterruptedWorking a2a.TaskState = "interrupt-working"
)

// chatAgent answers each turn with a task in the next scripted state.
type chatAgent struct {
	mu       sync.Mutex
	states   []a2a.TaskState
	requests []chatRequest
	tasks    []*a2a.Task
	canceled []string

	// interrupt stands in for Ctrl-C: it cancels the context of the latest interrupt
	// scope chat opened, which during a turn is that turn.
	interrupt context.CancelFunc
}

// notifyInterrupt replaces signal.NotifyContext so a test can interrupt chat.
func (a *chatAgent) notifyInterrupt(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	a.mu.Lock()
	a.interrupt = cancel
	a.mu.Unlock()
	return ctx, cancel
}

func startChatAgent(t *testing.T, states ...a2a.TaskState) (*chatAgent, string) {
	t.Helper()
	agent := &chatAgent{states: states}
	release := make(chan struct{})
	handler := a2asrv.NewRESTHandler(a2asrv.NewHandler(
		a2asrv.AgentExecutorFunc(func(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
			return func(yield func(a2a.Event, error) bool) {
				agent.mu.Lock()
				turn := len(agent.requests)
				agent.requests = append(agent.requests, chatRequest{
					Text:      ec.Message.Parts[0].Text(),
					TaskID:    string(ec.Message.TaskID),
					ContextID: ec.Message.ContextID,
				})
				state := agent.states[turn]
				task := &a2a.Task{
					ID:        ec.TaskID,
					ContextID: ec.ContextID,
					Status:    a2a.TaskStatus{State: a2a.TaskStateWorking},
				}
				agent.tasks = append(agent.tasks, task)
				agent.mu.Unlock()
				if state == chatTurnFails {
					yield(nil, errors.New("agent failed"))
					return
				}
				if state == chatTurnInterruptedWorking && !yield(task, nil) {
					return
				}
				if state == chatTurnInterrupted || state == chatTurnInterruptedWorking {
					if state == chatTurnInterrupted {
						agent.mu.Lock()
						agent.interrupt()
						agent.mu.Unlock()
					}
					select {
					case <-ctx.Done():
					case <-release:
					}
					return
				}
				// A working task followed by its final status, so a stream has more than one event.
				if yield(task, nil) {
					yield(a2a.NewStatusUpdateEvent(task, state, nil), nil)
				}
			}
		}),
	))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id, ok := strings.CutSuffix(strings.TrimPrefix(r.URL.Path, "/tasks/"), ":cancel"); ok {
			agent.mu.Lock()
			agent.canceled = append(agent.canceled, id)
			agent.mu.Unlock()
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })
	return agent, server.URL
}

func runChat(t *testing.T, url string, terminal bool, stdin string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	return runChatWith(t, deps{isTerminal: func() bool { return terminal }}, url, strings.NewReader(stdin), args...)
}

func runChatWith(t *testing.T, d deps, url string, stdin io.Reader, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out bytes.Buffer
	stderr, err = runChatTo(t, d, url, stdin, &out, args...)
	return out.String(), stderr, err
}

func runChatTo(t *testing.T, d deps, url string, stdin io.Reader, out io.Writer, args ...string) (stderr string, err error) {
	t.Helper()
	var errBuf bytes.Buffer
	cfg := &globalConfig{
		Printer:   output.NewPrinter(out, output.ModeText),
		svcParams: &flagparse.ServiceParams{},
		errOut:    &errBuf,
	}
	d.poller = polling.Stream
	d.cfgLoader = clicfg.LoadEmpty
	root := mustNewRoot(t, cfg, d)
	root.SetIn(stdin)
	root.SetArgs(append([]string{"chat", "-e", url, "--transport", "rest"}, args...))
	err = root.Execute()
	return errBuf.String(), err
}

// interruptOnPrint stands in for a user who presses Ctrl-C once chat prints something.
type interruptOnPrint struct{ agent *chatAgent }

func (w interruptOnPrint) Write(p []byte) (int, error) {
	w.agent.mu.Lock()
	defer w.agent.mu.Unlock()
	w.agent.interrupt()
	return len(p), nil
}

func TestChatCarriesTheConversationAcrossTurns(t *testing.T) {
	t.Parallel()
	agent, url := startChatAgent(t, a2a.TaskStateInputRequired, a2a.TaskStateCompleted, a2a.TaskStateCompleted)

	_, stderr, err := runChat(t, url, true, "two\nthree\n/quit\nnever sent\n", "one")
	if err != nil {
		t.Fatalf("chat error = %v, want nil", err)
	}

	first := agent.tasks[0]
	want := []chatRequest{
		{Text: "one"},
		// The first task asked for input, so the reply continues it.
		{Text: "two", TaskID: string(first.ID), ContextID: first.ContextID},
		// That task completed, so the next turn starts a new task in the same context.
		{Text: "three", ContextID: first.ContextID},
	}
	if diff := cmp.Diff(want, agent.requests); diff != "" {
		t.Fatalf("chat requests wrong (-want +got):\n%s", diff)
	}
	if !strings.Contains(stderr, "contextId: "+first.ContextID) {
		t.Fatalf("chat did not print the context id to resume with:\n%s", stderr)
	}
}

func TestChatEnds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		states       []a2a.TaskState
		stdin        string
		wantRequests int
		wantStderr   string
	}{
		{
			name:         "on EOF",
			states:       []a2a.TaskState{a2a.TaskStateInputRequired},
			stdin:        "one\n",
			wantRequests: 1,
			wantStderr:   "taskId: ",
		},
		{
			name:         "on /quit",
			states:       []a2a.TaskState{a2a.TaskStateCompleted},
			stdin:        "one\n/quit\ntwo\n",
			wantRequests: 1,
			wantStderr:   "contextId: ",
		},
		{
			name:         "when the agent needs authentication",
			states:       []a2a.TaskState{a2a.TaskStateAuthRequired},
			stdin:        "one\ntwo\n",
			wantRequests: 1,
			wantStderr:   "authentication",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			agent, url := startChatAgent(t, tt.states...)

			_, stderr, err := runChat(t, url, true, tt.stdin)
			if err != nil {
				t.Fatalf("chat error = %v, want nil", err)
			}
			if got := len(agent.requests); got != tt.wantRequests {
				t.Errorf("chat sent %d messages, want %d", got, tt.wantRequests)
			}
			if !strings.Contains(stderr, tt.wantStderr) {
				t.Errorf("chat stderr missing %q:\n%s", tt.wantStderr, stderr)
			}
		})
	}
}

func TestChatWithoutATerminal(t *testing.T) {
	t.Parallel()

	t.Run("sends the message once and reads no input", func(t *testing.T) {
		t.Parallel()
		agent, url := startChatAgent(t, a2a.TaskStateCompleted)

		if _, _, err := runChat(t, url, false, "two\n", "one"); err != nil {
			t.Fatalf("chat error = %v, want nil", err)
		}
		if diff := cmp.Diff([]chatRequest{{Text: "one"}}, agent.requests); diff != "" {
			t.Fatalf("chat requests wrong (-want +got):\n%s", diff)
		}
	})

	t.Run("without a message is a usage error", func(t *testing.T) {
		t.Parallel()
		agent, url := startChatAgent(t)

		_, _, err := runChat(t, url, false, "one\n")
		if ce := clierr.Classify(err); ce == nil || ce.Code != clierr.CodeUsage {
			t.Fatalf("chat error = %v, want a usage error", err)
		}
		if got := len(agent.requests); got != 0 {
			t.Fatalf("chat sent %d messages, want 0", got)
		}
	})
}

func TestChatResumesATask(t *testing.T) {
	t.Parallel()
	agent, url := startChatAgent(t, a2a.TaskStateInputRequired, a2a.TaskStateCompleted)

	if _, _, err := runChat(t, url, false, "", "one"); err != nil {
		t.Fatalf("chat error = %v, want nil", err)
	}
	first := agent.tasks[0]

	_, _, err := runChat(t, url, true, "two\n", "--context-id", first.ContextID, "--task-id", string(first.ID))
	if err != nil {
		t.Fatalf("chat error = %v, want nil", err)
	}

	want := chatRequest{Text: "two", TaskID: string(first.ID), ContextID: first.ContextID}
	if diff := cmp.Diff(want, agent.requests[1]); diff != "" {
		t.Fatalf("chat did not resume the task (-want +got):\n%s", diff)
	}
}

func TestChatContinuesAfterAnError(t *testing.T) {
	t.Parallel()
	agent, url := startChatAgent(t, a2a.TaskStateCompleted, chatTurnFails, a2a.TaskStateCompleted)

	_, stderr, err := runChat(t, url, true, "two\nthree\n", "one")
	if err != nil {
		t.Fatalf("chat error = %v, want nil", err)
	}

	contextID := agent.tasks[0].ContextID
	want := []chatRequest{
		{Text: "one"},
		{Text: "two", ContextID: contextID},
		// The failed turn is reported and the conversation goes on in the same context.
		{Text: "three", ContextID: contextID},
	}
	if diff := cmp.Diff(want, agent.requests); diff != "" {
		t.Fatalf("chat requests wrong (-want +got):\n%s", diff)
	}
	if !strings.Contains(stderr, "Error: failed to send message") {
		t.Fatalf("chat did not print the send error:\n%s", stderr)
	}
}

func TestChatStreams(t *testing.T) {
	t.Parallel()
	agent, url := startChatAgent(t, a2a.TaskStateInputRequired, a2a.TaskStateCompleted)

	stdout, _, err := runChat(t, url, true, "two\n", "--stream", "one")
	if err != nil {
		t.Fatalf("chat error = %v, want nil", err)
	}

	first := agent.tasks[0]
	want := []chatRequest{
		{Text: "one"},
		{Text: "two", TaskID: string(first.ID), ContextID: first.ContextID},
	}
	if diff := cmp.Diff(want, agent.requests); diff != "" {
		t.Fatalf("chat requests wrong (-want +got):\n%s", diff)
	}
	// Each turn streams the working task and then its final status update.
	if got := strings.Count(stdout, "[status] "); got != 2 {
		t.Fatalf("chat printed %d status updates, want 2:\n%s", got, stdout)
	}
}

func TestChatInterrupt(t *testing.T) {
	t.Parallel()

	t.Run("cancels the running turn and keeps the session open", func(t *testing.T) {
		t.Parallel()
		agent, url := startChatAgent(t, a2a.TaskStateCompleted, chatTurnInterrupted, a2a.TaskStateCompleted)
		d := deps{isTerminal: func() bool { return true }, notifyInterrupt: agent.notifyInterrupt}

		_, stderr, err := runChatWith(t, d, url, strings.NewReader("two\nthree\n"), "one")
		if err != nil {
			t.Fatalf("chat error = %v, want nil", err)
		}

		contextID := agent.tasks[0].ContextID
		want := []chatRequest{
			{Text: "one"},
			{Text: "two", ContextID: contextID},
			{Text: "three", ContextID: contextID},
		}
		if diff := cmp.Diff(want, agent.requests); diff != "" {
			t.Fatalf("chat requests wrong (-want +got):\n%s", diff)
		}
		if !strings.Contains(stderr, "Interrupted") {
			t.Fatalf("chat did not report the interrupted turn:\n%s", stderr)
		}
	})

	t.Run("cancels the task the agent is working on", func(t *testing.T) {
		t.Parallel()
		agent, url := startChatAgent(t, chatTurnInterruptedWorking)
		d := deps{isTerminal: func() bool { return true }, notifyInterrupt: agent.notifyInterrupt}

		stderr, err := runChatTo(t, d, url, strings.NewReader(""), interruptOnPrint{agent}, "--stream", "one")
		if err != nil {
			t.Fatalf("chat error = %v, want nil", err)
		}

		want := []string{string(agent.tasks[0].ID)}
		if diff := cmp.Diff(want, agent.canceled); diff != "" {
			t.Fatalf("chat canceled the wrong tasks (-want +got):\n%s\n%s", diff, stderr)
		}
		if !strings.Contains(stderr, "Canceled task "+string(agent.tasks[0].ID)) {
			t.Fatalf("chat did not report the canceled task:\n%s", stderr)
		}
	})

	t.Run("at the prompt ends the session", func(t *testing.T) {
		t.Parallel()
		agent, url := startChatAgent(t)
		interrupted := func(ctx context.Context) (context.Context, context.CancelFunc) {
			ctx, cancel := context.WithCancel(ctx)
			cancel()
			return ctx, cancel
		}
		d := deps{isTerminal: func() bool { return true }, notifyInterrupt: interrupted}
		// Input that never arrives, so only the interrupt can end the wait at the prompt.
		stdin, w := io.Pipe()
		t.Cleanup(func() { _ = w.Close() })

		_, stderr, err := runChatWith(t, d, url, stdin, "--context-id", "ctx-1")
		if err != nil {
			t.Fatalf("chat error = %v, want nil", err)
		}
		if got := len(agent.requests); got != 0 {
			t.Fatalf("chat sent %d messages, want 0", got)
		}
		if !strings.Contains(stderr, "contextId: ctx-1") {
			t.Fatalf("chat did not print the context id to resume with:\n%s", stderr)
		}
	})
}
