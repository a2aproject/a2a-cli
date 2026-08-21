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

var _ a2asrv.RequestHandler = (*proxyHandler)(nil)

func ServeProxy(ctx context.Context, sc Config, svcParams *flagparse.ServiceParams, client *a2aclient.Client, tenant string) error {
	var clientOpts []a2aclient.FactoryOption
	if !sc.Quiet {
		clientOpts = append(clientOpts, a2aclient.WithCallInterceptors(&proxyLogInterceptor{}))
	}

	var card *a2a.AgentCard
	if sc.CardPath != "" {
		localCard, err := createAgentCard(sc.CardParams)
		if err != nil {
			return err
		}
		card = localCard
	} else if clientCard := client.Card(); card != nil {
		card = clientCard
	} else {
		extendedCard, err := client.GetExtendedAgentCard(ctx, &a2a.GetExtendedAgentCardRequest{Tenant: tenant})
		if err != nil {
			return fmt.Errorf("resolving upstream agent card: %w", err)
		}
		card = deriveProxyCard(extendedCard, sc.AdvertiseAddress, sc.Transport)
	}

	handler := &proxyHandler{client: client, svcParams: svcParams}

	if !sc.Quiet {
		fmt.Fprintf(os.Stderr, "Proxying to %q agent\n", card.Name)
	}

	sc.Logger("proxy mode, transport=%s protocol=%s", sc.Transport, sc.ProtocolVersion)

	return serve(ctx, sc, handler, card)
}

func deriveProxyCard(upstream *a2a.AgentCard, addr string, proto a2a.TransportProtocol) *a2a.AgentCard {
	card := *upstream
	card.SupportedInterfaces = []*a2a.AgentInterface{a2a.NewAgentInterface(addr, proto)}
	return &card
}

type proxyHandler struct {
	client    *a2aclient.Client
	svcParams *flagparse.ServiceParams
}

func (p *proxyHandler) withParams(ctx context.Context) context.Context {
	if params := p.svcParams.Params(); len(params) > 0 {
		return a2aclient.AttachServiceParams(ctx, params)
	}
	return ctx
}

func (p *proxyHandler) GetTask(ctx context.Context, req *a2a.GetTaskRequest) (*a2a.Task, error) {
	return p.client.GetTask(p.withParams(ctx), req)
}

func (p *proxyHandler) ListTasks(ctx context.Context, req *a2a.ListTasksRequest) (*a2a.ListTasksResponse, error) {
	return p.client.ListTasks(p.withParams(ctx), req)
}

func (p *proxyHandler) CancelTask(ctx context.Context, req *a2a.CancelTaskRequest) (*a2a.Task, error) {
	return p.client.CancelTask(p.withParams(ctx), req)
}

func (p *proxyHandler) SendMessage(ctx context.Context, req *a2a.SendMessageRequest) (a2a.SendMessageResult, error) {
	return p.client.SendMessage(p.withParams(ctx), req)
}

func (p *proxyHandler) SubscribeToTask(ctx context.Context, req *a2a.SubscribeToTaskRequest) iter.Seq2[a2a.Event, error] {
	return p.client.SubscribeToTask(p.withParams(ctx), req)
}

func (p *proxyHandler) SendStreamingMessage(ctx context.Context, req *a2a.SendMessageRequest) iter.Seq2[a2a.Event, error] {
	return p.client.SendStreamingMessage(p.withParams(ctx), req)
}

func (p *proxyHandler) GetTaskPushConfig(ctx context.Context, req *a2a.GetTaskPushConfigRequest) (*a2a.PushConfig, error) {
	return p.client.GetTaskPushConfig(p.withParams(ctx), req)
}

func (p *proxyHandler) ListTaskPushConfigs(ctx context.Context, req *a2a.ListTaskPushConfigRequest) (*a2a.ListTaskPushConfigResponse, error) {
	resp, err := p.client.ListTaskPushConfigs(p.withParams(ctx), req)
	if err != nil {
		return nil, err
	}
	return &a2a.ListTaskPushConfigResponse{Configs: resp}, nil
}

func (p *proxyHandler) CreateTaskPushConfig(ctx context.Context, req *a2a.PushConfig) (*a2a.PushConfig, error) {
	return p.client.CreateTaskPushConfig(p.withParams(ctx), req)
}

func (p *proxyHandler) DeleteTaskPushConfig(ctx context.Context, req *a2a.DeleteTaskPushConfigRequest) error {
	return p.client.DeleteTaskPushConfig(p.withParams(ctx), req)
}

func (p *proxyHandler) GetExtendedAgentCard(ctx context.Context, req *a2a.GetExtendedAgentCardRequest) (*a2a.AgentCard, error) {
	return p.client.GetExtendedAgentCard(p.withParams(ctx), req)
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
