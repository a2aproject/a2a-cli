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
	"iter"
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

// chatAgent answers each turn with a task in the next scripted state.
type chatAgent struct {
	mu       sync.Mutex
	states   []a2a.TaskState
	requests []chatRequest
	tasks    []*a2a.Task
}

func startChatAgent(t *testing.T, states ...a2a.TaskState) (*chatAgent, string) {
	t.Helper()
	agent := &chatAgent{states: states}
	server := httptest.NewServer(a2asrv.NewRESTHandler(a2asrv.NewHandler(
		a2asrv.AgentExecutorFunc(func(ctx context.Context, ec *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
			return func(yield func(a2a.Event, error) bool) {
				agent.mu.Lock()
				turn := len(agent.requests)
				agent.requests = append(agent.requests, chatRequest{
					Text:      ec.Message.Parts[0].Text(),
					TaskID:    string(ec.Message.TaskID),
					ContextID: ec.Message.ContextID,
				})
				task := &a2a.Task{
					ID:        ec.TaskID,
					ContextID: ec.ContextID,
					Status:    a2a.TaskStatus{State: agent.states[turn]},
				}
				agent.tasks = append(agent.tasks, task)
				agent.mu.Unlock()
				yield(task, nil)
			}
		}),
	)))
	t.Cleanup(server.Close)
	return agent, server.URL
}

func runChat(t *testing.T, url string, terminal bool, stdin string, args ...string) (stderr string, err error) {
	t.Helper()
	var out, errBuf bytes.Buffer
	cfg := &globalConfig{
		Printer:   output.NewPrinter(&out, output.ModeText),
		svcParams: &flagparse.ServiceParams{},
		errOut:    &errBuf,
	}
	root := mustNewRoot(t, cfg, deps{
		poller:     polling.Stream,
		cfgLoader:  clicfg.LoadEmpty,
		isTerminal: func() bool { return terminal },
	})
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(append([]string{"chat", "-e", url, "--transport", "rest"}, args...))
	err = root.Execute()
	return errBuf.String(), err
}

func TestChatCarriesTheConversationAcrossTurns(t *testing.T) {
	t.Parallel()
	agent, url := startChatAgent(t, a2a.TaskStateInputRequired, a2a.TaskStateCompleted, a2a.TaskStateCompleted)

	stderr, err := runChat(t, url, true, "two\nthree\n/quit\nnever sent\n", "one")
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

			stderr, err := runChat(t, url, true, tt.stdin)
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

		if _, err := runChat(t, url, false, "two\n", "one"); err != nil {
			t.Fatalf("chat error = %v, want nil", err)
		}
		if diff := cmp.Diff([]chatRequest{{Text: "one"}}, agent.requests); diff != "" {
			t.Fatalf("chat requests wrong (-want +got):\n%s", diff)
		}
	})

	t.Run("without a message is a usage error", func(t *testing.T) {
		t.Parallel()
		agent, url := startChatAgent(t)

		_, err := runChat(t, url, false, "one\n")
		if ce := clierr.Classify(err); ce == nil || ce.Code != clierr.CodeUsage {
			t.Fatalf("chat error = %v, want a usage error", err)
		}
		if got := len(agent.requests); got != 0 {
			t.Fatalf("chat sent %d messages, want 0", got)
		}
	})
}
