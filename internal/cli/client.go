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
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/a2aproject/a2a-cli/internal/flagparse"
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
	"github.com/a2aproject/a2a-go/v2/a2acompat/a2av0"
	a2agrpcv0 "github.com/a2aproject/a2a-go/v2/a2agrpc/v0"
	a2agrpc "github.com/a2aproject/a2a-go/v2/a2agrpc/v1"
)

var compatCardResolver = func() *agentcard.Resolver {
	resolver := agentcard.NewResolver(&http.Client{Timeout: 30 * time.Second})
	resolver.CardParser = a2av0.NewAgentCardParser()
	return resolver
}()

func newAgentClient(ctx context.Context, cfg *globalConfig, extraOpts ...a2aclient.FactoryOption) (*a2aclient.Client, error) {
	switch {
	case cfg.url != "" && cfg.agentCard.IsSet():
		return nil, fmt.Errorf("--endpoint and --agent-card are mutually exclusive")
	case cfg.url != "":
		return newClientFromEndpoint(ctx, cfg, cfg.url, extraOpts...)
	case cfg.agentCard.IsSet():
		return newClientFromCard(ctx, cfg, cfg.agentCard.URL(), extraOpts...)
	default:
		return nil, fmt.Errorf("either '--agent-card <ref>' or '--endpoint <url> --transport <t>' must be provided")
	}
}

// newClientFromEndpoint connects straight to an agent interface URL without resolving an
// Agent Card. --endpoint requires exactly one --transport to name the protocol binding.
func newClientFromEndpoint(ctx context.Context, cfg *globalConfig, ref string, extraOpts ...a2aclient.FactoryOption) (*a2aclient.Client, error) {
	protocol, err := flagparse.SingleTransport(cfg.transports)
	if err != nil {
		return nil, err
	}
	endpointURL := ref
	if protocol == a2a.TransportProtocolGRPC {
		endpointURL = stripHTTPScheme(ref)
	}
	cfg.logf("connecting directly to %s via %s (skipping card resolution)", endpointURL, protocol)

	endpoint := a2a.NewAgentInterface(endpointURL, protocol)
	if cfg.a2aVersion != "" {
		endpoint.ProtocolVersion = a2a.ProtocolVersion(cfg.a2aVersion)
	}
	client, err := a2aclient.NewFromEndpoints(ctx, []*a2a.AgentInterface{endpoint}, append(clientFactoryOpts(cfg), extraOpts...)...)
	return client, hintInsecure(err)
}

// newClientFromCard resolves the Agent Card and builds a client for it, honoring
// --transport as an ordered client preference over the card's declared interfaces.
func newClientFromCard(ctx context.Context, cfg *globalConfig, ref string, extraOpts ...a2aclient.FactoryOption) (*a2aclient.Client, error) {
	protos, err := flagparse.Transports(cfg.transports)
	if err != nil {
		return nil, err
	}

	cfg.logf("resolving agent card from %s", ref)

	var resolveOpts []agentcard.ResolveOption
	if auth := cfg.svcParams.Auth(); auth != "" {
		resolveOpts = append(resolveOpts, agentcard.WithRequestHeader("Authorization", auth))
	}
	card, err := compatCardResolver.Resolve(ctx, ref, resolveOpts...)
	if err != nil {
		return nil, fmt.Errorf("resolving agent card: %w", err)
	}

	factoryOpts := append(clientFactoryOpts(cfg), extraOpts...)
	if len(protos) > 0 {
		factoryOpts = append(factoryOpts, a2aclient.WithConfig(a2aclient.Config{PreferredTransports: protos}))
	}
	cfg.logf("creating client for %s", card.Name)
	client, err := a2aclient.NewFromCard(ctx, card, factoryOpts...)
	return client, hintInsecure(err)
}

// hintInsecure wraps gRPC "no transport security set" errors with a user-friendly suggestion to pass --insecure.
func hintInsecure(err error) error {
	if err != nil && strings.Contains(err.Error(), "no transport security set") {
		return fmt.Errorf("%w\n\nhint: pass --insecure to allow plaintext gRPC connections", err)
	}
	return err
}

func clientFactoryOpts(cfg *globalConfig) []a2aclient.FactoryOption {
	var grpcOpts []grpc.DialOption
	if cfg.insecureGRPC {
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	opts := []a2aclient.FactoryOption{a2aclient.WithDefaultsDisabled()}
	if cfg.a2aVersion == "" || cfg.a2aVersion == "1.0" {
		opts = append(
			opts,
			a2aclient.WithRESTTransport(nil),
			a2aclient.WithJSONRPCTransport(nil),
			a2agrpc.WithGRPCTransport(grpcOpts...),
		)
	}
	if cfg.a2aVersion == "" || cfg.a2aVersion == "0.3" {
		opts = append(
			opts,
			a2av0.WithRESTTransport(a2av0.RESTTransportConfig{}),
			a2av0.WithJSONRPCTransport(a2av0.JSONRPCTransportConfig{}),
			a2agrpcv0.WithGRPCTransport(grpcOpts...),
		)
	}
	return opts
}

func stripHTTPScheme(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return raw
	}
	return u.Host
}

// withServiceParams attaches the --svc-param entries (including the --auth
// shorthand) to ctx so they ride along with every request the client makes.
func withServiceParams(ctx context.Context, cfg *globalConfig) context.Context {
	if params := cfg.svcParams.Params(); len(params) > 0 {
		ctx = a2aclient.AttachServiceParams(ctx, params)
	}
	return ctx
}

func destroyClient(cfg *globalConfig, client *a2aclient.Client) {
	if err := client.Destroy(); err != nil {
		cfg.logf("failed to destroy client: %v", err)
	}
}
