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
		_, _ = fmt.Fprintln(cfg.stderr(), insecureCredentialWarning)
	}
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
		err := clierr.Usage("either '--agent-card <ref>' or '--endpoint <url> --transport <t>' must be provided")
		err.Hint = "pass -a <agent URL or card> to name the agent"
		return nil, err
	}
}

// agentFlags returns the flags that select the agent, quoted for a shell, so a
// printed command can reach the same agent: the agent reference, then --tenant
// and --unix-socket when set. A reference that may carry a credential, in its
// query string or user info, is left out, and so is a missing one; it is
// replaced by a placeholder, or, with no other flags, the caller shows one.
func agentFlags(cfg *globalConfig) string {
	var flags []string
	switch {
	case cfg.agentCard.IsSet():
		if !mayCarryCredential(cfg.agentCard.URL()) {
			flags = append(flags, "-a "+shellQuote(cfg.agentCard.String()))
		}
	case cfg.url != "":
		if !mayCarryCredential(cfg.url) && len(cfg.transports) == 1 {
			flags = append(flags, "-e "+shellQuote(cfg.url)+" --transport "+shellQuote(cfg.transports[0]))
		}
	}
	if cfg.tenant == "" && cfg.unixSocket == "" {
		return strings.Join(flags, " ")
	}
	if len(flags) == 0 {
		flags = append(flags, "-a <agent>")
	}
	if cfg.tenant != "" {
		flags = append(flags, "--tenant "+shellQuote(cfg.tenant))
	}
	if cfg.unixSocket != "" {
		flags = append(flags, "--unix-socket "+shellQuote(cfg.unixSocket))
	}
	return strings.Join(flags, " ")
}

// mayCarryCredential reports whether a URL has a query string or user info,
// either of which may hold a token, or does not parse at all.
func mayCarryCredential(ref string) bool {
	u, err := url.Parse(ref)
	return err != nil || u.RawQuery != "" || u.ForceQuery || u.User != nil
}

// shellQuote returns s as a single shell word, wrapping it in single quotes
// unless it holds only characters a POSIX shell treats literally.
func shellQuote(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789@%+=:,./_-") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
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
	resolver := agentcard.NewResolver(&http.Client{Transport: cfg.httpTransport})
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
	httpClient := &http.Client{Transport: utils.ActivityTransport(cfg.httpTransport)}
	opts := []a2aclient.FactoryOption{a2aclient.WithDefaultsDisabled()}
	if cfg.credentials != nil {
		opts = append(opts, cfg.credentialInterceptor())
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
