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

package localsrv

import (
	"context"
	"fmt"
	"iter"
	"os"

	"github.com/a2aproject/a2a-cli/internal/flagparse"
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

// ProxyLogInterceptorOption returns a client factory option that logs proxied
// calls to stderr. Callers should apply it when building the upstream client so
// that proxied requests are traced unless running in quiet mode.
func ProxyLogInterceptorOption() a2aclient.FactoryOption {
	return a2aclient.WithCallInterceptors(&proxyLogInterceptor{})
}

// ServeProxy runs a local A2A server that forwards every request to the given
// upstream client, exposing the resolved (or supplied) agent card.
func ServeProxy(ctx context.Context, cfg Config, svcParams *flagparse.ServiceParams, client *a2aclient.Client, tenant string) error {
	card, err := resolveProxyCard(ctx, cfg, client, tenant)
	if err != nil {
		return err
	}

	handler := &ClientHandler{Client: client, Context: func(ctx context.Context) context.Context {
		if params := svcParams.Params(); len(params) > 0 {
			return a2aclient.AttachServiceParams(ctx, params)
		}
		return ctx
	}}

	if !cfg.Quiet {
		fmt.Fprintf(os.Stderr, "Proxying to %q agent\n", card.Name)
	}

	cfg.Logger("proxy mode, transport=%s protocol=%s", cfg.Transport, cfg.ProtocolVersion)

	return serve(ctx, cfg, handler, card)
}

func resolveProxyCard(ctx context.Context, cfg Config, client *a2aclient.Client, tenant string) (*a2a.AgentCard, error) {
	if cfg.CardPath != "" {
		return createAgentCard(cfg.CardParams)
	}

	upstream := client.Card()
	if upstream == nil {
		fetched, err := client.GetExtendedAgentCard(ctx, &a2a.GetExtendedAgentCardRequest{Tenant: tenant})
		if err != nil {
			return nil, fmt.Errorf("resolving upstream agent card: %w", err)
		}
		upstream = fetched
	}

	cardCopy := *upstream
	cardCopy.SupportedInterfaces = []*a2a.AgentInterface{getAgentInterface(cfg.CardParams)}
	return &cardCopy, nil
}

// ClientHandler serves A2A requests by forwarding them to an agent through
// Client. Context, if set, prepares the context of every forwarded request.
type ClientHandler struct {
	Client  *a2aclient.Client
	Context func(context.Context) context.Context
}

var _ a2asrv.RequestHandler = (*ClientHandler)(nil)

func (p *ClientHandler) ctx(ctx context.Context) context.Context {
	if p.Context != nil {
		return p.Context(ctx)
	}
	return ctx
}

func (p *ClientHandler) GetTask(ctx context.Context, req *a2a.GetTaskRequest) (*a2a.Task, error) {
	return p.Client.GetTask(p.ctx(ctx), req)
}

func (p *ClientHandler) ListTasks(ctx context.Context, req *a2a.ListTasksRequest) (*a2a.ListTasksResponse, error) {
	return p.Client.ListTasks(p.ctx(ctx), req)
}

func (p *ClientHandler) CancelTask(ctx context.Context, req *a2a.CancelTaskRequest) (*a2a.Task, error) {
	return p.Client.CancelTask(p.ctx(ctx), req)
}

func (p *ClientHandler) SendMessage(ctx context.Context, req *a2a.SendMessageRequest) (a2a.SendMessageResult, error) {
	return p.Client.SendMessage(p.ctx(ctx), req)
}

func (p *ClientHandler) SubscribeToTask(ctx context.Context, req *a2a.SubscribeToTaskRequest) iter.Seq2[a2a.Event, error] {
	return p.Client.SubscribeToTask(p.ctx(ctx), req)
}

func (p *ClientHandler) SendStreamingMessage(ctx context.Context, req *a2a.SendMessageRequest) iter.Seq2[a2a.Event, error] {
	return p.Client.SendStreamingMessage(p.ctx(ctx), req)
}

func (p *ClientHandler) GetTaskPushConfig(ctx context.Context, req *a2a.GetTaskPushConfigRequest) (*a2a.PushConfig, error) {
	return p.Client.GetTaskPushConfig(p.ctx(ctx), req)
}

func (p *ClientHandler) ListTaskPushConfigs(ctx context.Context, req *a2a.ListTaskPushConfigRequest) (*a2a.ListTaskPushConfigResponse, error) {
	resp, err := p.Client.ListTaskPushConfigs(p.ctx(ctx), req)
	if err != nil {
		return nil, err
	}
	return &a2a.ListTaskPushConfigResponse{Configs: resp}, nil
}

func (p *ClientHandler) CreateTaskPushConfig(ctx context.Context, req *a2a.PushConfig) (*a2a.PushConfig, error) {
	return p.Client.CreateTaskPushConfig(p.ctx(ctx), req)
}

func (p *ClientHandler) DeleteTaskPushConfig(ctx context.Context, req *a2a.DeleteTaskPushConfigRequest) error {
	return p.Client.DeleteTaskPushConfig(p.ctx(ctx), req)
}

func (p *ClientHandler) GetExtendedAgentCard(ctx context.Context, req *a2a.GetExtendedAgentCardRequest) (*a2a.AgentCard, error) {
	return p.Client.GetExtendedAgentCard(p.ctx(ctx), req)
}

type proxyLogInterceptor struct {
	a2aclient.PassthroughInterceptor
}

func (i *proxyLogInterceptor) After(_ context.Context, resp *a2aclient.Response) error {
	if resp.Err != nil {
		fmt.Fprintf(os.Stderr, "proxy %s → error: %v\n", resp.Method, resp.Err)
	} else {
		fmt.Fprintf(os.Stderr, "proxy %s → ok\n", resp.Method)
	}
	return nil
}
