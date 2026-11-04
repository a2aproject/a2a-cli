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
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-cli/internal/oauth"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
)

// OAuth modes accepted by --oauth.
const (
	oauthAuto   = "auto"
	oauthCached = "cached"
	oauthOff    = "off"
)

// annotationAgentCall marks commands that call an agent, so a sign-in the
// agent requires can run and the command can be retried.
const annotationAgentCall = "a2a-cli/agent-call"

// agentCall marks cmd as a command that calls an agent.
func agentCall(cmd *cobra.Command) *cobra.Command {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[annotationAgentCall] = "true"
	return cmd
}

// initOAuth sets up the OAuth manager, unless --oauth is off.
func (g *globalConfig) initOAuth(d deps) error {
	switch g.oauthMode {
	case oauthAuto, oauthCached:
	case oauthOff:
		return nil
	default:
		return clierr.Usage(fmt.Sprintf("invalid --oauth %q (want auto, cached or off)", g.oauthMode))
	}
	store, err := g.newOAuthStore(d)
	if err != nil {
		return err
	}
	g.oauth = &oauth.Manager{
		Clients:         g.oauthClients.Registrations(),
		DefaultClientID: g.oauthDefaultClientID,
		Store:           store,
		HTTPClient:      &http.Client{Transport: baseTransport{g}},
		Logf:            g.logf,
	}
	return nil
}

// agentTransport is the transport for requests to agents: the HTTP transport,
// wrapped to handle 401 responses when OAuth is enabled.
func (g *globalConfig) agentTransport() http.RoundTripper {
	if g.oauth == nil {
		return g.httpTransport
	}
	return g.oauth.Transport(baseTransport{g})
}

// baseTransport sends requests through the HTTP transport as it is when the
// request is made, so --ca-cert, which initTLS applies once a command connects,
// also covers requests to authorization servers and the OAuth retries.
type baseTransport struct{ g *globalConfig }

func (t baseTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.g.httpTransport == nil {
		return http.DefaultTransport.RoundTrip(req)
	}
	return t.g.httpTransport.RoundTrip(req)
}

func (g *globalConfig) newOAuthStore(d deps) (oauth.Store, error) {
	dir := d.oauthDir
	if dir == "" {
		var err error
		if dir, err = oauth.DefaultDir(); err != nil {
			return nil, err
		}
	}
	return &oauth.FileStore{Dir: dir}, nil
}

// wrapSignIn makes every command marked with [agentCall] sign in when the agent
// requires it and then run once more.
func wrapSignIn(cfg *globalConfig, d deps, cmd *cobra.Command) {
	if run := cmd.RunE; run != nil && cmd.Annotations[annotationAgentCall] != "" {
		cmd.RunE = func(c *cobra.Command, args []string) error {
			return cfg.withSignIn(c.Context(), d, func() error { return run(c, args) })
		}
	}
	for _, sub := range cmd.Commands() {
		wrapSignIn(cfg, d, sub)
	}
}

// withSignIn runs fn. When fn fails because the agent requires an authorization
// that has to be obtained interactively, it signs in, if allowed, and runs fn
// once more. fn creates its own clients and --timeout contexts, so the retry
// isn't limited by time spent signing in. A stream that already printed events
// is not restarted.
func (g *globalConfig) withSignIn(ctx context.Context, d deps, fn func() error) error {
	before := g.eventsHandled
	err := fn()
	var ir *oauth.InteractionRequiredError
	if !errors.As(err, &ir) {
		return g.oauthError(err)
	}
	if g.eventsHandled != before {
		return clierr.CredentialsMissing(
			fmt.Sprintf("authorization for %s ended during the stream", ir.Requirement.Resource),
			fmt.Sprintf("run `a2a auth login %s`, then follow the task with `a2a task subscribe <task-id>`", g.agentRef(ir)),
			err)
	}
	if reason := g.signInBlocked(d); reason != "" {
		return clierr.CredentialsMissing(
			fmt.Sprintf("%s, and %s", ir.Error(), reason),
			fmt.Sprintf("run `a2a auth login %s` (add --device on a machine without a browser)", g.agentRef(ir)),
			err)
	}
	if _, err := g.signIn(ctx, d, ir.Requirement, oauth.AcquireOptions{}); err != nil {
		return err
	}
	err = fn()
	if errors.As(err, &ir) {
		return clierr.AuthFailed(fmt.Sprintf("still not authorized after signing in: %v", err), err)
	}
	return g.oauthError(err)
}

// signInBlocked explains why an interactive sign-in can't start, or returns "".
func (g *globalConfig) signInBlocked(d deps) string {
	switch {
	case g.oauthMode == oauthCached:
		return "no usable stored grant was found (--oauth cached)"
	case !d.authInteractive():
		return "signing in needs a terminal"
	default:
		return ""
	}
}

// signIn obtains a grant interactively. It is bounded by its own timeout, not
// --timeout, and stops on Ctrl-C.
func (g *globalConfig) signIn(ctx context.Context, d deps, req *oauth.Requirement, opts oauth.AcquireOptions) (*oauth.Grant, error) {
	ctx, stop := d.notifyInterrupt(context.WithoutCancel(ctx))
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, d.authTimeout)
	defer cancel()

	opts.Prompter = oauth.Prompter{Out: g.stderr(), Open: d.openURL}
	opts.Sleep = d.authSleep
	grant, err := g.oauth.Acquire(ctx, req, opts)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, clierr.AuthFailed(fmt.Sprintf("authorization of %s timed out after %s", req.Resource, d.authTimeout), err)
		}
		return nil, g.oauthError(fmt.Errorf("authorizing %s at %s: %w", req.Resource, req.Endpoints.Issuer, err))
	}
	if _, err := fmt.Fprintf(g.stderr(), "Authorized. Grant saved for %s.\n", grant.Key.Resource); err != nil {
		return nil, err
	}
	return grant, nil
}

// oauthError classifies authorization failures; other errors pass through.
func (g *globalConfig) oauthError(err error) error {
	if err == nil {
		return nil
	}
	var nc *oauth.NoClientError
	if errors.As(err, &nc) {
		return clierr.Usage(nc.Error())
	}
	var ir *oauth.InteractionRequiredError
	var fe *oauth.FailureError
	var oe *oauth.Error
	if errors.As(err, &ir) || errors.As(err, &fe) || errors.As(err, &oe) {
		return clierr.AuthFailed(err.Error(), err)
	}
	return err
}

// agentRef is the agent to suggest in `a2a auth login` hints: the card
// reference, or --endpoint with its --transport, since auth login reads a bare
// argument as a card reference.
func (g *globalConfig) agentRef(ir *oauth.InteractionRequiredError) string {
	switch {
	case g.agentCard.IsSet():
		return g.agentCard.String()
	case g.url != "":
		parts := []string{"-e", g.url}
		for _, t := range g.transports {
			parts = append(parts, "--transport", t)
		}
		return strings.Join(parts, " ")
	default:
		return ir.Requirement.Resource
	}
}

// stderrIsTerminal reports whether standard error is a terminal.
func stderrIsTerminal() bool {
	return isCharDevice(os.Stderr)
}

// insecureTokenWarning warns, like for other credentials, when a call carries an
// Authorization header, such as an OAuth token, while --insecure turns off TLS
// for gRPC.
type insecureTokenWarning struct {
	a2aclient.PassthroughInterceptor
	cfg *globalConfig
}

func (w insecureTokenWarning) Before(ctx context.Context, req *a2aclient.Request) (context.Context, any, error) {
	if len(req.ServiceParams.Get("authorization")) > 0 {
		w.cfg.warnInsecure()
	}
	return ctx, nil, nil
}
