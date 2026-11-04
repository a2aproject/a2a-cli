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

package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// CallbackPath is the loopback redirect path, as listed in the a2a-cli Client ID
// Metadata Document.
const CallbackPath = "/oauth/callback"

const defaultDeviceInterval = 5 * time.Second

// Prompter shows the user what to do to authorize.
type Prompter struct {
	// Out receives the instructions, normally stderr.
	Out io.Writer
	// Open, if set, is called with the URL the user has to visit.
	Open func(url string)
}

func (p Prompter) printf(format string, args ...any) {
	if p.Out != nil {
		_, _ = fmt.Fprintf(p.Out, format, args...)
	}
}

func (p Prompter) open(u string) {
	if p.Open != nil {
		p.Open(u)
	}
}

func randomString(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// pkceChallenge returns the S256 challenge for a verifier (RFC 7636 §4.2).
func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

type callbackResult struct {
	code string
	err  error
}

// authorizationCode runs the authorization code flow with PKCE and a loopback
// redirect (RFC 7636, RFC 8252): it prints the authorization URL, waits for the
// browser to come back to 127.0.0.1, and exchanges the code.
func (f fetcher) authorizationCode(ctx context.Context, e Endpoints, client ClientChoice, resource string, scopes []string, p Prompter) (*tokenResponse, error) {
	if e.Authorization == "" {
		return nil, fmt.Errorf("authorization server %s has no authorization endpoint; use --device", e.Issuer)
	}
	authURL, err := url.Parse(e.Authorization)
	if err != nil {
		return nil, fmt.Errorf("invalid authorization endpoint: %w", err)
	}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("starting the loopback listener: %w", err)
	}
	redirectURI := fmt.Sprintf("http://%s%s", lis.Addr().String(), CallbackPath)
	state := randomString(32)
	verifier := randomString(48)

	results := make(chan callbackResult, 1)
	var once sync.Once
	deliver := func(r callbackResult) { once.Do(func() { results <- r }) }
	mux := http.NewServeMux()
	mux.HandleFunc(CallbackPath, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(state)) != 1 {
			http.Error(w, "Authorization failed: unexpected state. You can close this tab.", http.StatusBadRequest)
			return
		}
		if code := q.Get("error"); code != "" {
			deliver(callbackResult{err: &Error{Code: code, Description: q.Get("error_description")}})
			http.Error(w, "Authorization was not granted. You can close this tab.", http.StatusForbidden)
			return
		}
		code := q.Get("code")
		if code == "" {
			http.Error(w, "Authorization failed: no code. You can close this tab.", http.StatusBadRequest)
			return
		}
		deliver(callbackResult{code: code})
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "Authorized. You can close this tab and return to the terminal.\n")
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(lis) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	q := authURL.Query()
	q.Set("response_type", "code")
	q.Set("client_id", client.ClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("state", state)
	q.Set("code_challenge", pkceChallenge(verifier))
	q.Set("code_challenge_method", "S256")
	if len(scopes) > 0 {
		q.Set("scope", strings.Join(scopes, " "))
	}
	if resource != "" {
		q.Set("resource", resource)
	}
	authURL.RawQuery = q.Encode()

	p.printf("To authorize, open:\n%s\n", authURL)
	p.open(authURL.String())

	var res callbackResult
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("waiting for authorization: %w", ctx.Err())
	case res = <-results:
	}
	if res.err != nil {
		return nil, res.err
	}

	form := tokenForm(grantTypeAuthorizationCode, client.ClientID, resource)
	form.Set("code", res.code)
	form.Set("redirect_uri", redirectURI)
	form.Set("code_verifier", verifier)
	return f.token(ctx, e.Token, form)
}

type deviceAuthorization struct {
	DeviceCode              string  `json:"device_code"`
	UserCode                string  `json:"user_code"`
	VerificationURI         string  `json:"verification_uri"`
	VerificationURL         string  `json:"verification_url"`
	VerificationURIComplete string  `json:"verification_uri_complete"`
	ExpiresIn               flexInt `json:"expires_in"`
	Interval                flexInt `json:"interval"`
}

// sleepFunc waits for d or until ctx is done.
type sleepFunc func(ctx context.Context, d time.Duration) error

func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// deviceCode runs the device authorization grant (RFC 8628): it prints the
// verification URI and user code, then polls the token endpoint.
func (f fetcher) deviceCode(ctx context.Context, e Endpoints, client ClientChoice, resource string, scopes []string, p Prompter, sleep sleepFunc) (*tokenResponse, error) {
	if e.DeviceAuthorization == "" {
		return nil, fmt.Errorf("authorization server %s does not support the device flow", e.Issuer)
	}
	form := url.Values{"client_id": {client.ClientID}}
	if len(scopes) > 0 {
		form.Set("scope", strings.Join(scopes, " "))
	}
	if resource != "" {
		form.Set("resource", resource)
	}
	var da deviceAuthorization
	if err := f.postForm(ctx, e.DeviceAuthorization, form, &da); err != nil {
		return nil, fmt.Errorf("starting the device flow: %w", err)
	}
	verification := da.VerificationURI
	if verification == "" {
		verification = da.VerificationURL
	}
	if da.DeviceCode == "" || da.UserCode == "" || verification == "" {
		return nil, errors.New("device authorization response is missing device_code, user_code or verification_uri")
	}
	p.printf("To authorize, open: %s\nEnter code: %s\n", verification, da.UserCode)
	if da.VerificationURIComplete != "" {
		p.open(da.VerificationURIComplete)
	} else {
		p.open(verification)
	}

	interval := defaultDeviceInterval
	if da.Interval > 0 {
		interval = time.Duration(da.Interval) * time.Second
	}
	if da.ExpiresIn > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(da.ExpiresIn)*time.Second)
		defer cancel()
	}
	for {
		if err := sleep(ctx, interval); err != nil {
			return nil, fmt.Errorf("waiting for device authorization: %w", err)
		}
		poll := tokenForm(grantTypeDeviceCode, client.ClientID, resource)
		poll.Set("device_code", da.DeviceCode)
		t, err := f.token(ctx, e.Token, poll)
		switch {
		case err == nil:
			return t, nil
		case isOAuthError(err, "authorization_pending"):
			continue
		case isOAuthError(err, "slow_down"):
			interval += 5 * time.Second
			continue
		default:
			return nil, err
		}
	}
}

// refresh exchanges a refresh token for a new access token.
func (f fetcher) refresh(ctx context.Context, g *Grant) (*tokenResponse, error) {
	form := tokenForm(grantTypeRefreshToken, g.Key.ClientID, g.Key.Resource)
	form.Set("refresh_token", g.RefreshToken)
	if len(g.Key.Scopes) > 0 {
		form.Set("scope", strings.Join(g.Key.Scopes, " "))
	}
	return f.token(ctx, g.Endpoints.Token, form)
}

// revoke revokes the grant's refresh token, or its access token when it has none
// (RFC 7009).
func (f fetcher) revoke(ctx context.Context, g *Grant) error {
	if g.Endpoints.Revocation == "" {
		return errRevocationUnsupported
	}
	token, hint := g.RefreshToken, "refresh_token"
	if token == "" {
		token, hint = g.AccessToken, "access_token"
	}
	form := url.Values{"token": {token}, "token_type_hint": {hint}, "client_id": {g.Key.ClientID}}
	return f.postForm(ctx, g.Endpoints.Revocation, form, nil)
}

var errRevocationUnsupported = errors.New("the authorization server has no revocation endpoint")
