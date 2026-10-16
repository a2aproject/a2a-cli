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

package a2aclaude

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

// fakeClaude emits stream-json like `claude -p`. Without Bash approval it
// reports a permission denial; with it, it streams an answer.
const fakeClaude = `#!/bin/sh
echo "$@" >> "$ARGS_LOG"
cat > /dev/null
echo '{"type":"system","subtype":"init","session_id":"sess-1"}'
case "$*" in
*"Bash(rm x.txt)"*)
  echo '{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t2","name":"Bash","input":{"command":"rm x.txt"}}]}}'
  echo '{"type":"stream_event","event":{"type":"content_block_start","content_block":{"type":"text"}}}'
  echo '{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"Removed "}}}'
  echo '{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"x.txt."}}}'
  echo '{"type":"result","subtype":"success","is_error":false,"result":"Removed x.txt.","session_id":"sess-1"}'
  ;;
*)
  echo '{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"I need to run rm."}}}'
  echo '{"type":"result","subtype":"success","is_error":false,"result":"I need to run rm.","session_id":"sess-1","permission_denials":[{"tool_name":"Bash","tool_use_id":"t1","tool_input":{"command":"rm x.txt"}}]}'
  ;;
esac
`

func collect(t *testing.T, e *Executor, execCtx *a2asrv.ExecutorContext) []a2a.Event {
	t.Helper()
	var out []a2a.Event
	for ev, err := range e.Execute(context.Background(), execCtx) {
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		out = append(out, ev)
	}
	return out
}

func lastState(t *testing.T, events []a2a.Event) (a2a.TaskState, *a2a.Message) {
	t.Helper()
	su, ok := events[len(events)-1].(*a2a.TaskStatusUpdateEvent)
	if !ok {
		t.Fatalf("last event = %T, want status update", events[len(events)-1])
	}
	return su.Status.State, su.Status.Message
}

func artifactText(events []a2a.Event) string {
	var sb strings.Builder
	for _, ev := range events {
		if a, ok := ev.(*a2a.TaskArtifactUpdateEvent); ok {
			for _, p := range a.Artifact.Parts {
				sb.WriteString(p.Text())
			}
		}
	}
	return sb.String()
}

func TestPermissionDenialRoundTrip(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "claude")
	if err := os.WriteFile(bin, []byte(fakeClaude), 0o755); err != nil {
		t.Fatal(err)
	}
	argsLog := filepath.Join(dir, "args.log")
	e := NewExecutor(Config{Binary: bin, Dir: dir, Env: []string{"ARGS_LOG=" + argsLog}})

	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("delete x.txt"))
	execCtx := &a2asrv.ExecutorContext{Message: msg, TaskID: a2a.NewTaskID(), ContextID: "ctx-1"}

	events := collect(t, e, execCtx)
	state, status := lastState(t, events)
	if state != a2a.TaskStateInputRequired {
		t.Fatalf("first turn state = %s, want input-required", state)
	}
	if got := status.Parts[0].Text(); !strings.Contains(got, "Bash(rm x.txt)") {
		t.Errorf("question text = %q, want it to name the denied call", got)
	}

	answer := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("once"))
	execCtx2 := &a2asrv.ExecutorContext{Message: answer, TaskID: execCtx.TaskID, ContextID: "ctx-1", StoredTask: &a2a.Task{ID: execCtx.TaskID}}
	events = collect(t, e, execCtx2)
	state, _ = lastState(t, events)
	if state != a2a.TaskStateCompleted {
		t.Fatalf("second turn state = %s, want completed", state)
	}
	if got := artifactText(events); got != "\n\nRemoved x.txt." {
		t.Errorf("artifact text = %q", got)
	}

	data, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatal(err)
	}
	runs := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(runs) != 2 {
		t.Fatalf("claude ran %d times, want 2", len(runs))
	}
	if strings.Contains(runs[0], "--resume") {
		t.Errorf("first run resumed a session: %s", runs[0])
	}
	if !strings.Contains(runs[1], "--resume sess-1") || !strings.Contains(runs[1], "--allowedTools Bash(rm x.txt)") {
		t.Errorf("second run args = %s, want --resume sess-1 and the approved tool", runs[1])
	}
}

func TestDeniedAnswerContinuesWithoutTool(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "claude")
	if err := os.WriteFile(bin, []byte(fakeClaude), 0o755); err != nil {
		t.Fatal(err)
	}
	e := NewExecutor(Config{Binary: bin, Dir: dir, Env: []string{"ARGS_LOG=" + filepath.Join(dir, "args.log")}})
	execCtx := &a2asrv.ExecutorContext{Message: a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("delete x.txt")), TaskID: a2a.NewTaskID(), ContextID: "ctx-1"}
	collect(t, e, execCtx)

	answer := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("no"))
	events := collect(t, e, &a2asrv.ExecutorContext{Message: answer, TaskID: execCtx.TaskID, ContextID: "ctx-1", StoredTask: &a2a.Task{ID: execCtx.TaskID}})
	// The fake asks again because the tool is still not allowed.
	if state, _ := lastState(t, events); state != a2a.TaskStateInputRequired {
		t.Fatalf("state = %s, want input-required", state)
	}
}
