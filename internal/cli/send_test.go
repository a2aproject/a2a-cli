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
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

// terminalExecutor ends every task in the state under test.
type terminalExecutor struct {
	state a2a.TaskState
}

func (e *terminalExecutor) Execute(_ context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		if !yield(a2a.NewSubmittedTask(execCtx, execCtx.Message), nil) {
			return
		}
		yield(a2a.NewStatusUpdateEvent(execCtx, e.state, nil), nil)
	}
}

func (e *terminalExecutor) Cancel(_ context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCanceled, nil), nil)
	}
}

func TestSendExitsForTerminalTaskState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		state    a2a.TaskState
		wantCode string
		wantExit int
	}{
		{name: "failed task", state: a2a.TaskStateFailed, wantCode: clierr.CodeTaskFailed, wantExit: 6},
		{name: "rejected task", state: a2a.TaskStateRejected, wantCode: clierr.CodeTaskRejected, wantExit: 7},
		{name: "canceled task", state: a2a.TaskStateCanceled, wantCode: clierr.CodeTaskCanceled, wantExit: 8},
		{name: "completed task", state: a2a.TaskStateCompleted},
		{name: "input required task", state: a2a.TaskStateInputRequired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			url := startTestServerWith(t, a2a.AgentCapabilities{}, &terminalExecutor{state: tt.state})

			_, err := runCMD(t, "send", "-a", url, "hello")

			if tt.wantCode == "" {
				if err != nil {
					t.Fatalf("send() error = %v, want nil", err)
				}
				return
			}
			var got *clierr.Error
			if !errors.As(err, &got) {
				t.Fatalf("send() error = %v, want a *clierr.Error", err)
			}
			if got.Code != tt.wantCode || got.Exit != tt.wantExit {
				t.Fatalf("send() error = %s/exit %d, want %s/exit %d", got.Code, got.Exit, tt.wantCode, tt.wantExit)
			}
		})
	}
}

func TestSendStreamExitsForTerminalTaskState(t *testing.T) {
	t.Parallel()

	url := startTestServerWith(t, a2a.AgentCapabilities{Streaming: true}, &terminalExecutor{state: a2a.TaskStateFailed})

	_, err := runCMD(t, "send", "-a", url, "--stream", "hello")

	var got *clierr.Error
	if !errors.As(err, &got) {
		t.Fatalf("send --stream error = %v, want a *clierr.Error", err)
	}
	if got.Code != clierr.CodeTaskFailed || got.Exit != 6 {
		t.Fatalf("send --stream error = %s/exit %d, want %s/exit 6", got.Code, got.Exit, clierr.CodeTaskFailed)
	}
}

func TestSendPollingFallbackExitsForTerminalTaskState(t *testing.T) {
	t.Parallel()

	url := startTestServerWith(t, a2a.AgentCapabilities{Streaming: false}, &terminalExecutor{state: a2a.TaskStateCanceled})

	_, err := runCMD(t, "send", "-a", url, "--stream", "hello")

	var got *clierr.Error
	if !errors.As(err, &got) {
		t.Fatalf("send --stream fallback error = %v, want a *clierr.Error", err)
	}
	if got.Code != clierr.CodeTaskCanceled || got.Exit != 8 {
		t.Fatalf("send --stream fallback error = %s/exit %d, want %s/exit 8", got.Code, got.Exit, clierr.CodeTaskCanceled)
	}
}
