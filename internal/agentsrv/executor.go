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

package agentsrv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"github.com/a2aproject/a2a-go/v2/executor/a2aacp"

	"github.com/a2aproject/a2a-cli/internal/agentsrv/approvals"
	"github.com/a2aproject/a2a-cli/internal/auth"
	"github.com/a2aproject/a2a-cli/internal/cryptoutil"
	"github.com/a2aproject/a2a-cli/internal/fsutil"
	"github.com/a2aproject/a2a-cli/internal/harness"
)

// harnessSocketName is the name of the Unix socket the harness reaches the
// server through, in the harness socket directory.
const harnessSocketName = "a2a.sock"

// newExecutor starts the harness as an ACP agent in its sandbox. close stops
// the harness and cleans up its sandbox.
func (s *Server) newExecutor(ctx context.Context, harnessLog, imageProgress io.Writer) (exec a2asrv.AgentExecutor, close func(context.Context) error, err error) {
	cfg := s.config()
	profile, err := harness.Lookup(cfg.Harness, cfg.Command)
	if err != nil {
		return nil, nil, err
	}
	mounts, err := s.dir.Mounts(cfg)
	if err != nil {
		return nil, nil, err
	}
	env := append([]string{}, cfg.Env...)
	for _, name := range cfg.Secrets {
		v, ok := s.secrets[name]
		if !ok {
			return nil, nil, fmt.Errorf("secret %s is not set; pass --secret %s again with the variable set", name, name)
		}
		env = append(env, name+"="+v)
	}
	spec := harness.Spec{
		Profile:    profile,
		Sandbox:    cfg.Sandbox,
		Mounts:     mounts,
		Home:       s.dir.HomeDir(),
		Hidden:     []string{s.dir.Path()},
		Env:        env,
		Name:       "a2a-" + cfg.Name + "-" + s.dir.ID(),
		Image:      cfg.Image,
		Limits:     harness.Limits{Memory: cfg.Limits.Memory, CPUs: cfg.Limits.CPUs, PIDs: cfg.Limits.PIDs},
		Labels:     map[string]string{"a2a.agent": cfg.Name, "a2a.dir": s.dir.Root},
		Socket:     s.bridge.socket,
		SocketHost: harnessHost,
		Progress:   imageProgress,
	}
	if exe, err := os.Executable(); err == nil {
		spec.CLI = exe
	} else {
		s.logf("! the a2a CLI isn't available to the harness: %v", err)
	}
	prepared, err := harness.Prepare(ctx, spec)
	if err != nil {
		return nil, nil, err
	}
	s.logf("✓ %s", prepared.Summary)
	mountDesc := make([]string, len(mounts))
	for i, m := range mounts {
		mountDesc[i] = m.String()
	}
	s.logf("✓ workspace %s; config %s", strings.Join(mountDesc, ", "), s.dir.ConfigPath())

	acp, err := a2aacp.NewExecutor(ctx, a2aacp.Config{
		Command:    prepared.Args,
		Dir:        mounts[0].Src,
		Env:        prepared.Env,
		Stderr:     harnessLog,
		AllowTools: cfg.Approvals.Allow,
		DenyTools:  cfg.Approvals.Deny,
		Sessions:   s.store.Sessions(),
	})
	if err != nil {
		return nil, nil, errors.Join(fmt.Errorf("starting %s: %w", profile.Name(), err), prepared.Stop(ctx))
	}
	s.logf("✓ %s is running (ACP)", profile.Name())
	return acp, func(ctx context.Context) error { return errors.Join(acp.Close(), prepared.Stop(ctx)) }, nil
}

// listenHarness opens the listener through which the harness calls other
// agents: a Unix socket, which is mounted into a container, or TCP with a key
// for Docker Desktop, which can't mount it.
func (s *Server) listenHarness(sandboxed bool) (net.Listener, harnessBridge, error) {
	bind, dial, tcp := harness.TCPBridge(s.config().Sandbox)
	if !sandboxed || !tcp {
		return s.listenHarnessSocket()
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(bind, "0"))
	if err != nil {
		return nil, harnessBridge{}, fmt.Errorf("listening for the harness on %s: %w", bind, err)
	}
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		return nil, harnessBridge{}, errors.Join(err, ln.Close())
	}
	b := harnessBridge{url: "http://" + net.JoinHostPort(dial, port), key: cryptoutil.RandomString(40, cryptoutil.Alphanumeric)}
	if err := saveHarnessKey(s.dir.HomeDir(), b.url, b.key); err != nil {
		return nil, harnessBridge{}, errors.Join(fmt.Errorf("saving the harness key: %w", err), ln.Close())
	}
	return ln, b, nil
}

func (s *Server) listenHarnessSocket() (net.Listener, harnessBridge, error) {
	dir := s.dir.HarnessSocketDir()
	if err := fsutil.MkdirPrivate(filepath.Dir(dir)); err != nil {
		return nil, harnessBridge{}, err
	}
	if err := fsutil.MkdirPrivate(dir); err != nil {
		return nil, harnessBridge{}, err
	}
	sock := filepath.Join(dir, harnessSocketName)
	if err := removeIfExists(sock); err != nil {
		return nil, harnessBridge{}, err
	}
	ln, err := net.Listen("unix", sock)
	if err != nil {
		return nil, harnessBridge{}, fmt.Errorf("listening for the harness on %s: %w", sock, err)
	}
	return ln, harnessBridge{socket: sock}, nil
}

// observedExecutor publishes every event of the agent to the server's event
// bus and relays questions which the task's caller can't answer.
type observedExecutor struct {
	inner     a2asrv.AgentExecutor
	bus       *eventBus
	peers     *Peers
	approvals *approvals.Service
}

var _ a2asrv.AgentExecutor = (*observedExecutor)(nil)

func (e *observedExecutor) Execute(ctx context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	withLine := e.peers.withAgentsLine(ctx, execCtx)
	events := e.inner.Execute(ctx, withLine)
	if withLine == execCtx {
		return e.observe(ctx, execCtx, events)
	}
	return e.observe(ctx, execCtx, func(yield func(a2a.Event, error) bool) {
		for ev, err := range events {
			if t, ok := ev.(*a2a.Task); ok {
				ev = withOriginalMessage(t, withLine.Message, execCtx.Message)
			}
			if !yield(ev, err) {
				return
			}
		}
	})
}

// withOriginalMessage replaces the message with the agents line in a task's
// history by the caller's message, so the line stays out of the task store.
func withOriginalMessage(t *a2a.Task, withLine, original *a2a.Message) *a2a.Task {
	if !slices.Contains(t.History, withLine) {
		return t
	}
	c := *t
	c.History = slices.Clone(t.History)
	for i, m := range c.History {
		if m == withLine {
			c.History[i] = original
		}
	}
	return &c
}

func (e *observedExecutor) Cancel(ctx context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return e.observe(ctx, execCtx, e.inner.Cancel(ctx, execCtx))
}

func (e *observedExecutor) observe(ctx context.Context, execCtx *a2asrv.ExecutorContext, events iter.Seq2[a2a.Event, error]) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		for ev, err := range events {
			if err == nil {
				e.bus.publish(execCtx.TaskID, ev)
				if q := approvals.FromEvent(ev); q != nil {
					e.approvals.OnQuestion(ctx, execCtx, q)
				}
			}
			if !yield(ev, err) {
				return
			}
		}
	}
}

// eventBus fans out task events to in-process followers, such as a channel
// conversation waiting for a task someone else answered.
type eventBus struct {
	mu   sync.Mutex
	subs map[a2a.TaskID]map[chan a2a.Event]struct{}
}

func newEventBus() *eventBus { return &eventBus{subs: map[a2a.TaskID]map[chan a2a.Event]struct{}{}} }

func (b *eventBus) subscribe(id a2a.TaskID) (<-chan a2a.Event, func()) {
	ch := make(chan a2a.Event, 64)
	b.mu.Lock()
	if b.subs[id] == nil {
		b.subs[id] = map[chan a2a.Event]struct{}{}
	}
	b.subs[id][ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		delete(b.subs[id], ch)
		if len(b.subs[id]) == 0 {
			delete(b.subs, id)
		}
	}
}

func (b *eventBus) publish(id a2a.TaskID, ev a2a.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs[id] {
		select {
		case ch <- ev:
		default:
			slog.Warn("dropping event for a slow follower", "task", id)
		}
	}
}

// callerInterceptor makes the authenticated principal the A2A user and
// enforces who may answer approvals and how deep agent chains go.
type callerInterceptor struct {
	a2asrv.PassthroughCallInterceptor
	approvals *approvals.Service
}

func (i *callerInterceptor) Before(ctx context.Context, callCtx *a2asrv.CallContext, req *a2asrv.Request) (context.Context, any, error) {
	p, ok := auth.Caller(ctx)
	if !ok {
		return ctx, nil, a2a.ErrUnauthenticated
	}
	if p.Hops > auth.MaxHops {
		return ctx, nil, fmt.Errorf("%w: too many agents in this chain (%d)", a2a.ErrUnauthorized, p.Hops)
	}
	auth.SetCallUser(callCtx, p)
	if msg, ok := req.Payload.(*a2a.SendMessageRequest); ok && msg.Message != nil && msg.Message.TaskID != "" {
		if err := i.approvals.CheckAnswer(ctx, p, msg.Message.TaskID); err != nil {
			return ctx, nil, err
		}
	}
	return ctx, nil, nil
}
