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

// newAgentClient builds a client for the agent selected by the connection flags:
// --url for a direct connection to an interface (bypassing card resolution,
// requiring exactly one --transport), or --agent-card to resolve a card and
// negotiate a transport (with --transport as an ordered preference).
func newAgentClient(ctx context.Context, cfg *globalConfig, extraOpts ...a2aclient.FactoryOption) (*a2aclient.Client, error) {
	switch {
	case cfg.url != "" && cfg.agentCard != "":
		return nil, fmt.Errorf("--url and --agent-card are mutually exclusive")
	case cfg.url != "":
		return dialDirect(ctx, cfg, cfg.url, extraOpts...)
	case cfg.agentCard != "":
		return dialCard(ctx, cfg, cfg.agentCard, extraOpts...)
	default:
		return nil, fmt.Errorf("specify the agent with --agent-card <ref> or --url <ref>")
	}
}

// dialDirect connects straight to an agent interface URL without resolving an
// Agent Card. Per the spec, --url requires exactly one --transport to name the
// protocol binding.
func dialDirect(ctx context.Context, cfg *globalConfig, ref string, extraOpts ...a2aclient.FactoryOption) (*a2aclient.Client, error) {
	protos, err := parseTransports(cfg.transports)
	if err != nil {
		return nil, err
	}
	if len(protos) != 1 {
		return nil, fmt.Errorf("--url requires exactly one --transport (rest, jsonrpc, or grpc)")
	}
	proto := protos[0]

	endpointURL := ref
	if proto == a2a.TransportProtocolGRPC {
		endpointURL = stripHTTPScheme(ref)
	}
	cfg.logf("connecting directly to %s via %s (skipping card resolution)", endpointURL, proto)
	endpoint := a2a.NewAgentInterface(endpointURL, proto)
	client, err := a2aclient.NewFromEndpoints(ctx, []*a2a.AgentInterface{endpoint}, append(clientFactoryOpts(cfg), extraOpts...)...)
	return client, hintInsecure(err)
}

// dialCard resolves the Agent Card at ref and builds a client for it, honoring
// --transport as an ordered client preference over the card's declared
// interfaces.
func dialCard(ctx context.Context, cfg *globalConfig, ref string, extraOpts ...a2aclient.FactoryOption) (*a2aclient.Client, error) {
	protos, err := parseTransports(cfg.transports)
	if err != nil {
		return nil, err
	}

	cfg.logf("resolving agent card from %s", ref)
	var resolveOpts []agentcard.ResolveOption
	if cfg.auth != "" {
		resolveOpts = append(resolveOpts, agentcard.WithRequestHeader("Authorization", cfg.auth))
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

// hintInsecure wraps gRPC "no transport security set" errors with a
// user-friendly suggestion to pass --insecure.
func hintInsecure(err error) error {
	if err != nil && strings.Contains(err.Error(), "no transport security set") {
		return fmt.Errorf("%w\n\nhint: pass --insecure to allow plaintext gRPC connections", err)
	}
	return err
}

func clientFactoryOpts(cfg *globalConfig) []a2aclient.FactoryOption {
	factoryOpts := []a2aclient.FactoryOption{
		a2av0.WithRESTTransport(a2av0.RESTTransportConfig{}),
		a2av0.WithJSONRPCTransport(a2av0.JSONRPCTransportConfig{}),
	}
	var grpcOpts []grpc.DialOption
	if cfg.insecureGRPC {
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	factoryOpts = append(factoryOpts,
		a2agrpcv0.WithGRPCTransport(grpcOpts...),
		a2agrpc.WithGRPCTransport(grpcOpts...),
	)
	return factoryOpts
}

func withServiceParams(ctx context.Context, cfg *globalConfig) context.Context {
	params := a2aclient.ServiceParams{}
	for _, kv := range cfg.svcParams {
		if k, v, ok := strings.Cut(kv, "="); ok {
			params.Append(k, v)
		}
	}
	if cfg.auth != "" {
		params.Append("Authorization", cfg.auth)
	}
	if len(params) > 0 {
		ctx = a2aclient.AttachServiceParams(ctx, params)
	}
	return ctx
}

// stripHTTPScheme converts an HTTP(S) URL to a bare host:port suitable for
// grpc.NewClient, which expects a target address without an HTTP scheme.
func stripHTTPScheme(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return raw
	}
	return u.Host
}

func parseTransport(s string) (a2a.TransportProtocol, error) {
	switch strings.ToLower(s) {
	case "rest":
		return a2a.TransportProtocolHTTPJSON, nil
	case "jsonrpc":
		return a2a.TransportProtocolJSONRPC, nil
	case "grpc":
		return a2a.TransportProtocolGRPC, nil
	default:
		return "", fmt.Errorf("unknown transport %q (use rest, jsonrpc, or grpc)", s)
	}
}

// parseTransports converts the repeatable --transport values into an ordered
// list of protocols, preserving the caller's preference order.
func parseTransports(ss []string) ([]a2a.TransportProtocol, error) {
	if len(ss) == 0 {
		return nil, nil
	}
	protos := make([]a2a.TransportProtocol, 0, len(ss))
	for _, s := range ss {
		proto, err := parseTransport(s)
		if err != nil {
			return nil, err
		}
		protos = append(protos, proto)
	}
	return protos, nil
}
