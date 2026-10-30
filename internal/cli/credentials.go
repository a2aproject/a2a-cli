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
	"net/url"
	"strings"
	"sync"

	"github.com/a2aproject/a2a-go/v2/a2aclient"

	"github.com/a2aproject/a2a-cli/internal/credhelper"
	"github.com/a2aproject/a2a-cli/internal/unixsocket"
)

// initHTTPTransport routes HTTP requests through --unix-socket, if set.
func (g *globalConfig) initHTTPTransport() {
	if g.unixSocket != "" {
		g.httpTransport = unixsocket.Transport(g.unixSocket)
	}
}

// initCredentials sets up --credential-helper. Helpers run only when a
// request is made to a host in their scope.
func (g *globalConfig) initCredentials() {
	if helpers := g.credentialHelpers.Helpers(); len(helpers) > 0 {
		g.credentials = &credhelper.Source{Helpers: helpers, Logf: g.logf}
	}
}

// cardHeaders returns the helper headers for the card request to ref that
// --auth and --svc-param don't already set.
func (g *globalConfig) cardHeaders(ctx context.Context, ref string) (map[string][]string, error) {
	if g.credentials == nil || !isHTTPURL(ref) {
		return nil, nil
	}
	_, resp, err := g.credentials.Headers(ctx, ref)
	if err != nil || resp == nil {
		return nil, err
	}
	explicit := g.svcParams.Params()
	headers := map[string][]string{}
	for name, values := range resp.Headers {
		if len(explicit.Get(name)) == 0 {
			headers[name] = values
		}
	}
	return headers, nil
}

// credentialInterceptor adds the helper headers to every A2A call, chosen
// by the host of the interface the client is connected to.
func (g *globalConfig) credentialInterceptor() a2aclient.FactoryOption {
	var warn sync.Once
	return a2aclient.WithCallInterceptors(&credhelper.Interceptor{
		Source: g.credentials,
		OnApply: func(headers map[string][]string) {
			if !g.insecureGRPC {
				return
			}
			for name := range headers {
				if strings.EqualFold(name, "Authorization") {
					warn.Do(func() { _, _ = fmt.Fprintln(g.stderr(), insecureCredentialWarning) })
				}
			}
		},
	})
}

func isHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
