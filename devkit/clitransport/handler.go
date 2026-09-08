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

package clitransport

import (
	"context"
	"crypto/subtle"
	"iter"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

// transportHandler adapts an [a2aclient.Transport] into an [a2asrv.RequestHandler].
type transportHandler struct {
	transport a2aclient.Transport
	token     string
}

var _ a2asrv.RequestHandler = (*transportHandler)(nil)

func newTransportHandler(t a2aclient.Transport, token string) *transportHandler {
	return &transportHandler{transport: t, token: token}
}

func (h *transportHandler) GetTask(ctx context.Context, req *a2a.GetTaskRequest) (*a2a.Task, error) {
	params, err := h.authorize(ctx)
	if err != nil {
		return nil, err
	}
	return h.transport.GetTask(ctx, params, req)
}

func (h *transportHandler) ListTasks(ctx context.Context, req *a2a.ListTasksRequest) (*a2a.ListTasksResponse, error) {
	params, err := h.authorize(ctx)
	if err != nil {
		return nil, err
	}
	return h.transport.ListTasks(ctx, params, req)
}

func (h *transportHandler) CancelTask(ctx context.Context, req *a2a.CancelTaskRequest) (*a2a.Task, error) {
	params, err := h.authorize(ctx)
	if err != nil {
		return nil, err
	}
	return h.transport.CancelTask(ctx, params, req)
}

func (h *transportHandler) SendMessage(ctx context.Context, req *a2a.SendMessageRequest) (a2a.SendMessageResult, error) {
	params, err := h.authorize(ctx)
	if err != nil {
		return nil, err
	}
	return h.transport.SendMessage(ctx, params, req)
}

func (h *transportHandler) SubscribeToTask(ctx context.Context, req *a2a.SubscribeToTaskRequest) iter.Seq2[a2a.Event, error] {
	params, err := h.authorize(ctx)
	if err != nil {
		return errorEvents(err)
	}
	return h.transport.SubscribeToTask(ctx, params, req)
}

func (h *transportHandler) SendStreamingMessage(ctx context.Context, req *a2a.SendMessageRequest) iter.Seq2[a2a.Event, error] {
	params, err := h.authorize(ctx)
	if err != nil {
		return errorEvents(err)
	}
	return h.transport.SendStreamingMessage(ctx, params, req)
}

func (h *transportHandler) GetTaskPushConfig(ctx context.Context, req *a2a.GetTaskPushConfigRequest) (*a2a.PushConfig, error) {
	params, err := h.authorize(ctx)
	if err != nil {
		return nil, err
	}
	return h.transport.GetTaskPushConfig(ctx, params, req)
}

func (h *transportHandler) ListTaskPushConfigs(ctx context.Context, req *a2a.ListTaskPushConfigRequest) (*a2a.ListTaskPushConfigResponse, error) {
	params, err := h.authorize(ctx)
	if err != nil {
		return nil, err
	}
	configs, err := h.transport.ListTaskPushConfigs(ctx, params, req)
	if err != nil {
		return nil, err
	}
	return &a2a.ListTaskPushConfigResponse{Configs: configs}, nil
}

func (h *transportHandler) CreateTaskPushConfig(ctx context.Context, req *a2a.PushConfig) (*a2a.PushConfig, error) {
	params, err := h.authorize(ctx)
	if err != nil {
		return nil, err
	}
	return h.transport.CreateTaskPushConfig(ctx, params, req)
}

func (h *transportHandler) DeleteTaskPushConfig(ctx context.Context, req *a2a.DeleteTaskPushConfigRequest) error {
	params, err := h.authorize(ctx)
	if err != nil {
		return err
	}
	return h.transport.DeleteTaskPushConfig(ctx, params, req)
}

func (h *transportHandler) GetExtendedAgentCard(ctx context.Context, req *a2a.GetExtendedAgentCardRequest) (*a2a.AgentCard, error) {
	params, err := h.authorize(ctx)
	if err != nil {
		return nil, err
	}
	return h.transport.GetExtendedAgentCard(ctx, params, req)
}

func (h *transportHandler) authorize(ctx context.Context) (a2aclient.ServiceParams, error) {
	callCtx, ok := a2asrv.CallContextFrom(ctx)
	if !ok {
		return nil, a2a.ErrUnauthenticated
	}
	got, _ := callCtx.ServiceParams().Get(TokenSvcParam)
	if len(got) != 1 || subtle.ConstantTimeCompare([]byte(got[0]), []byte(h.token)) != 1 {
		return nil, a2a.ErrUnauthenticated
	}
	params := a2aclient.ServiceParams{}
	for k, v := range callCtx.ServiceParams().List() {
		if strings.EqualFold(k, TokenSvcParam) {
			continue
		}
		params[k] = v
	}
	return params, nil
}

func errorEvents(err error) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		yield(nil, err)
	}
}
