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
	"github.com/a2aproject/a2a-cli/internal/agentsrv/approvals"
	"github.com/a2aproject/a2a-cli/internal/agentsrv/nodes"
	"github.com/a2aproject/a2a-cli/internal/auth"
	"github.com/a2aproject/a2a-cli/internal/channel"
	"github.com/a2aproject/a2a-cli/internal/fsutil"
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
	cancel  context.CancelFunc

	cfgMu sync.Mutex
	cfg   *agentdir.Config

	store   *store.Store
	tasks   *store.TaskStore
	handler a2asrv.RequestHandler
	chain   *auth.Chain
	bus     *eventBus

	private network.Provider
	public  network.Provider
	bridge  harnessBridge

	urlMu      sync.Mutex
	localURL   string
	privateURL string
	publicURL  string

	router    *Router
	peers     *Peers
	nodes     *nodes.Host
	approvals *approvals.Service

	// instructionsMu serializes rewrites of the harness instruction files.
	instructionsMu sync.Mutex
	// pairFailures limits guesses of pairing codes.
	pairFailures *failureLimiter
}

// nodeWebhookRetries are the delays before each wake delivery attempt to a
// webhook node.
var nodeWebhookRetries = nodes.DefaultWebhookRetries

// stack holds what to close when the server stops, closed in reverse order
// of opening. Only Run owns one.
type stack []func(context.Context) error

func (st *stack) push(f func(context.Context) error) { *st = append(*st, f) }

func (st stack) close() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for i := len(st) - 1; i >= 0; i-- {
		if err := st[i](ctx); err != nil {
			slog.Warn("shutdown", "error", err)
		}
	}
}

// Run starts the agent server and serves until ctx ends or the server is
// stopped through its control socket. It opens the agent's state, builds
// the components, then serves; on the way out, background work stops first
// and everything opened is closed in reverse order.
func Run(ctx context.Context, opts Options) error {
	ctx, cancel := context.WithCancel(ctx)
	if opts.Out == nil {
		opts.Out = io.Discard
	}
	s := &Server{
		dir:     opts.Dir,
		cfg:     opts.Config,
		secrets: opts.Secrets,
		out:     opts.Out,
		started: time.Now(),
		cancel:  cancel,
		bus:     newEventBus(),

		pairFailures: newFailureLimiter(10, time.Minute),
	}
	var closers stack
	defer func() {
		cancel()
		closers.close()
	}()

	harnessLn, err := s.open(ctx, &closers, opts.Executor == nil)
	if err != nil {
		return err
	}
	if err := s.build(ctx, &closers, opts); err != nil {
		return err
	}
	chans := opts.Channels
	if chans == nil {
		if chans, err = s.buildChannels(); err != nil {
			return err
		}
	}
	errc, err := s.serve(ctx, &closers, harnessLn, chans)
	if err != nil {
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

// open locks the agent directory and opens its database, the network
// providers and the listener the harness reaches the server through.
func (s *Server) open(ctx context.Context, closers *stack, sandboxed bool) (net.Listener, error) {
	if err := s.dir.Init(); err != nil {
		return nil, err
	}
	release, err := fsutil.TryLock(s.dir.LockPath())
	if errors.Is(err, fsutil.ErrLocked) {
		return nil, fmt.Errorf("an agent server is already running in %s (a2a server status)", s.dir.Root)
	}
	if err != nil {
		return nil, fmt.Errorf("locking the agent directory: %w", err)
	}
	closers.push(func(context.Context) error { return release() })
	st, err := store.Open(s.dir.DBPath())
	if err != nil {
		return nil, err
	}
	closers.push(func(context.Context) error { return st.Close() })
	s.store = st
	s.tasks = st.Tasks("", s.viewer)
	n, err := s.tasks.FailUnfinished(ctx, "The agent server restarted before this task finished. Send the message again.")
	if err != nil {
		return nil, err
	}
	if n > 0 {
		slog.Info("failed tasks interrupted by the previous run", "count", n)
	}
	if err := s.setupNetwork(closers); err != nil {
		return nil, err
	}
	ln, bridge, err := s.listenHarness(sandboxed)
	if err != nil {
		return nil, err
	}
	closers.push(func(context.Context) error { return closeListener(ln) })
	s.bridge = bridge
	return ln, nil
}

// build creates the components and starts the harness. Nothing serves yet.
func (s *Server) build(ctx context.Context, closers *stack, opts Options) error {
	agentName := func() string { return s.config().Name }
	s.chain = &auth.Chain{Store: s.store, Net: s.private, Identities: s.config().Network.Identities}
	s.approvals = &approvals.Service{
		Store:     s.store,
		Tasks:     s.tasks,
		Approvers: func() []string { return s.config().Approvals.Approvers },
		BaseURL:   s.bestURL,
		AgentName: agentName,
	}
	s.nodes = nodes.New(ctx, nodes.Config{
		Store:          s.store,
		Viewer:         s.viewer,
		AgentName:      agentName,
		BaseURL:        s.bestURL,
		Authenticate:   s.authenticate,
		WebhookRetries: nodeWebhookRetries,
	})
	s.router = &Router{
		store:     s.store,
		tasks:     s.tasks,
		chain:     s.chain,
		approvals: s.approvals,
		bus:       s.bus,
		agentName: agentName,
		logf:      s.logf,
	}
	s.peers = &Peers{
		store:     s.store,
		private:   s.private,
		nodes:     s.nodes,
		approvals: s.approvals,
		chat:      s.router,
		agentName: agentName,
		bridge:    s.bridge,
		ctx:       ctx,
		watching:  map[string]bool{},
		clients:   map[string]*contactClient{},
	}
	closers.push(s.peers.close)

	s.refreshInstructions(ctx)
	exec := opts.Executor
	if exec == nil {
		acp, closeACP, err := s.newExecutor(ctx, opts.HarnessLog)
		if err != nil {
			return err
		}
		closers.push(closeACP)
		exec = acp
	}
	s.handler = a2asrv.NewHandler(&observedExecutor{inner: exec, bus: s.bus, peers: s.peers, approvals: s.approvals},
		a2asrv.WithTaskStore(s.tasks),
		a2asrv.WithCallInterceptors(&callerInterceptor{approvals: s.approvals}),
		a2asrv.WithCapabilityChecks(&a2a.AgentCapabilities{Streaming: true}),
		a2asrv.WithLogger(slog.Default()),
	)
	// The handler runs the executor, which hands questions to approvals;
	// approvals and the router send answers and turns back to the handler,
	// and approvals asks people through the router.
	s.router.handler = s.handler
	s.approvals.Handler = s.handler
	s.approvals.Notifier = s.router
	s.approvals.Remote = s.peers
	return nil
}

// serve starts the router, serves every listener and the control socket,
// starts the channels and resumes watches. Errors of listeners after the
// start arrive on the returned channel.
func (s *Server) serve(ctx context.Context, closers *stack, harnessLn net.Listener, chans []channel.Channel) (<-chan error, error) {
	errc := make(chan error, 1)
	s.router.start(ctx)
	closers.push(s.router.stopAll)
	closers.push(s.serveHTTP(ctx, harnessLn, network.ListenerHarness, "", errc))
	if err := s.listenLocal(ctx, closers, errc); err != nil {
		return nil, err
	}
	if err := s.listenNetwork(ctx, closers, errc); err != nil {
		return nil, err
	}
	if err := s.serveControl(ctx, closers, errc); err != nil {
		return nil, err
	}
	for _, ch := range chans {
		s.router.add(ch)
	}
	s.peers.resumeWatches(ctx)
	return errc, s.printSummary(ctx)
}

// Stop shuts the server down.
func (s *Server) Stop() { s.cancel() }

func closeListener(ln net.Listener) error {
	if err := ln.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		return err
	}
	return nil
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
func (s *Server) listenLocal(ctx context.Context, closers *stack, errc chan<- error) error {
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
	closers.push(s.serveHTTP(ctx, ln, network.ListenerLocal, u, errc))
	return nil
}

// serveHTTP serves the routes of a listener kind in the background and
// returns the function which shuts the listener down.
func (s *Server) serveHTTP(ctx context.Context, ln net.Listener, kind, baseURL string, errc chan<- error) func(context.Context) error {
	srv := &http.Server{
		Handler:           s.routes(kind, baseURL),
		ReadHeaderTimeout: 30 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			reportFatal(errc, fmt.Errorf("%s listener: %w", kind, err))
		}
	}()
	return func(ctx context.Context) error {
		err := srv.Shutdown(ctx)
		if errors.Is(err, context.DeadlineExceeded) {
			return srv.Close()
		}
		return err
	}
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
	if p, ok := auth.Caller(ctx); ok {
		return p.ID, p.Has(auth.RoleOwner), nil
	}
	return "", true, nil
}

func pid() int { return os.Getpid() }
