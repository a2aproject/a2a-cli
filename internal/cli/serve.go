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
	"net"
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/flagparse"
	"github.com/a2aproject/a2a-cli/internal/localsrv"
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
)

func newServeCmd(cfg *globalConfig) *cobra.Command {
	var (
		port             int
		host             string
		name             string
		desc             string
		cardFile         string
		cardCompat       bool
		protocol         string
		serveTransport   string
		quiet            bool
		echo             bool
		proxy            bool
		execCmd          string
		chunk            string
		advertiseAddress string
	)

	cmd := &cobra.Command{
		Use:   "server",
		Short: "Start an A2A-compliant server",
		RunE: func(cmd *cobra.Command, args []string) error {
			if protocol != "latest" && protocol != "0.3" {
				return fmt.Errorf("--protocol must be %q or %q", "latest", "0.3")
			}

			modes := 0
			if echo {
				modes++
			}
			if proxy {
				modes++
			}
			if execCmd != "" {
				modes++
			}
			if modes > 1 {
				return fmt.Errorf("--echo, --proxy, and --exec are mutually exclusive")
			}
			if modes == 0 {
				return fmt.Errorf("specify --echo, --proxy <url>, or --exec <cmd>")
			}

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()

			listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
			if err != nil {
				return fmt.Errorf("listen: %w", err)
			}

			transport, err := flagparse.SingleTransport([]string{serveTransport})
			if err != nil {
				return err
			}
			sc := localsrv.Config{
				Listener: listener,
				Logger:   cfg.logf,

				ProtocolVersion: a2a.ProtocolVersion(protocol),
				CardCompat:      cardCompat,
				Quiet:           quiet,

				CardParams: localsrv.CardParams{
					AgentName:        name,
					AgentDesc:        desc,
					CardPath:         cardFile,
					Transport:        transport,
					AdvertiseAddress: advertiseAddress,
				},
			}
			if sc.AdvertiseAddress == "" {
				sc.AdvertiseAddress = listener.Addr().String()
			}

			switch {
			case echo:
				return localsrv.ServeEcho(ctx, sc)
			case proxy:
				var proxyOpts []a2aclient.FactoryOption
				if !quiet {
					proxyOpts = append(proxyOpts, localsrv.ProxyLogInterceptorOption())
				}
				client, err := newAgentClient(ctx, cfg, proxyOpts...)
				if err != nil {
					return fmt.Errorf("creating upstream client: %w", err)
				}
				return localsrv.ServeProxy(ctx, sc, cfg.svcParams, client, cfg.tenant)
			default:
				return localsrv.ServeExec(ctx, sc, execCmd, chunk)
			}
		},
	}

	f := cmd.Flags()
	f.IntVar(&port, "port", 8080, "Listen port")
	f.StringVar(&host, "host", "127.0.0.1", "Bind address")
	f.StringVar(&name, "name", "", "Agent name for the auto-generated card")
	f.StringVar(&desc, "description", "", "Agent description")
	f.StringVar(&cardFile, "card", "", "Serve a custom agent card JSON file")
	f.BoolVar(&cardCompat, "card-compat", false, "Serve the agent card in a dual v0.3/v1.0 format")
	f.StringVar(&protocol, "protocol", "latest", `Protocol version: "latest" or "0.3"`)
	f.StringVar(&serveTransport, "serve-transport", "rest", "Transport to serve: rest, jsonrpc, grpc")
	f.BoolVar(&quiet, "quiet", false, "Suppress traffic logging to stderr")
	f.BoolVar(&echo, "echo", false, "Echo mode: return the user's message as a response")
	f.BoolVar(&proxy, "proxy", false, "Proxy mode: forward requests to the agent specified using --agent-card or --endpoint")
	f.StringVar(&execCmd, "exec", "", "Exec mode: run a command as an A2A agent")
	f.StringVar(&chunk, "chunk", "", "Delimiter for streaming exec output (implies --exec)")
	f.StringVar(&advertiseAddress, "advertise-address", "", "Agent endpoint which will appear in the server agent card")

	return cmd
}
