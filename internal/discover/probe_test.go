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
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/a2aproject/a2a-cli/internal/cardcache"
)

var sortStrings = cmp.Transformer("sort", func(in []string) []string {
	out := slices.Clone(in)
	slices.Sort(out)
	return out
})

func startCardServer(t *testing.T, body string) (string, *atomic.Int32) {
	t.Helper()
	return startCardServerWithStatus(t, http.StatusOK, body)
}

func startCardServerWithStatus(t *testing.T, status int, body string) (string, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path != "/.well-known/agent-card.json" || body == "" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("ResponseWriter.Write() error = %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL, &hits
}

func TestProberProbe(t *testing.T) {
	t.Parallel()

	cardJSON, err := json.Marshal(card("Remote"))
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	tests := []struct {
		name          string
		status        int
		body          string
		wantCached    bool
		wantFound     bool
		wantErrSubstr string
	}{
		{name: "card found", status: http.StatusOK, body: string(cardJSON), wantCached: true, wantFound: true},
		{name: "no card", status: http.StatusOK, wantCached: true, wantErrSubstr: "404"},
		{name: "json that is not a card", status: http.StatusOK, body: `{"hello":"world"}`, wantCached: true, wantErrSubstr: "not an agent card"},
		{name: "card behind auth", status: http.StatusUnauthorized, body: "{}", wantCached: true, wantErrSubstr: "401"},
		{name: "server error not cached", status: http.StatusServiceUnavailable, body: string(cardJSON)},
		{name: "rate limit not cached", status: http.StatusTooManyRequests, body: string(cardJSON)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			origin, _ := startCardServerWithStatus(t, tc.status, tc.body)
			dir := t.TempDir()
			cards := cardcache.New(dir)
			p := &Prober{Cards: cards, LockDir: filepath.Join(dir, "probes"), Timeout: 2 * time.Second}

			p.Probe(t.Context(), []string{origin})

			e, ok := cards.Get(origin)
			if ok != tc.wantCached {
				t.Fatalf("Cache.Get(%s) = %+v, %v; want cached %v", origin, e, ok, tc.wantCached)
			}
			if !ok {
				return
			}
			if e.Found() != tc.wantFound {
				t.Fatalf("Cache.Get(%s).Found() = %v, want %v (error %q)", origin, e.Found(), tc.wantFound, e.Error)
			}
			if tc.wantFound && e.Card.Name != "Remote" {
				t.Fatalf("Cache.Get(%s).Card.Name = %q, want %q", origin, e.Card.Name, "Remote")
			}
			if !strings.Contains(e.Error, tc.wantErrSubstr) {
				t.Fatalf("Cache.Get(%s).Error = %q, want containing %q", origin, e.Error, tc.wantErrSubstr)
			}
		})
	}
}

func TestProberSkipsFreshEntries(t *testing.T) {
	t.Parallel()
	origin, hits := startCardServer(t, "")
	dir := t.TempDir()
	cards := cardcache.New(dir)
	if err := cards.Put(cardcache.Entry{Ref: origin, FetchedAt: time.Now(), Error: "404"}); err != nil {
		t.Fatalf("Cache.Put() error = %v", err)
	}
	p := &Prober{Cards: cards, LockDir: filepath.Join(dir, "probes")}

	p.Probe(t.Context(), []string{origin})

	if got := hits.Load(); got != 0 {
		t.Fatalf("Prober.Probe() made %d requests, want 0", got)
	}
}

func TestProberRefusalsNotCached(t *testing.T) {
	t.Parallel()

	cardJSON, err := json.Marshal(card("Internal"))
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	internalURL, internalHits := startCardServer(t, string(cardJSON))
	redirector := httptest.NewServer(http.RedirectHandler(internalURL+"/.well-known/agent-card.json", http.StatusFound))
	t.Cleanup(redirector.Close)
	_, redirectorPort, err := net.SplitHostPort(strings.TrimPrefix(redirector.URL, "http://"))
	if err != nil {
		t.Fatalf("net.SplitHostPort() error = %v", err)
	}

	tests := []struct {
		name   string
		origin string
	}{
		{name: "public origin redirecting to internal", origin: "http://public.example:" + redirectorPort},
		{name: "link-local origin", origin: "http://metadata.example:" + redirectorPort},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			guard, _ := fakeGuard()
			guard.dial = func(ctx context.Context, network, addr string) (net.Conn, error) {
				_, port, err := net.SplitHostPort(addr)
				if err != nil {
					return nil, err
				}
				var d net.Dialer
				return d.DialContext(ctx, network, net.JoinHostPort("127.0.0.1", port))
			}
			dir := t.TempDir()
			cards := cardcache.New(dir)
			p := &Prober{Cards: cards, LockDir: filepath.Join(dir, "probes"), Guard: guard, Timeout: 2 * time.Second}

			p.Probe(t.Context(), []string{tc.origin})

			if e, ok := cards.Get(tc.origin); ok {
				t.Fatalf("Cache.Get(%s) = %+v, want no entry for a refused probe", tc.origin, e)
			}
			if got := internalHits.Load(); got != 0 {
				t.Fatalf("internal server got %d requests, want 0", got)
			}
		})
	}
}

func startProxy(t *testing.T, cardBody string) (*url.URL, *[]string) {
	t.Helper()
	var mu sync.Mutex
	var targets []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		targets = append(targets, r.URL.Host)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(cardBody)); err != nil {
			t.Errorf("ResponseWriter.Write() error = %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	proxyURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("url.Parse(%q) error = %v", srv.URL, err)
	}
	return proxyURL, &targets
}

func TestProberProxy(t *testing.T) {
	t.Parallel()

	cardJSON, err := json.Marshal(card("Proxied"))
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	tests := []struct {
		name        string
		origin      string
		wantFound   bool
		wantTargets []string
	}{
		{name: "public target", origin: "http://public.example", wantFound: true, wantTargets: []string{"public.example"}},
		{name: "unresolvable target assumed public", origin: "http://proxy-only.example", wantFound: true, wantTargets: []string{"proxy-only.example"}},
		{name: "link-local target refused", origin: "http://metadata.example"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			proxyURL, targets := startProxy(t, string(cardJSON))
			guard, dialed := fakeGuard()
			dir := t.TempDir()
			cards := cardcache.New(dir)
			p := &Prober{
				Cards:   cards,
				LockDir: filepath.Join(dir, "probes"),
				Guard:   guard,
				Timeout: 2 * time.Second,
				Proxy:   func(*http.Request) (*url.URL, error) { return proxyURL, nil },
			}

			p.Probe(t.Context(), []string{tc.origin})

			e, ok := cards.Get(tc.origin)
			if ok != tc.wantFound || (ok && !e.Found()) {
				t.Fatalf("Cache.Get(%s) = %+v, %v; want found %v", tc.origin, e, ok, tc.wantFound)
			}
			if diff := cmp.Diff(tc.wantTargets, *targets); diff != "" {
				t.Fatalf("proxy got wrong targets (-want +got) diff = %s", diff)
			}
			if len(*dialed) != 0 {
				t.Fatalf("guard dialed %v, want the proxy dialed directly", *dialed)
			}
		})
	}
}

func TestProberConnectionFailureNotCached(t *testing.T) {
	t.Parallel()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	origin := "http://" + ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("Listener.Close() error = %v", err)
	}
	dir := t.TempDir()
	cards := cardcache.New(dir)
	p := &Prober{Cards: cards, LockDir: filepath.Join(dir, "probes"), Timeout: 2 * time.Second}

	p.Probe(t.Context(), []string{origin})

	if e, ok := cards.Get(origin); ok {
		t.Fatalf("Cache.Get(%s) = %+v, want no entry after a connection failure", origin, e)
	}
}
