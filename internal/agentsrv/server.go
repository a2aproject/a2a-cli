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

// Package agentsrv is the agent server behind `a2a server`: one coding agent
// per process, started from its project directory, reachable over A2A on
// localhost, a private network and the internet, and from messengers.
package agentsrv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"github.com/a2aproject/a2a-cli/internal/agentdir"
	"github.com/a2aproject/a2a-cli/internal/auth"
	"github.com/a2aproject/a2a-cli/internal/channel"
	"github.com/a2aproject/a2a-cli/internal/network"
	"github.com/a2aproject/a2a-cli/internal/store"
)

// Options configure [Run].
type Options struct {
	Dir     agentdir.Dir
	Config  *agentdir.Config
	Secrets agentdir.Secrets
	// Out receives progress lines for the user.
	Out io.Writer
	// HarnessLog receives the harness's stderr. Discarded if nil.
	HarnessLog io.Writer
	// Executor replaces the ACP executor, for tests.
	Executor a2asrv.AgentExecutor
	// Channels replace the channels from the config, for tests.
	Channels []channel.Channel
	// Ready is called with the server once it serves requests.
	Ready func(*Server)
}

// Server is a running agent server.
type Server struct {
	dir     agentdir.Dir
	secrets agentdir.Secrets
	out     io.Writer
	started time.Time

	cfgMu sync.Mutex
	cfg   *agentdir.Config

	store   *store.Store
	tasks   *store.TaskStore
	exec    a2asrv.AgentExecutor
	handler a2asrv.RequestHandler
	chain   *auth.Chain
	bus     *eventBus

	private network.Provider
	public  network.Provider

	urlMu      sync.Mutex
	localURL   string
	privateURL string
	publicURL  string
	harnessURL string
	// harnessSocket is set when the harness reaches the server through a
	// Unix socket.
	harnessSocket string
	// harnessKey is required from the harness when it connects over TCP.
	harnessKey string

	router *Router
	peers  *Peers
	nodes  *Nodes

	// instructionsMu serializes rewrites of the harness instruction files.
	instructionsMu sync.Mutex
	// pairFailures limits guesses of pairing codes.
	pairFailures *failureLimiter

	// ctx ends when the server stops. Work which outlives a request, such as
	// watches, runs in it.
	ctx     context.Context
	cancel  context.CancelFunc
	closers []func(context.Context) error
}

// Run starts the agent server and serves until ctx ends or the server is
// stopped through its control socket.
func Run(ctx context.Context, opts Options) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if opts.Out == nil {
		opts.Out = io.Discard
	}
	s := &Server{
		dir:     opts.Dir,
		cfg:     opts.Config,
		secrets: opts.Secrets,
		out:     opts.Out,
		started: time.Now(),
		bus:     newEventBus(),
		ctx:     ctx,
		cancel:  cancel,

		pairFailures: newFailureLimiter(10, time.Minute),
	}
	defer s.shutdown()

	if err := s.dir.Init(); err != nil {
		return err
	}
	st, err := store.Open(s.dir.DBPath())
	if err != nil {
		return err
	}
	s.store = st
	s.closers = append(s.closers, func(context.Context) error { return st.Close() })
	s.tasks = st.Tasks("", s.viewer)
	n, err := s.tasks.FailUnfinished(ctx, "The agent server restarted before this task finished. Send the message again.")
	if err != nil {
		return err
	}
	if n > 0 {
		slog.Info("failed tasks interrupted by the previous run", "count", n)
	}

	if err := s.setupNetwork(); err != nil {
		return err
	}
	s.chain = &auth.Chain{Store: st, Net: s.private, Identities: s.cfg.Network.Identities}
	s.peers = newPeers(s)
	s.nodes = newNodes(s)

	harnessLn, err := s.listenHarness(ctx, opts.Executor == nil)
	if err != nil {
		return err
	}
	s.refreshInstructions(ctx)
	exec := opts.Executor
	if exec == nil {
		if exec, err = s.newExecutor(ctx, opts.HarnessLog); err != nil {
			return errors.Join(err, harnessLn.Close())
		}
	}
	s.exec = &observedExecutor{inner: exec, s: s}
	s.handler = a2asrv.NewHandler(s.exec,
		a2asrv.WithTaskStore(s.tasks),
		a2asrv.WithCallInterceptors(&callerInterceptor{s: s}),
		a2asrv.WithCapabilityChecks(&a2a.AgentCapabilities{Streaming: true}),
		a2asrv.WithLogger(slog.Default()),
	)

	chans := opts.Channels
	if chans == nil {
		if chans, err = s.buildChannels(); err != nil {
			return errors.Join(err, harnessLn.Close())
		}
	}
	// The router exists before any listener serves, because handlers use it.
	s.router = newRouter(ctx, s)
	s.closers = append(s.closers, s.router.stopAll)

	errc := make(chan error, 1)
	s.serve(ctx, harnessLn, network.ListenerHarness, "", errc)
	if err := s.listenLocal(ctx, errc); err != nil {
		return err
	}
	if err := s.listenNetwork(ctx, errc); err != nil {
		return err
	}
	if err := s.serveControl(ctx, errc); err != nil {
		return err
	}

	for _, ch := range chans {
		s.router.add(ch)
	}
	s.peers.resumeWatches(ctx)

	if err := s.printSummary(ctx); err != nil {
		return err
	}
	if opts.Ready != nil {
		opts.Ready(s)
	}

	select {
	case <-ctx.Done():
		return nil
	case err := <-errc:
		return err
	}
}

// Stop shuts the server down.
func (s *Server) Stop() { s.cancel() }

// shutdown stops background work first, then closes listeners and the store
// in reverse order of opening.
func (s *Server) shutdown() {
	s.cancel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for i := len(s.closers) - 1; i >= 0; i-- {
		if err := s.closers[i](ctx); err != nil {
			slog.Warn("shutdown", "error", err)
		}
	}
}

func (s *Server) config() *agentdir.Config {
	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()
	c := *s.cfg
	return &c
}

// updateConfig applies fn to the config and saves it.
func (s *Server) updateConfig(fn func(*agentdir.Config) error) error {
	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()
	next := *s.cfg
	if err := fn(&next); err != nil {
		return err
	}
	if err := s.dir.SaveConfig(&next); err != nil {
		return err
	}
	*s.cfg = next
	return nil
}

func (s *Server) logf(format string, args ...any) {
	if _, err := fmt.Fprintf(s.out, format+"\n", args...); err != nil {
		slog.Debug("writing progress", "error", err)
	}
}

// listenLocal opens the localhost listener. The first run picks a free port
// and saves it, so the local URL stays the same.
func (s *Server) listenLocal(ctx context.Context, errc chan<- error) error {
	port := s.config().Port
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return fmt.Errorf("listening on localhost:%d: %w", port, err)
	}
	if port == 0 {
		port = ln.Addr().(*net.TCPAddr).Port
		if err := s.updateConfig(func(c *agentdir.Config) error { c.Port = port; return nil }); err != nil {
			return errors.Join(err, ln.Close())
		}
	}
	u := "http://" + ln.Addr().String()
	s.urlMu.Lock()
	s.localURL = u
	s.urlMu.Unlock()
	s.serve(ctx, ln, network.ListenerLocal, u, errc)
	return nil
}

// serve serves the routes of a listener kind until shutdown.
func (s *Server) serve(ctx context.Context, ln net.Listener, kind, baseURL string, errc chan<- error) {
	srv := &http.Server{
		Handler:           s.routes(kind, baseURL),
		ReadHeaderTimeout: 30 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	s.closers = append(s.closers, func(ctx context.Context) error {
		err := srv.Shutdown(ctx)
		if errors.Is(err, context.DeadlineExceeded) {
			return srv.Close()
		}
		return err
	})
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			reportFatal(errc, fmt.Errorf("%s listener: %w", kind, err))
		}
	}()
}

// reportFatal hands err to Run, which stops on the first one; later errors
// are only logged.
func reportFatal(errc chan<- error, err error) {
	select {
	case errc <- err:
	default:
		slog.Error("server error after shutdown started", "error", err)
	}
}

// URLs returns the URLs the agent is reachable at.
func (s *Server) URLs() (local, private, public string) {
	s.urlMu.Lock()
	defer s.urlMu.Unlock()
	return s.localURL, s.privateURL, s.publicURL
}

// bestURL is the URL others should use: public, then private, then local.
func (s *Server) bestURL() string {
	local, private, public := s.URLs()
	switch {
	case public != "":
		return public
	case private != "":
		return private
	}
	return local
}

func (s *Server) printSummary(ctx context.Context) error {
	local, private, public := s.URLs()
	s.logf("✓ local    %s", local)
	if private != "" {
		s.logf("✓ private  %s", private)
	}
	if public != "" {
		s.logf("✓ public   %s", public)
	}
	return s.router.announce(ctx)
}

// viewer is the task store's view of the caller: owners see every task,
// everyone else their own. Calls without a principal come from the server
// itself.
func (s *Server) viewer(ctx context.Context) (string, bool, error) {
	if p, ok := principalOf(ctx); ok {
		return p.ID, p.Has(auth.RoleOwner), nil
	}
	return "", true, nil
}

func principalOf(ctx context.Context) (auth.Principal, bool) {
	if p, ok := auth.PrincipalFrom(ctx); ok {
		return p, true
	}
	if cc, ok := a2asrv.CallContextFrom(ctx); ok && cc.User != nil {
		if p, ok := cc.User.Attributes["principal"].(auth.Principal); ok {
			return p, true
		}
	}
	return auth.Principal{}, false
}

// canApprove reports whether the principal with id may answer permission requests.
func (s *Server) canApprove(ctx context.Context, p auth.Principal) bool {
	if auth.CanApprove(p, s.config().Approvals.Approvers) {
		return true
	}
	u, err := s.store.User(ctx, p.ID)
	return err == nil && auth.CanApprove(auth.Principal{ID: u.ID, Roles: u.Roles}, nil)
}

// inProcess returns a context for calling the A2A handler directly as p.
func inProcess(ctx context.Context, p auth.Principal) context.Context {
	ctx = auth.WithPrincipal(ctx, p)
	ctx, cc := a2asrv.NewCallContext(ctx, a2asrv.NewServiceParams(nil))
	cc.User = a2asrv.NewAuthenticatedUser(p.ID, map[string]any{"principal": p})
	return ctx
}

func pid() int { return os.Getpid() }
