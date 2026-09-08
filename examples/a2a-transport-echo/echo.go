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

package main

import (
	"context"
	"iter"
	"strings"
	"sync"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
)

// echoTransport is a custom a2aclient.Transport that fabricates responses
// locally instead of talking to a remote agent. It shows the shape of a real
// plugin transport without requiring any network dependency.
type echoTransport struct {
	endpoint string

	mu    sync.Mutex
	tasks map[a2a.TaskID]*a2a.Task
}

var _ a2aclient.Transport = (*echoTransport)(nil)

func newEchoTransport(endpoint string) *echoTransport {
	return &echoTransport{endpoint: endpoint, tasks: map[a2a.TaskID]*a2a.Task{}}
}

func (t *echoTransport) SendMessage(_ context.Context, _ a2aclient.ServiceParams, req *a2a.SendMessageRequest) (a2a.SendMessageResult, error) {
	task := t.completedTask(req.Message)
	t.mu.Lock()
	t.tasks[task.ID] = task
	t.mu.Unlock()
	return task, nil
}

func (t *echoTransport) SendStreamingMessage(_ context.Context, _ a2aclient.ServiceParams, req *a2a.SendMessageRequest) iter.Seq2[a2a.Event, error] {
	task := t.completedTask(req.Message)
	t.mu.Lock()
	t.tasks[task.ID] = task
	t.mu.Unlock()

	text := messageText(req.Message)
	return func(yield func(a2a.Event, error) bool) {
		submitted := &a2a.Task{ID: task.ID, ContextID: task.ContextID, Status: a2a.TaskStatus{State: a2a.TaskStateSubmitted}}
		if !yield(submitted, nil) {
			return
		}
		if !yield(a2a.NewStatusUpdateEvent(task, a2a.TaskStateWorking, nil), nil) {
			return
		}
		artifact := a2a.NewArtifactEvent(task, a2a.NewTextPart(text))
		artifact.LastChunk = true
		if !yield(artifact, nil) {
			return
		}
		yield(a2a.NewStatusUpdateEvent(task, a2a.TaskStateCompleted, nil), nil)
	}
}

func (t *echoTransport) GetTask(_ context.Context, _ a2aclient.ServiceParams, req *a2a.GetTaskRequest) (*a2a.Task, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	task, ok := t.tasks[req.ID]
	if !ok {
		return nil, a2a.ErrTaskNotFound
	}
	return task, nil
}

func (t *echoTransport) ListTasks(_ context.Context, _ a2aclient.ServiceParams, _ *a2a.ListTasksRequest) (*a2a.ListTasksResponse, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	tasks := make([]*a2a.Task, 0, len(t.tasks))
	for _, task := range t.tasks {
		tasks = append(tasks, task)
	}
	return &a2a.ListTasksResponse{Tasks: tasks}, nil
}

func (t *echoTransport) CancelTask(_ context.Context, _ a2aclient.ServiceParams, req *a2a.CancelTaskRequest) (*a2a.Task, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	task, ok := t.tasks[req.ID]
	if !ok {
		return nil, a2a.ErrTaskNotFound
	}
	task.Status.State = a2a.TaskStateCanceled
	return task, nil
}

func (t *echoTransport) SubscribeToTask(_ context.Context, _ a2aclient.ServiceParams, req *a2a.SubscribeToTaskRequest) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		t.mu.Lock()
		task, ok := t.tasks[req.ID]
		t.mu.Unlock()
		if !ok {
			yield(nil, a2a.ErrTaskNotFound)
			return
		}
		yield(a2a.NewStatusUpdateEvent(task, task.Status.State, nil), nil)
	}
}

func (t *echoTransport) GetTaskPushConfig(context.Context, a2aclient.ServiceParams, *a2a.GetTaskPushConfigRequest) (*a2a.PushConfig, error) {
	return nil, a2a.ErrPushNotificationNotSupported
}

func (t *echoTransport) ListTaskPushConfigs(context.Context, a2aclient.ServiceParams, *a2a.ListTaskPushConfigRequest) ([]*a2a.PushConfig, error) {
	return nil, a2a.ErrPushNotificationNotSupported
}

func (t *echoTransport) CreateTaskPushConfig(context.Context, a2aclient.ServiceParams, *a2a.PushConfig) (*a2a.PushConfig, error) {
	return nil, a2a.ErrPushNotificationNotSupported
}

func (t *echoTransport) DeleteTaskPushConfig(context.Context, a2aclient.ServiceParams, *a2a.DeleteTaskPushConfigRequest) error {
	return a2a.ErrPushNotificationNotSupported
}

func (t *echoTransport) GetExtendedAgentCard(context.Context, a2aclient.ServiceParams, *a2a.GetExtendedAgentCardRequest) (*a2a.AgentCard, error) {
	return &a2a.AgentCard{
		Name:               "Echo (via echo transport plugin)",
		Description:        "Echoes messages back; proxied from " + t.endpoint,
		Version:            "1.0.0",
		Capabilities:       a2a.AgentCapabilities{Streaming: true},
		DefaultInputModes:  []string{"text"},
		DefaultOutputModes: []string{"text"},
		Skills:             []a2a.AgentSkill{},
	}, nil
}

func (t *echoTransport) Destroy() error {
	return nil
}

func (t *echoTransport) completedTask(msg *a2a.Message) *a2a.Task {
	text := messageText(msg)
	task := &a2a.Task{
		ID:        a2a.NewTaskID(),
		ContextID: a2a.NewContextID(),
		Status:    a2a.TaskStatus{State: a2a.TaskStateCompleted},
		Artifacts: []*a2a.Artifact{{
			ID:    a2a.NewArtifactID(),
			Parts: a2a.ContentParts{a2a.NewTextPart(text)},
		}},
	}
	if msg != nil {
		task.History = []*a2a.Message{msg}
	}
	return task
}

func messageText(msg *a2a.Message) string {
	if msg == nil {
		return ""
	}
	var sb strings.Builder
	for i, part := range msg.Parts {
		if i > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString(part.Text())
	}
	return sb.String()
}
