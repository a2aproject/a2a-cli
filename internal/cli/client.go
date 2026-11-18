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
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/a2aproject/a2a-cli/internal/aicatalog"
	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-cli/internal/flagparse"
	"github.com/a2aproject/a2a-cli/internal/transportplugin"
	"github.com/a2aproject/a2a-cli/internal/unixsocket"
	"github.com/a2aproject/a2a-cli/internal/utils"
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
	"github.com/a2aproject/a2a-go/v2/a2acompat/a2av0"
	a2agrpcv0 "github.com/a2aproject/a2a-go/v2/a2agrpc/v0"
	a2agrpc "github.com/a2aproject/a2a-go/v2/a2agrpc/v1"
)

const insecureCredentialWarning = "warning: --insecure disables TLS for gRPC; a credential sent over gRPC may be exposed."

// warnInsecureCredential warns once when a credential is sent with --insecure.
func warnInsecureCredential(cfg *globalConfig) {
	if cfg.insecureGRPC && cfg.svcParams.HasCredential() {
		cfg.warnInsecure()
	}
}

// warnInsecure prints insecureCredentialWarning, once per command.
func (g *globalConfig) warnInsecure() {
	g.insecureWarning.Do(func() { _, _ = fmt.Fprintln(g.stderr(), insecureCredentialWarning) })
}

func newAgentClient(ctx context.Context, cfg *globalConfig, extraOpts ...a2aclient.FactoryOption) (*a2aclient.Client, error) {
	warnInsecureCredential(cfg)
	if err := cfg.initTLS(); err != nil {
		return nil, err
	}
	switch {
	case cfg.url != "" && cfg.agentCard.IsSet():
		return nil, clierr.Usage("--endpoint and --agent-card are mutually exclusive")
	case cfg.url != "" && cfg.catalog != "":
		return nil, clierr.Usage("--catalog applies to --agent-card, not --endpoint")
	case cfg.url != "":
		return newClientFromEndpoint(ctx, cfg, cfg.url, extraOpts...)
	case cfg.agentCard.IsSet():
		return newClientFromCard(ctx, cfg, extraOpts...)
	case cfg.catalog != "":
		return nil, clierr.Usage("--catalog requires --agent-card <identifier>")
	default:
		return nil, clierr.Usage("either '--agent-card <ref>' or '--endpoint <url> --transport <t>' must be provided")
	}
}

// newClientFromEndpoint connects straight to an agent interface URL without resolving an
// Agent Card. --endpoint requires exactly one --transport to name the protocol binding.
func newClientFromEndpoint(ctx context.Context, cfg *globalConfig, ref string, extraOpts ...a2aclient.FactoryOption) (*a2aclient.Client, error) {
	protocol, err := flagparse.SingleTransport(cfg.transports)
	if err != nil {
		return nil, err
	}
	cfg.logf("connecting directly to %s via %s (skipping card resolution)", ref, protocol)

	factoryOpts := append(clientFactoryOpts(cfg), extraOpts...)
	if !transportplugin.IsBuiltin(protocol) {
		pluginOpt, err := transportplugin.Load(protocol, cfg)
		if err != nil {
			return nil, err
		}
		factoryOpts = append(factoryOpts, pluginOpt)
	}

	endpoint := a2a.NewAgentInterface(ref, protocol)
	if cfg.a2aVersion != "" {
		endpoint.ProtocolVersion = a2a.ProtocolVersion(cfg.a2aVersion)
	}
	return a2aclient.NewFromEndpoints(ctx, []*a2a.AgentInterface{endpoint}, factoryOpts...)
}

// newClientFromCard resolves the Agent Card and builds a client for it, honoring
// --transport as an ordered client preference over the card's declared interfaces.
func newClientFromCard(ctx context.Context, cfg *globalConfig, extraOpts ...a2aclient.FactoryOption) (*a2aclient.Client, error) {
	transportPrefs, err := flagparse.Transports(cfg.transports)
	if err != nil {
		return nil, err
	}

	card, err := resolveAgentCard(ctx, cfg)
	if err != nil {
		return nil, err
	}

	factoryOpts := append(clientFactoryOpts(cfg), extraOpts...)
	pluginOpts, err := transportplugin.LoadForCard(card, cfg)
	if err != nil {
		return nil, err
	}
	factoryOpts = append(factoryOpts, pluginOpts...)
	if len(transportPrefs) > 0 {
		factoryOpts = append(factoryOpts, a2aclient.WithConfig(
			a2aclient.Config{PreferredTransports: transportPrefs},
		))
	}
	cfg.resolvedCard = card
	cfg.logf("creating client for %s", card.Name)
	return a2aclient.NewFromCard(ctx, card, factoryOpts...)
}

// errCatalogDocument marks a card request answered with an AI Catalog.
var errCatalogDocument = errors.New("the document is an AI Catalog, not an Agent Card")

// fetchCardDocument fetches the agent card with every service parameter sent as a
// request header, so a gateway authenticating the card fetch sees what the transport
// sends. It also returns the card as served. When allowCatalog is set and ref serves
// an AI Catalog, it returns a nil card and the catalog.
func fetchCardDocument(ctx context.Context, cfg *globalConfig, ref string, allowCatalog bool) (*a2a.AgentCard, []byte, error) {
	if err := cfg.initTLS(); err != nil {
		return nil, nil, err
	}
	var opts []agentcard.ResolveOption
	for k, vals := range cfg.svcParams.Params() {
		opts = append(opts, agentcard.WithRequestHeader(k, strings.Join(vals, ", ")))
	}
	headers, err := cfg.cardHeaders(ctx, ref)
	if err != nil {
		return nil, nil, err
	}
	for k, vals := range headers {
		opts = append(opts, agentcard.WithRequestHeader(k, strings.Join(vals, ", ")))
	}
	cfg.cardURL = cardRequestURL(ref)
	if token := cfg.cardToken(ctx, cfg.cardURL); token != "" {
		opts = append(opts, agentcard.WithRequestHeader("Authorization", "Bearer "+token))
	}
	var raw []byte
	resolver := cardResolver(cfg)
	parse := resolver.CardParser
	resolver.CardParser = func(body []byte) (*a2a.AgentCard, error) {
		raw = body
		if aicatalog.IsCatalogDocument(body) {
			return nil, errCatalogDocument
		}
		return parse(body)
	}
	cfg.logf("resolving agent card from %s", ref)
	card, err := resolver.Resolve(ctx, ref, opts...)
	if allowCatalog && errors.Is(err, errCatalogDocument) {
		return nil, raw, nil
	}
	if err != nil {
		return nil, nil, clierr.CardResolution(err)
	}
	return card, raw, nil
}

func cardResolver(cfg *globalConfig) *agentcard.Resolver {
	resolver := agentcard.NewResolver(&http.Client{Transport: cfg.agentTransport()})
	resolver.CardParser = a2av0.NewAgentCardParser()
	return resolver
}

func clientFactoryOpts(cfg *globalConfig) []a2aclient.FactoryOption {
	creds := credentials.NewTLS(&tls.Config{RootCAs: cfg.rootCAs})
	if cfg.insecureGRPC {
		creds = insecure.NewCredentials()
	}
	grpcOpts := []grpc.DialOption{grpc.WithTransportCredentials(creds)}
	if cfg.unixSocket != "" {
		grpcOpts = append(grpcOpts, grpc.WithContextDialer(unixsocket.Dialer(cfg.unixSocket)))
	}
	// The a2a-go default HTTP client caps every request at 3 minutes. Use one without a
	// total timeout so the --timeout context deadline is what bounds a request.
	// ActivityTransport lets SSE keep-alives reset the --stream inactivity timeout.
	httpClient := &http.Client{Transport: utils.ActivityTransport(cfg.agentTransport())}
	opts := []a2aclient.FactoryOption{a2aclient.WithDefaultsDisabled()}
	switch {
	case cfg.oauthEnabled():
		// Plugins run before interceptors: the credential helper's headers
		// have to be set before the OAuth plugin looks at a call.
		var plugins []a2aclient.Plugin
		if cfg.credentials != nil {
			plugins = append(plugins, beforePlugin{name: "a2a-cli-credential-helper", ic: cfg.credentialHelperInterceptor()})
		}
		plugins = append(plugins, oauthPlugin{inner: cfg.oauth.Plugin(), cfg: cfg})
		opts = append(opts, a2aclient.WithPlugins(plugins...))
		if cfg.insecureGRPC {
			opts = append(opts, a2aclient.WithCallInterceptors(insecureTokenWarning{cfg: cfg}))
		}
	case cfg.credentials != nil:
		opts = append(opts, a2aclient.WithCallInterceptors(cfg.credentialHelperInterceptor()))
	}
	if cfg.a2aVersion == "" || cfg.a2aVersion == "1.0" {
		opts = append(
			opts,
			a2aclient.WithRESTTransport(httpClient),
			a2aclient.WithJSONRPCTransport(httpClient),
			a2aclient.WithTransport(a2a.TransportProtocolGRPC, grpcTransportFactory(a2agrpc.NewGRPCTransport, cfg.unixSocket != "", grpcOpts...)),
		)
	}
	if cfg.a2aVersion == "" || cfg.a2aVersion == "0.3" {
		opts = append(
			opts,
			a2av0.WithRESTTransport(a2av0.RESTTransportConfig{Client: httpClient}),
			a2av0.WithJSONRPCTransport(a2av0.JSONRPCTransportConfig{Client: httpClient}),
			a2aclient.WithCompatTransport(a2av0.Version, a2a.TransportProtocolGRPC, grpcTransportFactory(a2agrpcv0.NewGRPCTransport, cfg.unixSocket != "", grpcOpts...)),
		)
	}
	return opts
}

// grpcTransportFactory dials the host and port of a gRPC interface URL, which
// cards may advertise with an http(s) scheme. The a2agrpc options pass the URL
// to grpc.NewClient as is, which cannot resolve such a target. With
// --unix-socket the host is not resolved, since the dialer ignores it.
func grpcTransportFactory(newTransport func(*grpc.ClientConn) a2aclient.Transport, unixSocket bool, opts ...grpc.DialOption) a2aclient.TransportFactory {
	return a2aclient.TransportFactoryFn(func(_ context.Context, _ *a2a.AgentCard, iface *a2a.AgentInterface) (a2aclient.Transport, error) {
		target := stripHTTPScheme(iface.URL)
		if unixSocket {
			target = "passthrough:///" + target
		}
		conn, err := grpc.NewClient(target, opts...)
		if err != nil {
			return nil, err
		}
		return newTransport(conn), nil
	})
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
