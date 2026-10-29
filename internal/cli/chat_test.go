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

type chatRequest struct {
	Text      string
	TaskID    string
	ContextID string
}

// Scripted turns that are not task states.
const (
	chatTurnFails a2a.TaskState = "fail"
	// chatTurnInterrupted interrupts the turn before the agent names a task.
	chatTurnInterrupted a2a.TaskState = "interrupt"
	// chatTurnHangs reports a working task and never finishes it.
	chatTurnHangs a2a.TaskState = "hang"
)

// chatAgent answers each turn with a task in the next scripted state.
type chatAgent struct {
	mu       sync.Mutex
	states   []a2a.TaskState
	requests []chatRequest
	tasks    []*a2a.Task
	canceled []string

	// hangCancel makes a cancel request interrupt chat and then wait for the client to give up.
	hangCancel bool

	// interrupt stands in for Ctrl-C: it cancels the latest interrupt scope chat opened.
	interrupt context.CancelFunc
}

func (a *chatAgent) notifyInterrupt(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	a.mu.Lock()
	a.interrupt = cancel
	a.mu.Unlock()
	return ctx, cancel
}

func (a *chatAgent) pressCtrlC() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.interrupt()
}

func (a *chatAgent) deps() deps {
	return deps{isTerminal: func() bool { return true }, notifyInterrupt: a.notifyInterrupt}
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

				switch state {
				case chatTurnFails:
					yield(nil, errors.New("agent failed"))
					return
				case chatTurnInterrupted:
					agent.pressCtrlC()
				case chatTurnHangs:
					if !yield(task, nil) {
						return
					}
				default:
					if yield(task, nil) {
						yield(a2a.NewStatusUpdateEvent(task, state, nil), nil)
					}
					return
				}
				select {
				case <-ctx.Done():
				case <-release:
				}
			}
		}),
	))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, isCancel := strings.CutSuffix(strings.TrimPrefix(r.URL.Path, "/tasks/"), ":cancel")
		if !isCancel {
			handler.ServeHTTP(w, r)
			return
		}
		agent.mu.Lock()
		agent.canceled = append(agent.canceled, id)
		hang := agent.hangCancel
		agent.mu.Unlock()
		if hang {
			agent.pressCtrlC()
			<-r.Context().Done()
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })
	return agent, server.URL
}

func runChat(t *testing.T, url string, terminal bool, stdin string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out bytes.Buffer
	stderr, err = runChatTo(t, deps{isTerminal: func() bool { return terminal }}, url, strings.NewReader(stdin), &out, args...)
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

// interruptOnPrint presses Ctrl-C whenever chat prints to stdout.
type interruptOnPrint struct{ agent *chatAgent }

func (w interruptOnPrint) Write(p []byte) (int, error) {
	w.agent.pressCtrlC()
	return len(p), nil
}

func TestChatCarriesTheConversationAcrossTurns(t *testing.T) {
	t.Parallel()
	agent, url := startChatAgent(t, a2a.TaskStateInputRequired, a2a.TaskStateCompleted, a2a.TaskStateCompleted)

	_, stderr, err := runChat(t, url, true, "two\nthree\n/quit\nnever sent\n", "one")
	if err != nil {
		t.Fatalf("a2a chat error = %v, want nil", err)
	}

	first := agent.tasks[0]
	want := []chatRequest{
		{Text: "one"},
		{Text: "two", TaskID: string(first.ID), ContextID: first.ContextID},
		{Text: "three", ContextID: first.ContextID},
	}
	if diff := cmp.Diff(want, agent.requests); diff != "" {
		t.Fatalf("a2a chat requests wrong result (-want +got) diff = %s", diff)
	}
	if !strings.Contains(stderr, "contextId: "+first.ContextID) {
		t.Fatalf("a2a chat stderr = %q, want the context id", stderr)
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
			name:         "on /exit",
			states:       []a2a.TaskState{a2a.TaskStateCompleted},
			stdin:        "one\n/exit\ntwo\n",
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
				t.Fatalf("a2a chat error = %v, want nil", err)
			}
			if got := len(agent.requests); got != tt.wantRequests {
				t.Fatalf("a2a chat sent %d messages, want %d", got, tt.wantRequests)
			}
			if !strings.Contains(stderr, tt.wantStderr) {
				t.Fatalf("a2a chat stderr = %q, want it to contain %q", stderr, tt.wantStderr)
			}
		})
	}
}

func TestChatWithoutATerminalIsAUsageError(t *testing.T) {
	t.Parallel()
	agent, url := startChatAgent(t)

	_, _, err := runChat(t, url, false, "two\n", "one")
	if ce := clierr.Classify(err); ce == nil || ce.Code != clierr.CodeUsage {
		t.Fatalf("a2a chat error = %v, want a usage error", err)
	}
	if got := len(agent.requests); got != 0 {
		t.Fatalf("a2a chat sent %d messages, want 0", got)
	}
}

func TestChatResumesATask(t *testing.T) {
	t.Parallel()
	agent, url := startChatAgent(t, a2a.TaskStateInputRequired, a2a.TaskStateCompleted)

	if _, _, err := runChat(t, url, true, "", "one"); err != nil {
		t.Fatalf("a2a chat error = %v, want nil", err)
	}
	first := agent.tasks[0]

	_, _, err := runChat(t, url, true, "two\n", "--context-id", first.ContextID, "--task-id", string(first.ID))
	if err != nil {
		t.Fatalf("a2a chat error = %v, want nil", err)
	}

	want := chatRequest{Text: "two", TaskID: string(first.ID), ContextID: first.ContextID}
	if diff := cmp.Diff(want, agent.requests[1]); diff != "" {
		t.Fatalf("a2a chat resumed request wrong result (-want +got) diff = %s", diff)
	}
}

func TestChatContinuesAfterAnError(t *testing.T) {
	t.Parallel()
	agent, url := startChatAgent(t, a2a.TaskStateCompleted, chatTurnFails, a2a.TaskStateCompleted)

	_, stderr, err := runChat(t, url, true, "two\nthree\n", "one")
	if err != nil {
		t.Fatalf("a2a chat error = %v, want nil", err)
	}

	contextID := agent.tasks[0].ContextID
	want := []chatRequest{
		{Text: "one"},
		{Text: "two", ContextID: contextID},
		{Text: "three", ContextID: contextID},
	}
	if diff := cmp.Diff(want, agent.requests); diff != "" {
		t.Fatalf("a2a chat requests wrong result (-want +got) diff = %s", diff)
	}
	if !strings.Contains(stderr, "Error: failed to send message") {
		t.Fatalf("a2a chat stderr = %q, want the send error", stderr)
	}
}

func TestChatStreams(t *testing.T) {
	t.Parallel()
	agent, url := startChatAgent(t, a2a.TaskStateInputRequired, a2a.TaskStateCompleted)

	stdout, _, err := runChat(t, url, true, "two\n", "--stream", "one")
	if err != nil {
		t.Fatalf("a2a chat error = %v, want nil", err)
	}

	first := agent.tasks[0]
	want := []chatRequest{
		{Text: "one"},
		{Text: "two", TaskID: string(first.ID), ContextID: first.ContextID},
	}
	if diff := cmp.Diff(want, agent.requests); diff != "" {
		t.Fatalf("a2a chat requests wrong result (-want +got) diff = %s", diff)
	}
	if got := strings.Count(stdout, "[status] "); got != 2 {
		t.Fatalf("a2a chat printed %d status updates, want 2:\n%s", got, stdout)
	}
}

func TestChatTimeoutCancelsTheTask(t *testing.T) {
	t.Parallel()
	agent, url := startChatAgent(t, chatTurnHangs, a2a.TaskStateCompleted)

	_, stderr, err := runChat(t, url, true, "two\n", "--stream", "--timeout", "500ms", "one")
	if err != nil {
		t.Fatalf("a2a chat error = %v, want nil", err)
	}

	hung := agent.tasks[0]
	if diff := cmp.Diff([]string{string(hung.ID)}, agent.canceled); diff != "" {
		t.Fatalf("a2a chat canceled tasks wrong result (-want +got) diff = %s", diff)
	}
	want := []chatRequest{
		{Text: "one"},
		{Text: "two", ContextID: hung.ContextID},
	}
	if diff := cmp.Diff(want, agent.requests); diff != "" {
		t.Fatalf("a2a chat requests wrong result (-want +got) diff = %s", diff)
	}
	for _, want := range []string{"hint: increase --timeout", "Canceled task " + string(hung.ID)} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("a2a chat stderr = %q, want it to contain %q", stderr, want)
		}
	}
}

func TestChatInterrupt(t *testing.T) {
	t.Parallel()

	t.Run("cancels the running turn and keeps the session open", func(t *testing.T) {
		t.Parallel()
		agent, url := startChatAgent(t, a2a.TaskStateCompleted, chatTurnInterrupted, a2a.TaskStateCompleted)

		stderr, err := runChatTo(t, agent.deps(), url, strings.NewReader("two\nthree\n"), &bytes.Buffer{}, "one")
		if err != nil {
			t.Fatalf("a2a chat error = %v, want nil", err)
		}

		contextID := agent.tasks[0].ContextID
		want := []chatRequest{
			{Text: "one"},
			{Text: "two", ContextID: contextID},
			{Text: "three", ContextID: contextID},
		}
		if diff := cmp.Diff(want, agent.requests); diff != "" {
			t.Fatalf("a2a chat requests wrong result (-want +got) diff = %s", diff)
		}
		if !strings.Contains(stderr, "Interrupted") {
			t.Fatalf("a2a chat stderr = %q, want it to report the interrupted turn", stderr)
		}
	})

	t.Run("cancels the task the agent is working on", func(t *testing.T) {
		t.Parallel()
		agent, url := startChatAgent(t, chatTurnHangs)

		stderr, err := runChatTo(t, agent.deps(), url, strings.NewReader(""), interruptOnPrint{agent}, "--stream", "one")
		if err != nil {
			t.Fatalf("a2a chat error = %v, want nil", err)
		}

		id := string(agent.tasks[0].ID)
		if diff := cmp.Diff([]string{id}, agent.canceled); diff != "" {
			t.Fatalf("a2a chat canceled tasks wrong result (-want +got) diff = %s", diff)
		}
		if !strings.Contains(stderr, "Canceled task "+id) {
			t.Fatalf("a2a chat stderr = %q, want it to report the canceled task", stderr)
		}
	})

	t.Run("again abandons a pending cancel", func(t *testing.T) {
		t.Parallel()
		agent, url := startChatAgent(t, chatTurnHangs)
		agent.mu.Lock()
		agent.hangCancel = true
		agent.mu.Unlock()

		stderr, err := runChatTo(t, agent.deps(), url, strings.NewReader(""), interruptOnPrint{agent}, "--stream", "--timeout", "5s", "one")
		if err != nil {
			t.Fatalf("a2a chat error = %v, want nil", err)
		}

		id := string(agent.tasks[0].ID)
		if !strings.Contains(stderr, "Could not cancel task "+id) || !strings.Contains(stderr, "context canceled") {
			t.Fatalf("a2a chat stderr = %q, want it to report the abandoned cancel", stderr)
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
		// The pipe never delivers input, so only the interrupt can end the wait at the prompt.
		stdin, w := io.Pipe()
		t.Cleanup(func() {
			if err := w.Close(); err != nil {
				t.Errorf("io.PipeWriter.Close() error = %v", err)
			}
		})

		stderr, err := runChatTo(t, d, url, stdin, &bytes.Buffer{}, "--context-id", "ctx-1")
		if err != nil {
			t.Fatalf("a2a chat error = %v, want nil", err)
		}
		if got := len(agent.requests); got != 0 {
			t.Fatalf("a2a chat sent %d messages, want 0", got)
		}
		if !strings.Contains(stderr, "contextId: ctx-1") {
			t.Fatalf("a2a chat stderr = %q, want the context id", stderr)
		}
	})
}
