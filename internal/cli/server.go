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
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-cli/internal/flagparse"
	"github.com/a2aproject/a2a-cli/internal/localsrv"
	"github.com/a2aproject/a2a-cli/internal/transportplugin"
	"github.com/a2aproject/a2a-go/v2/a2a"
)

func newServerCmd(cfg *globalConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "server",
		Aliases: []string{"serve"},
		Short:   "Start an A2A server",
		Long: `Start an A2A server in one of several modes.

echo, exec and proxy are for learning, demos and testing. opencode and claude
serve a coding agent harness. relay runs or uses a relay that gives agents
without a public address a public A2A URL. opencode, claude and relay are
prototypes.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return clierr.Usage(fmt.Sprintf("unknown server mode %q", args[0]))
			}
			return clierr.Usage("specify a server mode: echo, exec, proxy, opencode, claude or relay")
		},
	}
	cmd.AddCommand(
		newServerEchoCmd(cfg),
		newServerExecCmd(cfg),
		newServerProxyCmd(cfg),
		newServerOpencodeCmd(cfg),
		newServerClaudeCmd(cfg),
		newServerRelayCmd(cfg),
	)
	return cmd
}

// serverFlags are the flags shared by the local server modes.
type serverFlags struct {
	port             int
	host             string
	name             string
	desc             string
	cardFile         string
	cardCompat       bool
	protocol         string
	transport        string
	quiet            bool
	advertiseAddress string
}

func (s *serverFlags) register(f *pflag.FlagSet) {
	f.IntVar(&s.port, "port", 8080, "Listen port")
	f.StringVar(&s.host, "host", "127.0.0.1", "Bind address")
	f.StringVar(&s.name, "name", "", "Agent name for the auto-generated card")
	f.StringVar(&s.desc, "description", "", "Agent description")
	f.StringVar(&s.cardFile, "card", "", "Serve a custom agent card JSON file")
	f.BoolVar(&s.cardCompat, "card-compat", false, "Serve the agent card in a dual v0.3/v1.0 format")
	f.StringVar(&s.protocol, "protocol", "latest", `Protocol version: "latest" or "0.3"`)
	f.StringVar(&s.transport, "serve-transport", "rest", "Transport to serve: rest, jsonrpc, grpc")
	f.BoolVar(&s.quiet, "quiet", false, "Suppress traffic logging to stderr")
	f.StringVar(&s.advertiseAddress, "advertise-address", "", "Agent endpoint which will appear in the server agent card")
}

// listen validates the flags, opens the listener and returns the server config.
func (s *serverFlags) listen(cfg *globalConfig) (localsrv.Config, error) {
	if s.protocol != "latest" && s.protocol != "0.3" {
		return localsrv.Config{}, clierr.Usage(fmt.Sprintf("--protocol must be %q or %q", "latest", "0.3"))
	}
	transport, err := flagparse.SingleTransport([]string{s.transport})
	if err != nil {
		return localsrv.Config{}, err
	}
	if !transportplugin.IsBuiltin(transport) {
		return localsrv.Config{}, fmt.Errorf("serve %q transport not supported", transport)
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(s.host, fmt.Sprint(s.port)))
	if err != nil {
		return localsrv.Config{}, fmt.Errorf("listen: %w", err)
	}
	sc := localsrv.Config{
		Listener:        listener,
		Logger:          cfg.logf,
		ProtocolVersion: a2a.ProtocolVersion(s.protocol),
		CardCompat:      s.cardCompat,
		Quiet:           s.quiet,
		CardParams: localsrv.CardParams{
			AgentName:        s.name,
			AgentDesc:        s.desc,
			CardPath:         s.cardFile,
			Transport:        transport,
			AdvertiseAddress: s.advertiseAddress,
		},
	}
	if sc.AdvertiseAddress == "" {
		sc.AdvertiseAddress = listener.Addr().String()
	}
	return sc, nil
}

func signalContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
}
