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

// Package signin runs the interactive part of an OAuth sign-in: it shows the
// user where to authorize and receives the authorization code on a loopback
// address, using the protocol steps of an [oauth.Manager].
package signin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2aclient/oauth"
	"github.com/a2aproject/a2a-go/v2/log"
)

// CallbackPath is the loopback redirect path, as listed in the a2a-cli Client ID
// Metadata Document.
const CallbackPath = "/oauth/callback"

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

// Options control an interactive sign-in.
type Options struct {
	// Device forces the device authorization grant.
	Device bool
	// Scopes replace the scopes from the requirement and the registration.
	Scopes []string
	// Prompter shows the user what to do.
	Prompter Prompter
	// PollInterval, if set, replaces the interval between device flow polls the
	// authorization server asks for.
	PollInterval time.Duration
}

// Acquire obtains a new grant interactively and stores it with m: with the
// device flow when requested or when the server or card offers no other,
// otherwise with the authorization code flow and a loopback redirect.
func Acquire(ctx context.Context, m *oauth.Manager, req *oauth.Requirement, opts Options) (*oauth.Grant, error) {
	_, choice, err := m.Key(req, opts.Scopes)
	if err != nil {
		return nil, err
	}
	log.Debug(ctx, "authorizing", "resource", req.Resource, "issuer", req.Endpoints.Issuer, "client_id", choice.ClientID, "client_source", choice.Source)
	if opts.Device || req.PreferDevice || req.Endpoints.Authorization == "" {
		return deviceCode(ctx, m, req, opts)
	}
	return authorizationCode(ctx, m, req, opts)
}

type callbackResult struct {
	code string
	err  error
}

// authorizationCode runs the authorization code flow with PKCE and a loopback
// redirect (RFC 8252): it prints the authorization URL, waits for the browser to
// come back to 127.0.0.1, and exchanges the code.
func authorizationCode(ctx context.Context, m *oauth.Manager, req *oauth.Requirement, opts Options) (*oauth.Grant, error) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("starting the loopback listener: %w", err)
	}
	ar, err := m.NewAuthorizationRequest(req, fmt.Sprintf("http://%s%s", lis.Addr(), CallbackPath), opts.Scopes)
	if err != nil {
		_ = lis.Close()
		return nil, err
	}

	results := make(chan callbackResult, 1)
	var once sync.Once
	deliver := func(r callbackResult) { once.Do(func() { results <- r }) }
	mux := http.NewServeMux()
	mux.HandleFunc(CallbackPath, func(w http.ResponseWriter, r *http.Request) {
		code, err := ar.ParseRedirect(r.URL.Query())
		var oe *oauth.Error
		switch {
		case errors.As(err, &oe):
			deliver(callbackResult{err: err})
			http.Error(w, "Authorization was not granted. You can close this tab.", http.StatusForbidden)
		case err != nil:
			// Not the redirect for this request; keep waiting for it.
			http.Error(w, fmt.Sprintf("Authorization failed: %v. You can close this tab.", err), http.StatusBadRequest)
		default:
			deliver(callbackResult{code: code})
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = io.WriteString(w, "Authorized. You can close this tab and return to the terminal.\n")
		}
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(lis) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	opts.Prompter.printf("To authorize, open:\n%s\n", ar.URL)
	opts.Prompter.open(ar.URL)

	var res callbackResult
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("waiting for authorization: %w", ctx.Err())
	case res = <-results:
	}
	if res.err != nil {
		return nil, res.err
	}
	return m.ExchangeCode(ctx, ar, res.code)
}

// deviceCode runs the device authorization grant (RFC 8628): it prints the
// verification URI and user code, then polls for the grant.
func deviceCode(ctx context.Context, m *oauth.Manager, req *oauth.Requirement, opts Options) (*oauth.Grant, error) {
	da, err := m.StartDeviceAuthorization(ctx, req, opts.Scopes)
	if err != nil {
		return nil, err
	}
	opts.Prompter.printf("To authorize, open: %s\nEnter code: %s\n", da.VerificationURI, da.UserCode)
	if da.VerificationURIComplete != "" {
		opts.Prompter.open(da.VerificationURIComplete)
	} else {
		opts.Prompter.open(da.VerificationURI)
	}
	if opts.PollInterval > 0 {
		da.Interval = opts.PollInterval
	}
	return m.PollDeviceAuthorization(ctx, da)
}
