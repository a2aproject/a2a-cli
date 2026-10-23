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
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/a2aproject/a2a-cli/internal/agentdir"
	"github.com/a2aproject/a2a-cli/internal/agentsrv"
	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-cli/internal/flagparse"
	"github.com/a2aproject/a2a-cli/internal/harness"
	"github.com/a2aproject/a2a-cli/internal/localsrv"
	"github.com/a2aproject/a2a-cli/internal/transportplugin"
	"github.com/a2aproject/a2a-cli/internal/wizard"
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
)

// serverFlags holds the flags of `a2a server` itself: the agent directory,
// background modes, and the echo, proxy and exec modes.
type serverFlags struct {
	dir     string
	detach  bool
	service bool

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
}

func newServerCmd(cfg *globalConfig) *cobra.Command {
	var f serverFlags
	cmd := &cobra.Command{
		Use:     "server",
		Aliases: []string{"serve"},
		Short:   "Run a coding agent as an A2A server, or a local test server",
		Long: `Run a coding agent as an A2A server: one agent per process, started from its
project directory, sandboxed in Docker, reachable over A2A on localhost, a
private network (Tailscale, LAN) and the internet (Funnel, ngrok), and from
Telegram, Discord and email.

  a2a server opencode --name coder     first run in a project: saves .a2a/config.yaml
  a2a server                           later runs, from the project or a subdirectory

Management commands (pair, status, contact, ...) find the agent the way
git finds a repository, or take --dir.

--echo, --proxy and --exec run a local test server instead.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if f.echo || f.proxy || f.execCmd != "" {
				return runLocalServer(cmd.Context(), cfg, &f)
			}
			return runConfiguredAgent(cmd, cfg, &f)
		},
	}
	pf := cmd.PersistentFlags()
	pf.StringVar(&f.dir, "dir", "", "Agent directory (default: the nearest directory with .a2a, walking up from the current one)")

	fl := cmd.Flags()
	fl.BoolVar(&f.detach, "detach", false, "Run the agent in the background")
	fl.BoolVar(&f.service, "service", false, "Install and start the agent as a launchd or systemd user service")
	fl.IntVar(&f.port, "port", 8080, "Listen port (test servers)")
	fl.StringVar(&f.host, "host", "127.0.0.1", "Bind address (test servers)")
	fl.StringVar(&f.name, "name", "", "Agent name for the auto-generated card (test servers)")
	fl.StringVar(&f.desc, "description", "", "Agent description (test servers)")
	fl.StringVar(&f.cardFile, "card", "", "Serve a custom agent card JSON file (test servers)")
	fl.BoolVar(&f.cardCompat, "card-compat", false, "Serve the agent card in a dual v0.3/v1.0 format (test servers)")
	fl.StringVar(&f.protocol, "protocol", "latest", `Protocol version: "latest" or "0.3" (test servers)`)
	fl.StringVar(&f.serveTransport, "serve-transport", "rest", "Transport to serve: rest, jsonrpc, grpc (test servers)")
	fl.BoolVar(&f.quiet, "quiet", false, "Suppress traffic logging to stderr (test servers)")
	fl.BoolVar(&f.echo, "echo", false, "Echo mode: return the user's message as a response")
	fl.BoolVar(&f.proxy, "proxy", false, "Proxy mode: forward requests to the agent specified using --agent-card or --endpoint")
	fl.StringVar(&f.execCmd, "exec", "", "Exec mode: run a command as an A2A agent")
	fl.StringVar(&f.chunk, "chunk", "", "Delimiter for streaming exec output (implies --exec)")
	fl.StringVar(&f.advertiseAddress, "advertise-address", "", "Agent endpoint which will appear in the server agent card (test servers)")

	for _, name := range harness.ProfileNames() {
		cmd.AddCommand(newServerHarnessCmd(cfg, &f, name))
	}
	cmd.AddCommand(
		newServerHarnessCmd(cfg, &f, "acp"),
		newServerStatusCmd(cfg, &f),
		newServerLogsCmd(cfg, &f),
		newServerStopCmd(cfg, &f),
		newServerPairCmd(cfg, &f),
		newServerPeersCmd(cfg, &f),
		newServerContactCmd(cfg, &f),
		newServerChannelCmd(cfg, &f),
	)
	return cmd
}

// agentDir returns the agent of --dir, or the nearest one up from the cwd.
func (f *serverFlags) agentDir() (agentdir.Dir, error) {
	if f.dir != "" {
		d, err := agentdir.Open(f.dir)
		if err != nil {
			return agentdir.Dir{}, err
		}
		if !d.Exists() {
			return agentdir.Dir{}, fmt.Errorf("%s has no .a2a/config.yaml; start an agent there with `a2a server <harness>`", d.Root)
		}
		return d, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return agentdir.Dir{}, err
	}
	return agentdir.Find(wd)
}

// control returns a client for the running agent.
func (f *serverFlags) control() (*agentsrv.Client, error) {
	d, err := f.agentDir()
	if err != nil {
		return nil, err
	}
	return agentsrv.NewClient(d), nil
}

// runConfiguredAgent starts the agent saved in .a2a/config.yaml, or asks
// for its settings on the first run in a terminal.
func runConfiguredAgent(cmd *cobra.Command, cfg *globalConfig, f *serverFlags) error {
	d, err := f.agentDir()
	if errors.Is(err, agentdir.ErrNotFound) && f.dir == "" {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return clierr.Usage("no agent here: start one with `a2a server <harness>` (" + strings.Join(harness.ProfileNames(), ", ") + "), or a test server with --echo, --proxy or --exec")
		}
		return runWizard(cmd, cfg)
	}
	if err != nil {
		return err
	}
	return runAgent(cmd.Context(), cfg, d, f.detach, f.service)
}

func runWizard(cmd *cobra.Command, cfg *globalConfig) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	d, err := agentdir.Open(wd)
	if err != nil {
		return err
	}
	res, err := wizard.Run(os.Stdin, cfg.Out, d.DefaultName(), harness.ProfileNames(), os.Getenv)
	if err != nil {
		return err
	}
	if err := saveSettings(d, res.Settings); err != nil {
		return err
	}
	return runAgent(cmd.Context(), cfg, d, res.Mode == wizard.ModeDetach, res.Mode == wizard.ModeService)
}

// saveSettings merges start flags into the agent's config and secrets.
func saveSettings(d agentdir.Dir, s agentdir.Settings) error {
	conf := &agentdir.Config{}
	if d.Exists() {
		var err error
		if conf, err = d.LoadConfig(); err != nil {
			return err
		}
	}
	secrets, err := s.Apply(conf, d.DefaultName())
	if err != nil {
		return clierr.Usage(err.Error())
	}
	if err := d.SaveSecrets(secrets); err != nil {
		return err
	}
	return d.SaveConfig(conf)
}

// runAgent runs the agent in the foreground, or starts it in the background.
func runAgent(ctx context.Context, cfg *globalConfig, d agentdir.Dir, detach, service bool) error {
	if detach || service {
		return agentsrv.Background(ctx, d, service, cfg.Out)
	}
	ctx, stop := signalContext(ctx)
	defer stop()
	return agentsrv.Foreground(ctx, d, cfg.Out, cfg.verbose, func() {
		if _, err := fmt.Fprintln(cfg.Out, "  (running in the foreground; Ctrl-C to stop, or start with --detach)"); err != nil {
			slog.Debug("writing progress", "error", err)
		}
	})
}

func signalContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
}

// runLocalServer runs the echo, proxy or exec test server.
func runLocalServer(ctx context.Context, cfg *globalConfig, f *serverFlags) error {
	if f.protocol != "latest" && f.protocol != "0.3" {
		return fmt.Errorf("--protocol must be %q or %q", "latest", "0.3")
	}
	modes := 0
	for _, on := range []bool{f.echo, f.proxy, f.execCmd != ""} {
		if on {
			modes++
		}
	}
	if modes > 1 {
		return clierr.Usage("--echo, --proxy, and --exec are mutually exclusive")
	}
	ctx, stop := signalContext(ctx)
	defer stop()
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", f.host, f.port))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	transport, err := flagparse.SingleTransport([]string{f.serveTransport})
	if err != nil {
		return err
	}
	if !transportplugin.IsBuiltin(transport) {
		return fmt.Errorf("serve %q transport not supported", transport)
	}
	sc := localsrv.Config{
		Listener:        listener,
		Logger:          cfg.logf,
		ProtocolVersion: a2a.ProtocolVersion(f.protocol),
		CardCompat:      f.cardCompat,
		Quiet:           f.quiet,
		CardParams: localsrv.CardParams{
			AgentName:        f.name,
			AgentDesc:        f.desc,
			CardPath:         f.cardFile,
			Transport:        transport,
			AdvertiseAddress: f.advertiseAddress,
		},
	}
	if sc.AdvertiseAddress == "" {
		sc.AdvertiseAddress = listener.Addr().String()
	}
	switch {
	case f.echo:
		return localsrv.ServeEcho(ctx, sc)
	case f.proxy:
		var proxyOpts []a2aclient.FactoryOption
		if !f.quiet {
			proxyOpts = append(proxyOpts, localsrv.ProxyLogInterceptorOption())
		}
		client, err := newAgentClient(ctx, cfg, proxyOpts...)
		if err != nil {
			return fmt.Errorf("creating upstream client: %w", err)
		}
		return localsrv.ServeProxy(ctx, sc, cfg.svcParams, client, cfg.tenant)
	default:
		return localsrv.ServeExec(ctx, sc, f.execCmd, f.chunk)
	}
}
