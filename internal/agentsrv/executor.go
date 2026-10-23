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

	"github.com/a2aproject/a2a-cli/internal/agentdir"
	"github.com/a2aproject/a2a-cli/internal/auth"
	"github.com/a2aproject/a2a-cli/internal/fsutil"
	"github.com/a2aproject/a2a-cli/internal/harness"
	"github.com/a2aproject/a2a-cli/internal/store"
)

// Sandbox kinds.
const (
	SandboxDocker = "docker"
	SandboxLocal  = "local"
)

// newExecutor starts the harness as an ACP agent through the configured runner.
func (s *Server) newExecutor(ctx context.Context, harnessLog io.Writer) (a2asrv.AgentExecutor, error) {
	cfg := s.config()
	profile, err := harness.Lookup(cfg.Harness, cfg.Command)
	if err != nil {
		return nil, err
	}
	mounts, err := s.dir.Mounts(cfg)
	if err != nil {
		return nil, err
	}
	// ACP sessions get the workspace as cwd, and the executor starts the
	// agent there, so the workspace must have the same path on both sides.
	if mounts[0].Dst != mounts[0].Src {
		return nil, fmt.Errorf("the first --mount is the workspace and must keep its path (%s), got %s", mounts[0].Src, mounts[0].Dst)
	}
	ws := harness.Workspace{Dir: mounts[0].Src, Home: s.dir.HomeDir()}
	if err := profile.Bootstrap(ws); err != nil {
		return nil, fmt.Errorf("bootstrapping %s: %w", profile.Name(), err)
	}

	env := append([]string{}, cfg.Env...)
	for _, name := range cfg.Secrets {
		v, ok := s.secrets[name]
		if !ok {
			return nil, fmt.Errorf("secret %s is not set; pass --secret %s again with the variable set", name, name)
		}
		env = append(env, name+"="+v)
	}

	var runner harness.Runner
	switch cfg.Sandbox {
	case "", SandboxDocker:
		rootless, err := harness.CheckDocker(ctx)
		if err != nil {
			return nil, err
		}
		image := cfg.Image
		if image == "" {
			image = profile.Image()
		}
		image, what, err := harness.EnsureImage(ctx, image, profile.Name(), harnessLog)
		if err != nil {
			return nil, err
		}
		s.logf("✓ docker: %s", what)
		dr := &harness.DockerRunner{
			Name:     "a2a-" + cfg.Name + "-" + s.dir.ID(),
			Image:    image,
			Mounts:   mounts,
			Home:     s.dir.HomeDir(),
			Env:      env,
			Limits:   harness.Limits{Memory: cfg.Limits.Memory, CPUs: cfg.Limits.CPUs, PIDs: cfg.Limits.PIDs},
			Labels:   map[string]string{"a2a.agent": cfg.Name, "a2a.dir": s.dir.Root},
			Rootless: rootless,
		}
		if s.harnessKey != "" {
			if err := saveHarnessKey(s.dir.HomeDir(), s.harnessURL, s.harnessKey); err != nil {
				return nil, fmt.Errorf("saving the harness key: %w", err)
			}
		}
		if s.harnessSocket != "" {
			dr.Mounts = append(slices.Clone(mounts), agentdir.Mount{Src: filepath.Dir(s.harnessSocket), Dst: harness.SandboxDir})
			dr.Env = append(dr.Env, "A2ACLI_UNIX_SOCKET="+harnessHost+"="+harness.SandboxSocket)
			if exe, err := os.Executable(); err == nil {
				dr.Mounts = append(dr.Mounts, agentdir.Mount{Src: exe, Dst: harness.SandboxCLI, ReadOnly: true})
			} else {
				s.logf("! the a2a CLI isn't available to the harness: %v", err)
			}
		}
		s.closers = append(s.closers, dr.Stop)
		runner = dr
	case SandboxLocal:
		runner = harness.LocalRunner{}
		exe, err := os.Executable()
		if err != nil {
			s.logf("! the a2a CLI isn't on the harness's PATH: %v", err)
		} else {
			env = append(env, "PATH="+filepath.Dir(exe)+string(os.PathListSeparator)+os.Getenv("PATH"))
		}
		env = append(env, "A2ACLI_UNIX_SOCKET="+harnessHost+"="+s.harnessSocket)
		for _, kv := range env {
			profile = withEnv(profile, kv)
		}
	default:
		return nil, fmt.Errorf("unknown sandbox %q: want docker or local", cfg.Sandbox)
	}
	mountDesc := make([]string, len(mounts))
	for i, m := range mounts {
		mountDesc[i] = m.String()
	}
	s.logf("✓ workspace %s; config %s", strings.Join(mountDesc, ", "), s.dir.ConfigPath())

	cmd, err := runner.Command(ctx, profile.Command(ws))
	if err != nil {
		return nil, err
	}
	exec, err := a2aacp.NewExecutor(ctx, a2aacp.Config{
		Command:    cmd.Args,
		Dir:        ws.Dir,
		Env:        cmd.Env,
		Stderr:     harnessLog,
		AllowTools: cfg.Approvals.Allow,
		DenyTools:  cfg.Approvals.Deny,
		Sessions:   s.store.Sessions(),
	})
	if err != nil {
		return nil, fmt.Errorf("starting %s: %w", profile.Name(), err)
	}
	s.closers = append(s.closers, func(context.Context) error { return exec.Close() })
	s.logf("✓ %s is running (ACP)", profile.Name())
	return exec, nil
}

// withEnv adds an environment variable to a profile's command.
func withEnv(p harness.Profile, kv string) harness.Profile { return envProfile{Profile: p, kv: kv} }

type envProfile struct {
	harness.Profile
	kv string
}

func (p envProfile) Command(ws harness.Workspace) harness.Command {
	c := p.Profile.Command(ws)
	c.Env = append(c.Env, p.kv)
	return c
}

// listenHarness opens the listener through which the harness calls other
// agents: a Unix socket, which is mounted into a container, or TCP with a key
// for Docker Desktop, which can't mount it.
func (s *Server) listenHarness(ctx context.Context, sandboxed bool) (net.Listener, error) {
	if !sandboxed || s.config().Sandbox == SandboxLocal {
		return s.listenHarnessSocket()
	}
	bind, dial, err := (&harness.DockerRunner{}).HostAddr(ctx)
	if errors.Is(err, harness.ErrNoHostNetwork) {
		return s.listenHarnessSocket()
	}
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(bind, "0"))
	if err != nil {
		return nil, fmt.Errorf("listening for the harness on %s: %w", bind, err)
	}
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		return nil, errors.Join(err, ln.Close())
	}
	s.urlMu.Lock()
	s.harnessURL = "http://" + net.JoinHostPort(dial, port)
	s.harnessKey = store.RandomString(40, store.Alphanumeric)
	s.urlMu.Unlock()
	return ln, nil
}

func (s *Server) listenHarnessSocket() (net.Listener, error) {
	dir := s.dir.HarnessSocketDir()
	if err := fsutil.MkdirPrivate(filepath.Dir(dir)); err != nil {
		return nil, err
	}
	if err := fsutil.MkdirPrivate(dir); err != nil {
		return nil, err
	}
	sock := filepath.Join(dir, filepath.Base(harness.SandboxSocket))
	if err := removeIfExists(sock); err != nil {
		return nil, err
	}
	ln, err := net.Listen("unix", sock)
	if err != nil {
		return nil, fmt.Errorf("listening for the harness on %s: %w", sock, err)
	}
	s.urlMu.Lock()
	s.harnessSocket = sock
	s.urlMu.Unlock()
	return ln, nil
}

// observedExecutor publishes every event of the agent to the server's event
// bus and relays questions which the task's caller can't answer.
type observedExecutor struct {
	inner a2asrv.AgentExecutor
	s     *Server
}

var _ a2asrv.AgentExecutor = (*observedExecutor)(nil)

func (e *observedExecutor) Execute(ctx context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	withLine := e.s.withAgentsLine(ctx, execCtx)
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
				e.s.bus.publish(execCtx.TaskID, ev)
				if q := questionOf(ev); q != nil {
					e.s.onQuestion(ctx, execCtx, q)
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
	s *Server
}

func (i *callerInterceptor) Before(ctx context.Context, callCtx *a2asrv.CallContext, req *a2asrv.Request) (context.Context, any, error) {
	p, ok := principalOf(ctx)
	if !ok {
		return ctx, nil, a2a.ErrUnauthenticated
	}
	if p.Hops > auth.MaxHops {
		return ctx, nil, fmt.Errorf("%w: too many agents in this chain (%d)", a2a.ErrUnauthorized, p.Hops)
	}
	callCtx.User = a2asrv.NewAuthenticatedUser(p.ID, map[string]any{"principal": p})
	if msg, ok := req.Payload.(*a2a.SendMessageRequest); ok && msg.Message != nil && msg.Message.TaskID != "" {
		if err := i.s.checkAnswer(ctx, p, msg.Message.TaskID); err != nil {
			return ctx, nil, err
		}
	}
	return ctx, nil, nil
}

// checkAnswer rejects a reply to a pending permission question from a
// caller who may not approve.
func (s *Server) checkAnswer(ctx context.Context, p auth.Principal, taskID a2a.TaskID) error {
	stored, err := s.tasks.Get(auth.WithPrincipal(ctx, auth.Owner()), taskID)
	if errors.Is(err, a2a.ErrTaskNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("checking task %s: %w", taskID, err)
	}
	task := stored.Task
	if task.Status.State != a2a.TaskStateInputRequired {
		return nil
	}
	q := questionOfStatus(task.Status)
	if q == nil || q.Type != questionPermission {
		return nil
	}
	if s.canApprove(ctx, p) {
		return nil
	}
	return fmt.Errorf("%w: only the owner or an approver can answer this approval; it was sent to them", a2a.ErrUnauthorized)
}
