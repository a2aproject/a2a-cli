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

package transportplugin

import (
	"context"
	"fmt"
	"iter"
	"maps"
	"net/http"
	"net/url"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/a2aproject/a2a-cli/devkit/clitransport"
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	a2agrpc "github.com/a2aproject/a2a-go/v2/a2agrpc/v1"
)

type session struct {
	handshake *clitransport.Endpoint
	closeFunc func() error
}

func (s *session) close() error {
	if s.closeFunc != nil {
		return s.closeFunc()
	}
	return nil
}

type launcher interface {
	launch(ctx context.Context, binary, endpoint string) (*session, error)
}

func newPluginTransportFactory(binary string, launcher launcher) a2aclient.TransportFactory {
	if launcher == nil {
		launcher = execLauncher{}
	}
	return a2aclient.TransportFactoryFn(func(ctx context.Context, _ *a2a.AgentCard, iface *a2a.AgentInterface) (a2aclient.Transport, error) {
		session, err := launcher.launch(ctx, binary, iface.URL)
		if err != nil {
			return nil, fmt.Errorf("launching transport plugin %q: %w", binary, err)
		}
		base, err := buildBaseTransport(session.handshake)
		if err != nil {
			_ = session.close()
			return nil, err
		}
		return &proxyTransport{base: base, session: session}, nil
	})
}

func buildBaseTransport(hs *clitransport.Endpoint) (a2aclient.Transport, error) {
	switch hs.Binding {
	case a2a.TransportProtocolJSONRPC:
		client, err := httpClient(hs.CertPEM)
		if err != nil {
			return nil, err
		}
		return a2aclient.NewJSONRPCTransport(hs.Address, client), nil

	case a2a.TransportProtocolHTTPJSON:
		u, err := url.Parse(hs.Address)
		if err != nil {
			return nil, fmt.Errorf("failed to parse endpoint URL: %w", err)
		}
		client, err := httpClient(hs.CertPEM)
		if err != nil {
			return nil, err
		}
		return a2aclient.NewRESTTransport(u, client), nil

	case a2a.TransportProtocolGRPC:
		creds, err := grpcCreds(hs.CertPEM)
		if err != nil {
			return nil, err
		}
		conn, err := grpc.NewClient(hs.Address, grpc.WithTransportCredentials(creds))
		if err != nil {
			return nil, fmt.Errorf("dialing plugin gRPC server %q: %w", hs.Address, err)
		}
		return a2agrpc.NewGRPCTransport(conn), nil

	default:
		return nil, fmt.Errorf("plugin advertised unsupported binding %q", hs.Binding)
	}
}

// httpClient returns an HTTP client that trusts only the plugin's per-launch certificate.
func httpClient(certPEM string) (*http.Client, error) {
	if certPEM == "" {
		return nil, nil
	}
	tlsConfig, err := clitransport.ClientTLSConfig([]byte(certPEM))
	if err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	return &http.Client{Transport: transport}, nil
}

// grpcCreds returns transport credentials trusting only the plugin's per-launch certificate.
func grpcCreds(certPEM string) (credentials.TransportCredentials, error) {
	if certPEM == "" {
		return insecure.NewCredentials(), nil
	}
	tlsConfig, err := clitransport.ClientTLSConfig([]byte(certPEM))
	if err != nil {
		return nil, err
	}
	return credentials.NewTLS(tlsConfig), nil
}

type proxyTransport struct {
	base    a2aclient.Transport
	session *session
}

func (t *proxyTransport) GetTask(ctx context.Context, sp a2aclient.ServiceParams, req *a2a.GetTaskRequest) (*a2a.Task, error) {
	return t.base.GetTask(ctx, t.withToken(sp), req)
}

func (t *proxyTransport) ListTasks(ctx context.Context, sp a2aclient.ServiceParams, req *a2a.ListTasksRequest) (*a2a.ListTasksResponse, error) {
	return t.base.ListTasks(ctx, t.withToken(sp), req)
}

func (t *proxyTransport) CancelTask(ctx context.Context, sp a2aclient.ServiceParams, req *a2a.CancelTaskRequest) (*a2a.Task, error) {
	return t.base.CancelTask(ctx, t.withToken(sp), req)
}

func (t *proxyTransport) SendMessage(ctx context.Context, sp a2aclient.ServiceParams, req *a2a.SendMessageRequest) (a2a.SendMessageResult, error) {
	return t.base.SendMessage(ctx, t.withToken(sp), req)
}

func (t *proxyTransport) SubscribeToTask(ctx context.Context, sp a2aclient.ServiceParams, req *a2a.SubscribeToTaskRequest) iter.Seq2[a2a.Event, error] {
	return t.base.SubscribeToTask(ctx, t.withToken(sp), req)
}

func (t *proxyTransport) SendStreamingMessage(ctx context.Context, sp a2aclient.ServiceParams, req *a2a.SendMessageRequest) iter.Seq2[a2a.Event, error] {
	return t.base.SendStreamingMessage(ctx, t.withToken(sp), req)
}

func (t *proxyTransport) GetTaskPushConfig(ctx context.Context, sp a2aclient.ServiceParams, req *a2a.GetTaskPushConfigRequest) (*a2a.PushConfig, error) {
	return t.base.GetTaskPushConfig(ctx, t.withToken(sp), req)
}

func (t *proxyTransport) ListTaskPushConfigs(ctx context.Context, sp a2aclient.ServiceParams, req *a2a.ListTaskPushConfigRequest) ([]*a2a.PushConfig, error) {
	return t.base.ListTaskPushConfigs(ctx, t.withToken(sp), req)
}

func (t *proxyTransport) CreateTaskPushConfig(ctx context.Context, sp a2aclient.ServiceParams, req *a2a.PushConfig) (*a2a.PushConfig, error) {
	return t.base.CreateTaskPushConfig(ctx, t.withToken(sp), req)
}

func (t *proxyTransport) DeleteTaskPushConfig(ctx context.Context, sp a2aclient.ServiceParams, req *a2a.DeleteTaskPushConfigRequest) error {
	return t.base.DeleteTaskPushConfig(ctx, t.withToken(sp), req)
}

func (t *proxyTransport) GetExtendedAgentCard(ctx context.Context, sp a2aclient.ServiceParams, req *a2a.GetExtendedAgentCardRequest) (*a2a.AgentCard, error) {
	return t.base.GetExtendedAgentCard(ctx, t.withToken(sp), req)
}

func (p *proxyTransport) Destroy() error {
	err := p.base.Destroy()
	if cerr := p.session.close(); cerr != nil && err == nil {
		err = cerr
	}
	return err
}

func (t *proxyTransport) withToken(sp a2aclient.ServiceParams) a2aclient.ServiceParams {
	params := make(a2aclient.ServiceParams, len(sp)+1)
	maps.Copy(params, sp)
	params.Append(clitransport.TokenSvcParam, t.session.handshake.Token)
	return params
}
