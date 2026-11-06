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
	"io"
	"net/http"
	"path/filepath"
	"sync"
	"time"

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
	Now     func() time.Time
	Logf    func(format string, args ...any)
}

// Probe fetches the cards of origins in parallel. Origins whose cache entry is
// fresh, or that another prober is already fetching, are skipped.
func (p *Prober) Probe(ctx context.Context, origins []string) {
	resolver := agentcard.NewResolver(p.httpClient())
	resolver.CardParser = a2av0.NewAgentCardParser()

	var wg sync.WaitGroup
	for _, origin := range origins {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.probeOne(ctx, resolver, origin)
		}()
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
	if err != nil {
		if ctx.Err() != nil && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return
		}
		// A refusal depends on the guard configuration rather than the origin,
		// so it isn't cached and an allow-list change applies at once.
		if blocked := (*BlockedError)(nil); errors.As(err, &blocked) {
			p.logf("not probing %s: %v", origin, err)
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

func (p *Prober) httpClient() *http.Client {
	transport := &http.Transport{
		// A proxy would make the connection, so the guard couldn't check the target address.
		Proxy:                  nil,
		DialContext:            p.Guard.DialContext,
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
// an unbounded card.
type limitBody struct {
	next http.RoundTripper
	max  int64
}

func (l limitBody) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := l.next.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	resp.Body = &limitedReadCloser{Reader: io.LimitReader(resp.Body, l.max), Closer: resp.Body}
	return resp, nil
}

type limitedReadCloser struct {
	io.Reader
	io.Closer
}
