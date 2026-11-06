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
	"iter"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-cli/internal/credhelper"
	"github.com/a2aproject/a2a-cli/internal/oauthstore"
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
	"github.com/a2aproject/a2a-go/v2/a2aclient/oauth"
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

// initOAuth checks --oauth and --oauth-store and sets up the OAuth manager when
// OAuth is on. With OAuth off the manager is built only when an a2a auth
// command needs it, so other commands don't need a home directory or a working
// grant store.
func (g *globalConfig) initOAuth(d deps) error {
	switch g.oauthMode {
	case oauthAuto, oauthCached, oauthOff:
	default:
		return clierr.Usage(fmt.Sprintf("invalid --oauth %q (want auto, cached or off)", g.oauthMode))
	}
	switch g.oauthStore {
	case "", "file", "keychain":
	default:
		return clierr.Usage(fmt.Sprintf("invalid --oauth-store %q (want file or keychain)", g.oauthStore))
	}
	g.buildOAuth = func() error {
		store, err := g.newOAuthStore(d)
		if err != nil {
			return err
		}
		g.oauth = &oauth.Manager{
			Clients:         g.oauthClients.Registrations(),
			DefaultClientID: g.oauthDefaultClientID,
			Store:           store,
			HTTPClient:      &http.Client{Transport: baseTransport{g}},
		}
		// The grant store's file lock serializes refreshes with other a2a
		// processes.
		if l, ok := store.(oauth.Locker); ok {
			g.oauth.Locker = l
		}
		return nil
	}
	if g.oauthMode == oauthOff {
		return nil
	}
	return g.requireOAuth()
}

// requireOAuth builds the OAuth manager if it hasn't been built yet.
func (g *globalConfig) requireOAuth() error {
	if g.oauth != nil {
		return nil
	}
	if g.buildOAuth == nil {
		return errors.New("OAuth is not set up")
	}
	return g.buildOAuth()
}

// oauthEnabled reports whether agent calls use OAuth. It is off by default, so
// agents that take other credentials work as they did before OAuth support.
func (g *globalConfig) oauthEnabled() bool {
	return g.oauth != nil && g.oauthMode != oauthOff
}

// agentTransport is the transport for requests to agents: the HTTP transport,
// wrapped to record Bearer challenges while OAuth is off.
func (g *globalConfig) agentTransport() http.RoundTripper {
	if !g.oauthEnabled() {
		return challengeRecorder{g}
	}
	return baseTransport{g}
}

// challengeRecorder sends requests like baseTransport and records whether the
// latest response was a 401 with a Bearer challenge to a request without an
// Authorization header, so that with OAuth off oauthOffHint can point to it.
// A 401 for the user's own credential is not OAuth's to fix. The SDK's
// transport errors don't carry the response's headers.
type challengeRecorder struct{ g *globalConfig }

func (t challengeRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := baseTransport(t).RoundTrip(req)
	if err != nil {
		t.g.bearerChallenged.Store(false)
		return resp, err
	}
	sent := req
	if resp.Request != nil {
		sent = resp.Request
	}
	t.g.bearerChallenged.Store(resp.StatusCode == http.StatusUnauthorized && hasBearerChallenge(resp.Header.Values("WWW-Authenticate")) && sent.Header.Get("Authorization") == "")
	return resp, err
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
		if dir, err = oauthstore.DefaultDir(); err != nil {
			return nil, err
		}
	}
	switch g.oauthStore {
	case "", "file":
		return &oauthstore.FileStore{Dir: dir}, nil
	case "keychain":
		kr := d.keyring
		if kr == nil {
			kr = oauthstore.SystemKeyring{}
		}
		return &oauthstore.KeyringStore{Index: &oauthstore.FileStore{Dir: dir, Name: oauthstore.KeyringIndexName}, Keyring: kr}, nil
	default:
		return nil, clierr.Usage(fmt.Sprintf("invalid --oauth-store %q (want file or keychain)", g.oauthStore))
	}
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
	if !g.oauthEnabled() {
		return g.oauthOffHint(fn())
	}
	before := g.eventsHandled
	err := fn()
	sr, err := g.signInRequired(ctx, err)
	if sr == nil {
		return g.oauthError(err)
	}
	if g.eventsHandled != before {
		return clierr.CredentialsMissing(
			fmt.Sprintf("authorization for %s ended during the stream", sr.Requirement.Resource),
			fmt.Sprintf("run `a2a auth login %s`, then follow the task with `a2a task subscribe <task-id>`", g.agentRef(sr)),
			err)
	}
	if reason := g.signInBlocked(d); reason != "" {
		return clierr.CredentialsMissing(
			fmt.Sprintf("%s, and %s", sr.Error(), reason),
			fmt.Sprintf("run `a2a auth login %s` (add --device on a machine without a browser)", g.agentRef(sr)),
			err)
	}
	if _, err := g.signIn(ctx, d, sr.Requirement, signInOptions{}); err != nil {
		return err
	}
	if sr, err = g.signInRequired(ctx, fn()); sr != nil {
		return clierr.AuthFailed(fmt.Sprintf("still not authorized after signing in: %v", err), err)
	}
	return g.oauthError(err)
}

// signInRequired returns the sign-in err asks for: the
// [*oauth.SignInRequiredError] of an A2A call, or the requirement behind a 401
// for the agent card request, which is not an A2A call, unless that request
// carried an Authorization header of the user's. It returns err, or the error
// finding the card's requirement, unchanged otherwise.
func (g *globalConfig) signInRequired(ctx context.Context, err error) (*oauth.SignInRequiredError, error) {
	var sr *oauth.SignInRequiredError
	if errors.As(err, &sr) {
		return sr, err
	}
	var cardErr *agentcard.ErrStatusNotOK
	if !errors.As(err, &cardErr) || cardErr.StatusCode != http.StatusUnauthorized || g.cardURL == "" || g.oauth == nil || g.cardAuthorization(ctx, g.cardURL) {
		return nil, err
	}
	r, rerr := g.oauth.Discover(ctx, g.cardURL, nil)
	switch {
	case rerr != nil:
		return nil, rerr
	case r == nil:
		return nil, err
	}
	sr = &oauth.SignInRequiredError{Requirement: r, Err: err}
	return sr, sr
}

// cardToken returns the stored OAuth token for the agent card request to
// cardURL, or "" when OAuth is off, the request carries the user's own
// Authorization, or no grant can be used without signing in.
func (g *globalConfig) cardToken(ctx context.Context, cardURL string) string {
	if !g.oauthEnabled() || g.cardAuthorization(ctx, cardURL) {
		return ""
	}
	token, err := g.oauth.Token(ctx, cardURL)
	if err != nil {
		if !errors.Is(err, oauth.ErrGrantNotFound) {
			g.logf("not using a stored OAuth grant for %s: %v", clierr.RedactURL(cardURL), err)
		}
		return ""
	}
	return token
}

// cardAuthorization reports whether the card request to ref carries an
// Authorization header from --auth, --svc-param or a credential helper.
func (g *globalConfig) cardAuthorization(ctx context.Context, ref string) bool {
	if len(g.svcParams.Params().Get("authorization")) > 0 {
		return true
	}
	headers, err := g.cardHeaders(ctx, ref)
	if err != nil {
		return false
	}
	for name := range headers {
		if strings.EqualFold(name, "Authorization") {
			return true
		}
	}
	return false
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

// callbackPath is the loopback redirect path, as listed in the a2a-cli Client ID
// Metadata Document.
const callbackPath = "/oauth/callback"

// signInOptions control an interactive sign-in.
type signInOptions struct {
	// device forces the device authorization grant.
	device bool
	// scopes replace the scopes from the requirement and the registration.
	scopes []string
}

// signIn obtains a grant interactively: with the device flow when requested
// or when the server or card offers no other, otherwise with the authorization
// code flow and a loopback redirect. It is bounded by its own timeout, not
// --timeout, and stops on Ctrl-C.
func (g *globalConfig) signIn(ctx context.Context, d deps, req *oauth.Requirement, opts signInOptions) (*oauth.Grant, error) {
	ctx, stop := d.notifyInterrupt(context.WithoutCancel(ctx))
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, d.authTimeout)
	defer cancel()

	open := func(u string) {
		if d.openURL != nil {
			d.openURL(u)
		}
	}
	so := oauth.SignInOptions{
		Scopes:       opts.scopes,
		RedirectPath: callbackPath,
		ShowCode: func(dc oauth.DeviceCode) {
			_, _ = fmt.Fprintf(g.stderr(), "To authorize, open: %s\nEnter code: %s\n", dc.VerificationURI, dc.UserCode)
			if dc.VerificationURIComplete != "" {
				open(dc.VerificationURIComplete)
			} else {
				open(dc.VerificationURI)
			}
		},
	}
	if !opts.device {
		so.ShowURL = func(u string) {
			_, _ = fmt.Fprintf(g.stderr(), "To authorize, open:\n%s\n", u)
			open(u)
		}
	}
	grant, err := g.oauth.SignIn(ctx, req, so)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, clierr.AuthFailed(fmt.Sprintf("authorization of %s timed out after %s", req.Resource, d.authTimeout), err)
		}
		if errors.Is(err, context.Canceled) && ctx.Err() != nil {
			return nil, clierr.AuthFailed(fmt.Sprintf("authorization of %s was interrupted", req.Resource), err)
		}
		return nil, g.oauthError(fmt.Errorf("authorizing %s at %s: %w", req.Resource, req.Endpoints.Issuer, err))
	}
	if _, err := fmt.Fprintf(g.stderr(), "Authorized. Grant saved for %s.\n", grant.Resource); err != nil {
		return nil, err
	}
	return grant, nil
}

// oauthError classifies authorization failures; other errors pass through.
func (g *globalConfig) oauthError(err error) error {
	if err == nil {
		return nil
	}
	if msg, ok := g.noClientMessage(err); ok {
		return clierr.Usage(msg)
	}
	var ac *oauth.AmbiguousClientError
	if errors.As(err, &ac) {
		return clierr.Usage(fmt.Sprintf("oauth-clients has several entries for the authorization server %s (client IDs %s); keep one", ac.Issuer, strings.Join(ac.ClientIDs, ", ")))
	}
	var sr *oauth.SignInRequiredError
	var fe *oauth.FailureError
	var oe *oauth.Error
	if errors.As(err, &sr) || errors.As(err, &fe) || errors.As(err, &oe) {
		return clierr.AuthFailed(err.Error(), err)
	}
	return err
}

// noClientMessage explains an [oauth.NoClientError] in terms of
// --oauth-clients. For an OAuth flow the agent card declares inline, the error
// names the origin of the flow's token endpoint, which need not be the issuer,
// so the message names the token endpoint instead of suggesting that origin.
func (g *globalConfig) noClientMessage(err error) (string, bool) {
	var nc *oauth.NoClientError
	if errors.As(err, &nc) && len(nc.Issuers) == 1 {
		if token := inlineTokenURL(g.resolvedCard, nc.Issuers[0]); token != "" {
			return fmt.Sprintf(`no client in oauth-clients matches the authorization server whose token endpoint is %s, which the agent card declares; add {"authority":"<the issuer of that server>","client_id":"<client id or CIMD URL>"} to oauth-clients`, token), true
		}
	}
	return oauthstore.NoClientMessage(err)
}

// inlineTokenURL returns the token endpoint of an OAuth flow card declares
// inline, without a metadata URL, whose origin is origin, or "".
func inlineTokenURL(card *a2a.AgentCard, origin string) string {
	if card == nil {
		return ""
	}
	for _, scheme := range card.SecuritySchemes {
		var s a2a.OAuth2SecurityScheme
		switch v := scheme.(type) {
		case a2a.OAuth2SecurityScheme:
			s = v
		case *a2a.OAuth2SecurityScheme:
			if v == nil {
				continue
			}
			s = *v
		default:
			continue
		}
		if s.Oauth2MetadataURL != "" {
			continue
		}
		var token string
		switch f := s.Flows.(type) {
		case a2a.AuthorizationCodeOAuthFlow:
			token = f.TokenURL
		case *a2a.AuthorizationCodeOAuthFlow:
			if f != nil {
				token = f.TokenURL
			}
		case a2a.DeviceCodeOAuthFlow:
			token = f.TokenURL
		case *a2a.DeviceCodeOAuthFlow:
			if f != nil {
				token = f.TokenURL
			}
		}
		if u, err := url.Parse(oauth.NormalizeResource(token)); err == nil && token != "" && u.Scheme+"://"+u.Host == oauth.NormalizeResource(origin) {
			return token
		}
	}
	return ""
}

// oauthOffHint points to OAuth when it is off and the agent answered 401 with a
// Bearer challenge to a request without credentials. The error keeps its code
// and exit status.
func (g *globalConfig) oauthOffHint(err error) error {
	if err == nil || !g.challengedByBearer(err) {
		return err
	}
	ce := *clierr.Classify(err)
	if ce.Hint == "" || ce.Code == clierr.CodeCardInvalid {
		ce.Hint = fmt.Sprintf("the agent asks for an OAuth token: run `a2a auth login %s`, then retry with --oauth auto (or set oauth: auto in ~/.config/a2a-cli/config.yaml)", g.loginRef("<agent>"))
	}
	return &ce
}

// challengedByBearer reports whether err comes from a 401 response with a
// Bearer challenge to a request without credentials, as challengeRecorder saw
// the latest response, for an agent card request too.
func (g *globalConfig) challengedByBearer(err error) bool {
	var cardErr *agentcard.ErrStatusNotOK
	if errors.As(err, &cardErr) && cardErr.StatusCode != http.StatusUnauthorized {
		return false
	}
	return g.bearerChallenged.Load()
}

// hasBearerChallenge reports whether WWW-Authenticate values offer a Bearer
// challenge. A challenge starts with its scheme, a token followed by a space or
// the end of the challenge; the parameters that follow are name=value pairs,
// whose quoted values may contain commas.
func hasBearerChallenge(values []string) bool {
	for _, v := range values {
		for _, part := range splitChallengeList(v) {
			scheme, _, _ := strings.Cut(strings.TrimSpace(part), " ")
			if strings.EqualFold(scheme, "bearer") {
				return true
			}
		}
	}
	return false
}

// splitChallengeList splits a WWW-Authenticate value at the commas outside
// quoted strings.
func splitChallengeList(v string) []string {
	var parts []string
	quoted, escaped, start := false, false, 0
	for i, r := range v {
		switch {
		case escaped:
			escaped = false
		case quoted && r == '\\':
			escaped = true
		case r == '"':
			quoted = !quoted
		case r == ',' && !quoted:
			parts = append(parts, v[start:i])
			start = i + 1
		}
	}
	return append(parts, v[start:])
}

// agentRef is the agent to suggest in `a2a auth login` hints for sr.
func (g *globalConfig) agentRef(sr *oauth.SignInRequiredError) string {
	return g.loginRef(sr.Requirement.Resource)
}

// loginRef is the agent to suggest in `a2a auth login` hints: the card
// reference, or --endpoint with its --transport, since auth login reads a bare
// argument as a card reference, or else fallback.
func (g *globalConfig) loginRef(fallback string) string {
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
		return fallback
	}
}

// stderrIsTerminal reports whether standard error is a terminal.
func stderrIsTerminal() bool {
	return fileIsTerminal(os.Stderr)
}

// fileIsTerminal reports whether f is a terminal. Unlike a character device
// check, it is false for /dev/null.
func fileIsTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// oauthPlugin wraps the OAuth plugin so explicit credentials win: when the user
// supplied credentials of their own, a card that requires OAuth doesn't stop
// the call before it is sent, neither for a missing grant nor because its OAuth
// requirement can't be resolved, and OAuth only reacts to the agent's 401. A
// call that already carries an Authorization header, from --auth, --svc-param
// or a credential helper, is left to that credential by the OAuth plugin
// itself, 401 included.
type oauthPlugin struct {
	a2aclient.PassthroughPlugin
	inner a2aclient.Plugin
	cfg   *globalConfig
}

// Name implements [a2aclient.Plugin].
func (oauthPlugin) Name() string { return "a2a-cli-oauth" }

// WrapCall implements [a2aclient.Plugin].
func (p oauthPlugin) WrapCall(next a2aclient.CallHandler) a2aclient.CallHandler {
	return func(ctx context.Context, req a2aclient.Request) iter.Seq2[any, error] {
		if req.ProtocolBinding == a2a.TransportProtocolGRPC && p.cfg.insecureGRPC && p.cfg.insecureFromDiscoveredFile() {
			// A cloned repository's .env must not be able to make a2a send a
			// stored token in cleartext.
			p.cfg.insecureOAuthWarning.Do(func() {
				_, _ = fmt.Fprintln(p.cfg.stderr(), "warning: not sending OAuth tokens over plaintext gRPC: --insecure comes from "+p.cfg.insecureSource()+"; pass --insecure on the command line to allow it")
			})
			return next(ctx, req)
		}
		return func(yield func(any, error) bool) {
			sent := false
			call := p.inner.WrapCall(func(ctx context.Context, r a2aclient.Request) iter.Seq2[any, error] {
				sent = true
				return next(ctx, r)
			})
			for v, err := range call(ctx, req) {
				if err != nil && !sent && blocksCall(err) && p.cfg.explicitCredentials(req.BaseURL) {
					p.cfg.logf("sending the request to %s without OAuth: other credentials are configured (%v)", req.BaseURL, err)
					p.sendWithout(ctx, req, next, err, yield)
					return
				}
				if !yield(v, err) {
					return
				}
			}
		}
	}
}

// sendWithout sends a call the OAuth plugin refused to send with blocked, with
// the user's other credentials only. When the card requires a sign-in and the
// agent refuses the call with a Bearer challenge, or none, the call fails with
// the sign-in, so the command can sign in and run again.
func (p oauthPlugin) sendWithout(ctx context.Context, req a2aclient.Request, next a2aclient.CallHandler, blocked error, yield func(any, error) bool) {
	var sr *oauth.SignInRequiredError
	first := true
	for v, err := range next(ctx, req) {
		if first && err != nil && errors.As(blocked, &sr) && errors.Is(err, a2a.ErrUnauthenticated) && challengesBearer(err) {
			yield(nil, &oauth.SignInRequiredError{Requirement: sr.Requirement, Err: err})
			return
		}
		first = false
		if !yield(v, err) {
			return
		}
	}
}

// challengesBearer reports whether the response err came from offers a Bearer
// challenge, or no challenge at all.
func challengesBearer(err error) bool {
	var ae *a2a.Error
	if !errors.As(err, &ae) {
		return true
	}
	var values []string
	for k, v := range ae.ServiceParams {
		if strings.EqualFold(k, "WWW-Authenticate") {
			values = append(values, v...)
		}
	}
	return len(values) == 0 || hasBearerChallenge(values)
}

// blocksCall reports whether err is one the OAuth plugin returns instead of
// sending a call: the call needs a sign-in, or its OAuth requirement can't be
// resolved.
func blocksCall(err error) bool {
	var sr *oauth.SignInRequiredError
	var nc *oauth.NoClientError
	var ac *oauth.AmbiguousClientError
	var fe *oauth.FailureError
	return errors.As(err, &sr) || errors.As(err, &nc) || errors.As(err, &ac) || errors.As(err, &fe)
}

// explicitCredentials reports whether the user supplied credentials for calls
// to target other than OAuth: --auth or any --svc-param, a credential helper
// for its host, or --unix-socket, whose proxy may authenticate the calls.
func (g *globalConfig) explicitCredentials(target string) bool {
	if len(g.svcParams.Params()) > 0 || g.unixSocket != "" {
		return true
	}
	if g.credentials == nil {
		return false
	}
	// Helpers are chosen by host, as for the calls themselves; gRPC addresses
	// have no scheme.
	if !strings.Contains(target, "://") {
		target = "https://" + target
	}
	_, ok := credhelper.Select(g.credentials.Helpers, target)
	return ok
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

// insecureFromDiscoveredFile reports whether --insecure was set by a .env found
// by walking up from the working directory, rather than on the command line or
// in the user's own configuration.
func (g *globalConfig) insecureFromDiscoveredFile() bool {
	if g.store == nil {
		return false
	}
	for _, b := range g.bindings {
		if b.Name == "insecure" && b.Source == "flag" {
			return false
		}
	}
	_, source, ok := g.store.LookupFlag("insecure", "A2ACLI_INSECURE")
	return ok && source.Discovered
}

// insecureSource names the file --insecure came from.
func (g *globalConfig) insecureSource() string {
	if g.store != nil {
		if _, source, ok := g.store.LookupFlag("insecure", "A2ACLI_INSECURE"); ok && source.Path != "" {
			return source.Path
		}
	}
	return "a discovered .env"
}
