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
	"regexp"
	"slices"
	"sync"
	"time"

	"github.com/Agent-Card/ai-catalog-go/catalog"
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
	"github.com/a2aproject/a2a-go/v2/a2acompat/a2av0"

	"github.com/a2aproject/a2a-cli/internal/aicatalog"
	"github.com/a2aproject/a2a-cli/internal/cardcache"
)

const (
	defaultProbeTimeout = 5 * time.Second
	maxCardBytes        = 512 << 10
	maxRedirects        = 3
)

// Prober fetches the well-known agent card and AI Catalog of origins and
// records each outcome in the card cache.
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

// CatalogRef returns the cache reference of the well-known AI Catalog of origin.
func CatalogRef(origin string) string {
	return origin + catalog.WellKnownPath
}

// Probe fetches the cards and catalogs of origins in parallel. Documents whose
// cache entry is fresh, and origins another prober is already fetching, are
// skipped.
func (p *Prober) Probe(ctx context.Context, origins []string) {
	client := p.httpClient()
	resolver := agentcard.NewResolver(client)
	resolver.CardParser = a2av0.NewAgentCardParser()
	fetcher := &aicatalog.Fetcher{Client: client}

	var wg sync.WaitGroup
	for _, origin := range origins {
		wg.Go(func() { p.probeOne(ctx, resolver, fetcher, origin) })
	}
	wg.Wait()
}

func (p *Prober) probeOne(ctx context.Context, resolver *agentcard.Resolver, fetcher *aicatalog.Fetcher, origin string) {
	catalogRef := CatalogRef(origin)
	needCard, needCatalog := p.stale(origin), p.stale(catalogRef)
	if !needCard && !needCatalog {
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

	ctx, cancel := context.WithTimeout(ctx, p.timeout())
	defer cancel()
	var wg sync.WaitGroup
	if needCard {
		wg.Go(func() { p.put(p.probeCard(ctx, resolver, origin)) })
	}
	if needCatalog {
		wg.Go(func() { p.put(p.probeCatalog(ctx, fetcher, catalogRef)) })
	}
	wg.Wait()
}

func (p *Prober) put(e *cardcache.Entry) {
	if e == nil {
		return
	}
	if err := p.Cards.Put(*e); err != nil {
		p.logf("caching probe result for %s: %v", e.Ref, err)
	}
}

func (p *Prober) stale(ref string) bool {
	e, ok := p.Cards.Get(ref)
	return !ok || !e.Fresh(p.now(), ttl(e))
}

// probeCard fetches the agent card of origin and returns the entry to cache,
// or nil when the outcome is not worth caching.
func (p *Prober) probeCard(ctx context.Context, resolver *agentcard.Resolver, origin string) *cardcache.Entry {
	card, err := resolver.Resolve(withRedirectChain(ctx), origin)
	if err == nil {
		err = validateCard(card)
	}
	entry := &cardcache.Entry{Ref: origin, FetchedAt: p.now()}
	if err != nil {
		if !p.cacheable(ctx, origin, err) {
			return nil
		}
		p.logf("no agent card at %s: %v", origin, err)
		entry.Error = err.Error()
		return entry
	}
	p.logf("discovered agent %q at %s", card.Name, origin)
	entry.Card = card
	return entry
}

// probeCatalog fetches the AI Catalog at ref and returns the entry to cache,
// or nil when the outcome is not worth caching. Nested catalogs are not
// followed: a probe makes one request per document.
func (p *Prober) probeCatalog(ctx context.Context, fetcher *aicatalog.Fetcher, ref string) *cardcache.Entry {
	data, err := fetcher.Fetch(withRedirectChain(ctx), ref)
	var agents []cardcache.CatalogAgent
	if err == nil {
		agents, err = catalogAgents(ref, data)
	}
	entry := &cardcache.Entry{Ref: ref, FetchedAt: p.now()}
	if err != nil {
		if !p.cacheable(ctx, ref, err) {
			return nil
		}
		p.logf("no AI Catalog agents at %s: %v", ref, err)
		entry.Error = err.Error()
		return entry
	}
	p.logf("discovered %d AI Catalog agents at %s", len(agents), ref)
	entry.Agents = agents
	return entry
}

// cacheable reports whether a failed fetch of ref should be cached as a
// negative result.
func (p *Prober) cacheable(ctx context.Context, ref string, err error) bool {
	if ctx.Err() != nil && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return false
	}
	// A refusal depends on the guard configuration rather than the origin,
	// so it isn't cached and an allow-list change applies at once.
	if _, ok := errors.AsType[*BlockedError](err); ok {
		p.logf("not probing %s: %v", ref, err)
		return false
	}
	// Only a response says there is no document; a connection failure,
	// timeout or server error may be transient, so the origin is probed
	// again on its next mention.
	if isTransient(err) {
		p.logf("probing %s failed: %v", ref, err)
		return false
	}
	return true
}

// errNoCatalogAgents marks an AI Catalog that lists no usable A2A agent.
var errNoCatalogAgents = errors.New("AI Catalog lists no A2A agents")

// maxIdentifierLen bounds the identifiers of catalog agents put in the context.
const maxIdentifierLen = 200

// identifierPattern admits the characters of URNs and URLs that are safe to
// show unquoted in a suggested command line, starting with a letter or digit
// so an identifier can't pose as a flag or a home directory.
var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._~:/@+%=-]*$`)

func catalogAgents(ref string, data []byte) ([]cardcache.CatalogAgent, error) {
	if !aicatalog.IsCatalogDocument(data) {
		return nil, errors.New("not an AI Catalog")
	}
	entries, err := aicatalog.Parse(ref, data)
	if err != nil {
		return nil, err
	}
	var agents []cardcache.CatalogAgent
	for _, e := range aicatalog.Agents(entries) {
		if len(e.Identifier) > maxIdentifierLen || !identifierPattern.MatchString(e.Identifier) {
			continue
		}
		if slices.ContainsFunc(agents, func(a cardcache.CatalogAgent) bool { return a.Identifier == e.Identifier }) {
			continue
		}
		agents = append(agents, cardcache.CatalogAgent{
			Identifier:  e.Identifier,
			DisplayName: e.DisplayName,
			Description: e.Description,
		})
	}
	if len(agents) == 0 {
		return nil, errNoCatalogAgents
	}
	return agents, nil
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
	if status, ok := errors.AsType[*agentcard.ErrStatusNotOK](err); ok {
		return transientStatus(status.StatusCode)
	}
	if status, ok := errors.AsType[*aicatalog.StatusError](err); ok {
		return transientStatus(status.StatusCode)
	}
	return false
}

func transientStatus(code int) bool {
	return code >= http.StatusInternalServerError || code == http.StatusTooManyRequests
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
// an unbounded document. It also marks read failures, which the card resolver
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

// errCardTooLarge reports a card or catalog body over maxCardBytes.
var errCardTooLarge = fmt.Errorf("document is larger than %d bytes", maxCardBytes)

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
