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

package discover

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
	"github.com/a2aproject/a2a-go/v2/a2acompat/a2av0"

	"github.com/a2aproject/a2a-cli/internal/cardcache"
)

const (
	defaultProbeTimeout = 5 * time.Second
	maxCardBytes        = 512 << 10
	maxRedirects        = 3
)

// Prober fetches the well-known agent card of origins and records each outcome
// in the card cache.
type Prober struct {
	Cards *cardcache.Cache
	// LockDir holds per-origin lock files so concurrent probers don't fetch the same card.
	LockDir string
	Guard   Guard
	// RootCAs, when set, replaces the system trust store.
	RootCAs *x509.CertPool
	// Timeout bounds each card fetch.
	Timeout time.Duration
	// Proxy picks the proxy for a request. Defaults to http.ProxyFromEnvironment.
	Proxy func(*http.Request) (*url.URL, error)
	Now   func() time.Time
	Logf  func(format string, args ...any)

	// proxies holds the addresses of proxies in use; dials to them skip the guard.
	proxies sync.Map
}

// Probe fetches the cards of origins in parallel. Origins whose cache entry is
// fresh, or that another prober is already fetching, are skipped.
func (p *Prober) Probe(ctx context.Context, origins []string) {
	resolver := agentcard.NewResolver(p.httpClient())
	resolver.CardParser = a2av0.NewAgentCardParser()

	var wg sync.WaitGroup
	for _, origin := range origins {
		wg.Go(func() { p.probeOne(ctx, resolver, origin) })
	}
	wg.Wait()
}

func (p *Prober) probeOne(ctx context.Context, resolver *agentcard.Resolver, origin string) {
	if e, ok := p.Cards.Get(origin); ok && e.Fresh(p.now(), ttl(e)) {
		return
	}
	lock, ok := tryLock(filepath.Join(p.LockDir, hashName(origin)+".lock"), 2*p.timeout())
	if !ok {
		p.logf("skipping %s: already being probed", origin)
		return
	}
	defer func() {
		if err := lock.unlock(); err != nil {
			p.logf("releasing probe lock for %s: %v", origin, err)
		}
	}()

	ctx, cancel := context.WithTimeout(withRedirectChain(ctx), p.timeout())
	defer cancel()
	entry := cardcache.Entry{Ref: origin, FetchedAt: p.now()}
	card, err := resolver.Resolve(ctx, origin)
	if err == nil {
		err = validateCard(card)
	}
	if err != nil {
		if ctx.Err() != nil && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return
		}
		// A refusal depends on the guard configuration rather than the origin,
		// so it isn't cached and an allow-list change applies at once.
		if _, ok := errors.AsType[*BlockedError](err); ok {
			p.logf("not probing %s: %v", origin, err)
			return
		}
		// Only a response says there is no card; a connection failure, timeout
		// or server error may be transient, so the origin is probed again on
		// its next mention.
		if isTransient(err) {
			p.logf("probing %s failed: %v", origin, err)
			return
		}
		p.logf("no agent card at %s: %v", origin, err)
		entry.Error = err.Error()
	} else {
		p.logf("discovered agent %q at %s", card.Name, origin)
		entry.Card = card
	}
	if err := p.Cards.Put(entry); err != nil {
		p.logf("caching probe result for %s: %v", origin, err)
	}
}

// errNotAgentCard marks a document at the well-known path that parses as JSON
// but does not describe an agent.
var errNotAgentCard = errors.New("not an agent card: no name or interfaces")

func validateCard(card *a2a.AgentCard) error {
	if card.Name == "" || !slices.ContainsFunc(card.SupportedInterfaces, func(i *a2a.AgentInterface) bool { return i != nil && i.URL != "" }) {
		return errNotAgentCard
	}
	return nil
}

func isTransient(err error) bool {
	if _, ok := errors.AsType[*url.Error](err); ok {
		return true
	}
	if _, ok := errors.AsType[*bodyReadError](err); ok {
		return true
	}
	status, ok := errors.AsType[*agentcard.ErrStatusNotOK](err)
	return ok && (status.StatusCode >= http.StatusInternalServerError || status.StatusCode == http.StatusTooManyRequests)
}

func (p *Prober) httpClient() *http.Client {
	transport := &http.Transport{
		Proxy:       p.proxy,
		DialContext: p.dial,
		// Every request dials, so the guard sees each hop of a redirect chain.
		DisableKeepAlives:      true,
		TLSHandshakeTimeout:    p.timeout(),
		ResponseHeaderTimeout:  p.timeout(),
		MaxResponseHeaderBytes: 64 << 10,
	}
	if p.RootCAs != nil {
		transport.TLSClientConfig = &tls.Config{RootCAs: p.RootCAs}
	}
	return &http.Client{
		Transport: limitBody{next: transport, max: maxCardBytes},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errors.New("too many redirects")
			}
			return nil
		},
	}
}

// proxy returns the proxy for req after checking the target, which the guard
// can't see at dial time when the connection goes to the proxy.
func (p *Prober) proxy(req *http.Request) (*url.URL, error) {
	proxyFor := p.Proxy
	if proxyFor == nil {
		proxyFor = http.ProxyFromEnvironment
	}
	proxyURL, err := proxyFor(req)
	if err != nil || proxyURL == nil {
		return proxyURL, err
	}
	if err := p.Guard.CheckProxied(req.Context(), req.URL.Hostname()); err != nil {
		return nil, err
	}
	p.proxies.Store(proxyAddr(proxyURL), true)
	return proxyURL, nil
}

func (p *Prober) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	if _, ok := p.proxies.Load(addr); ok {
		var d net.Dialer
		return d.DialContext(ctx, network, addr)
	}
	return p.Guard.DialContext(ctx, network, addr)
}

// proxyAddr returns the host:port the transport dials for proxyURL.
func proxyAddr(proxyURL *url.URL) string {
	if proxyURL.Port() != "" {
		return proxyURL.Host
	}
	port := map[string]string{"http": "80", "https": "443", "socks5": "1080", "socks5h": "1080"}[proxyURL.Scheme]
	return net.JoinHostPort(proxyURL.Hostname(), port)
}

func (p *Prober) timeout() time.Duration {
	if p.Timeout > 0 {
		return p.Timeout
	}
	return defaultProbeTimeout
}

func (p *Prober) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

func (p *Prober) logf(format string, args ...any) {
	if p.Logf != nil {
		p.Logf(format, args...)
	}
}

// limitBody caps response bodies so a hostile origin can't make the prober read
// an unbounded card. It also marks read failures, which the card resolver
// doesn't wrap in a *url.Error, so they count as transient.
type limitBody struct {
	next http.RoundTripper
	max  int64
}

func (l limitBody) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := l.next.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	resp.Body = &limitedReadCloser{body: resp.Body, remaining: l.max}
	return resp, nil
}

// errCardTooLarge reports a card body over maxCardBytes.
var errCardTooLarge = fmt.Errorf("card is larger than %d bytes", maxCardBytes)

// bodyReadError is a failure to read a response body, such as a timeout or a
// reset connection.
type bodyReadError struct{ err error }

func (e *bodyReadError) Error() string { return e.err.Error() }

func (e *bodyReadError) Unwrap() error { return e.err }

type limitedReadCloser struct {
	body      io.ReadCloser
	remaining int64
}

func (l *limitedReadCloser) Read(p []byte) (int, error) {
	if l.remaining < 0 {
		return 0, errCardTooLarge
	}
	// Reading one byte past the limit tells an oversized body from one that
	// ends exactly at it.
	if int64(len(p)) > l.remaining+1 {
		p = p[:l.remaining+1]
	}
	n, err := l.body.Read(p)
	l.remaining -= int64(n)
	if l.remaining < 0 {
		return n + int(l.remaining), errCardTooLarge
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return n, &bodyReadError{err: err}
	}
	return n, err
}

func (l *limitedReadCloser) Close() error { return l.body.Close() }
