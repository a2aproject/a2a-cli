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
	"net"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"

	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-cli/internal/flagparse"
	"github.com/a2aproject/a2a-cli/internal/transportplugin"
)

// Options select a local server mode and how it is served.
type Options struct {
	Echo  bool
	Proxy bool
	Exec  string
	// Chunk streams exec output split at this delimiter.
	Chunk string

	Host           string
	Port           int
	Protocol       string
	ServeTransport string
	CardCompat     bool
	Quiet          bool
	Logger         LoggerFunc
	Card           CardParams

	// Upstream creates the client of the proxied agent.
	Upstream func(ctx context.Context, opts ...a2aclient.FactoryOption) (*a2aclient.Client, error)
	// SvcParams and Tenant are forwarded to the proxied agent.
	SvcParams *flagparse.ServiceParams
	Tenant    string
}

// Run serves the echo, proxy or exec test server until ctx ends.
func Run(ctx context.Context, o Options) error {
	if o.Protocol != "latest" && o.Protocol != "0.3" {
		return fmt.Errorf("--protocol must be %q or %q", "latest", "0.3")
	}
	modes := 0
	for _, on := range []bool{o.Echo, o.Proxy, o.Exec != ""} {
		if on {
			modes++
		}
	}
	if modes > 1 {
		return clierr.Usage("--echo, --proxy, and --exec are mutually exclusive")
	}
	if modes == 0 {
		return clierr.Usage("specify --echo, --proxy, or --exec <cmd>")
	}
	transport, err := flagparse.SingleTransport([]string{o.ServeTransport})
	if err != nil {
		return err
	}
	if !transportplugin.IsBuiltin(transport) {
		return fmt.Errorf("serve %q transport not supported", transport)
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(o.Host, fmt.Sprint(o.Port)))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	sc := Config{
		Listener:        listener,
		Logger:          o.Logger,
		ProtocolVersion: a2a.ProtocolVersion(o.Protocol),
		CardCompat:      o.CardCompat,
		Quiet:           o.Quiet,
		CardParams:      o.Card,
	}
	sc.Transport = transport
	if sc.AdvertiseAddress == "" {
		sc.AdvertiseAddress = listener.Addr().String()
	}
	switch {
	case o.Echo:
		return ServeEcho(ctx, sc)
	case o.Proxy:
		var proxyOpts []a2aclient.FactoryOption
		if !o.Quiet {
			proxyOpts = append(proxyOpts, ProxyLogInterceptorOption())
		}
		client, err := o.Upstream(ctx, proxyOpts...)
		if err != nil {
			return fmt.Errorf("creating upstream client: %w", err)
		}
		return ServeProxy(ctx, sc, o.SvcParams, client, o.Tenant)
	default:
		return ServeExec(ctx, sc, o.Exec, o.Chunk)
	}
}
