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

package clitransport

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
)

// IO is used to redirect Run stdio.
type IO struct {
	// Out is where transport plugin writes handshake on start.
	Out io.Writer
	// Out is where transport plugin writes debug logs and errors.
	Err io.Writer
}

// Config describes a transport plugin.
type Config struct {
	// Name is the transport name.
	Name string
	// Version is the plugin's own version, reported by "info".
	Version string
	// Description is an optional human-readable summary reported by "info".
	Description string
	// Binding is the loopback binding the proxy serves. Defaults to [a2a.TransportProtocolGRPC].
	Binding a2a.TransportProtocol
	// NewTransport builds the custom client transport for the given upstream endpoint.
	NewTransport func(ctx context.Context, endpoint string) (a2aclient.Transport, error)
}

// Main runs the plugin using os.Args and exits the process with an appropriate
// status code. It is the intended entrypoint for a plugin's func main.
func Main(cfg Config) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	go func() { //stdin pipe closed
		defer stop()
		_, _ = io.Copy(io.Discard, bufio.NewReader(os.Stdin))
	}()

	defer stop()

	if err := Run(ctx, cfg, os.Args[1:], &IO{Out: os.Stdout, Err: os.Stderr}); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", cfg.Name, err)
		os.Exit(1)
	}
}

// Run executes a single plugin invocation exposed separately from [Main].
func Run(ctx context.Context, cfg Config, args []string, fds *IO) error {
	if cfg.Name == "" {
		return fmt.Errorf("clitransport: Config.Name is required")
	}
	if cfg.NewTransport == nil {
		return fmt.Errorf("clitransport: Config.NewTransport is required")
	}
	if len(args) == 0 {
		return fmt.Errorf("expected a subcommand: %s or %s", SubcommandServe, SubcommandInfo)
	}
	if cfg.Binding == "" {
		cfg.Binding = a2a.TransportProtocolGRPC
	}
	if fds == nil {
		fds = &IO{}
	}
	if fds.Out == nil {
		fds.Out = io.Discard
	}
	if fds.Err == nil {
		fds.Err = io.Discard
	}

	switch args[0] {
	case SubcommandServe:
		srv, err := runServe(ctx, cfg, args[1:], fds)
		if err != nil {
			if aerr := announce(fds.Out, Handshake{Success: false, Error: err.Error()}); aerr != nil {
				return fmt.Errorf("%w (and failed to announce: %v)", err, aerr)
			}
			return err
		}
		return srv.await()

	case SubcommandInfo:
		enc := json.NewEncoder(fds.Out)
		return enc.Encode(Info{
			Name:        cfg.Name,
			Version:     cfg.Version,
			Description: cfg.Description,
			Protocol:    a2a.Version,
			Binding:     cfg.Binding,
		})

	default:
		return fmt.Errorf("unknown subcommand %q (want %s or %s)", args[0], SubcommandServe, SubcommandInfo)
	}
}

func runServe(ctx context.Context, cfg Config, args []string, fds *IO) (*server, error) {
	fs := flag.NewFlagSet(SubcommandServe, flag.ContinueOnError)
	fs.SetOutput(fds.Err)

	endpoint := fs.String("endpoint", "", "Upstream endpoint URL to proxy to")
	bind := fs.String("bind", string(cfg.Binding), "Loopback binding to serve: grpc, jsonrpc or http+json")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	return serveProxy(ctx, cfg, *endpoint, a2a.TransportProtocol(*bind), fds)
}

func serveProxy(ctx context.Context, cfg Config, endpoint string, binding a2a.TransportProtocol, proc *IO) (*server, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("--endpoint is required")
	}

	transport, err := cfg.NewTransport(ctx, endpoint)
	if err != nil {
		return nil, fmt.Errorf("creating upstream transport: %w", err)
	}
	destroy := func() {
		if derr := transport.Destroy(); derr != nil {
			_, _ = fmt.Fprintf(proc.Err, "closing upstream transport: %v\n", derr)
		}
	}

	token := a2a.NewContextID()
	handler := newTransportHandler(transport, token)
	srv, err := newServer(binding, token, handler, cleanupFunc(destroy))
	if err != nil {
		destroy()
		return nil, err
	}

	if err := srv.start(ctx); err != nil {
		srv.cleanup()
		return nil, fmt.Errorf("server could not start: %w", err)
	}

	if err := announce(proc.Out, Handshake{Success: true, Endpoint: srv.body}); err != nil {
		srv.stop()
		return nil, fmt.Errorf("writing handshake: %w", err)
	}

	return srv, nil
}

func announce(stdout io.Writer, hs Handshake) error {
	data, err := json.Marshal(hs)
	if err != nil {
		return err
	}
	_, err = stdout.Write(append(data, '\n'))
	return err
}
