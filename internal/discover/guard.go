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
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// sharedAddressSpace is the RFC 6598 carrier-grade NAT range, also used by
// overlay networks such as Tailscale.
var sharedAddressSpace = netip.MustParsePrefix("100.64.0.0/10")

// Guard decides which addresses discovery may connect to. Probing fetches URLs
// that merely appeared in a conversation, so it refuses link-local addresses
// (such as cloud metadata endpoints) and, within one redirect chain, an
// internal address reached after a public one: a public site can't steer the
// prober into the local network. The checks run at dial time on the resolved
// address, so DNS rebinding can't bypass them.
type Guard struct {
	// AllowHosts are hostnames admitted whatever address they resolve to.
	AllowHosts []string

	lookup func(ctx context.Context, host string) ([]netip.Addr, error)
	dial   func(ctx context.Context, network, addr string) (net.Conn, error)
}

// BlockedError reports a connection the guard refused.
type BlockedError struct {
	Host   string
	Addr   netip.Addr
	Reason string
}

func (e *BlockedError) Error() string {
	return fmt.Sprintf("discovery refuses to connect to %s at %s: %s", e.Host, e.Addr, e.Reason)
}

// redirectChain records whether a request and its redirects reached a public address.
type redirectChain struct {
	mu     sync.Mutex
	public bool
}

type redirectChainKey struct{}

// withRedirectChain marks ctx as one request chain for the public-to-internal check.
func withRedirectChain(ctx context.Context) context.Context {
	return context.WithValue(ctx, redirectChainKey{}, &redirectChain{})
}

// DialContext dials addr after checking every address its host resolves to.
func (g Guard) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if g.hostAllowed(host) {
		return g.dialAddr(ctx, network, addr)
	}
	addrs, err := g.resolve(ctx, host)
	if err != nil {
		return nil, err
	}
	if err := g.checkAddrs(ctx, host, addrs); err != nil {
		return nil, err
	}

	var errs []error
	for _, a := range addrs {
		conn, err := g.dialAddr(ctx, network, net.JoinHostPort(a.String(), port))
		if err == nil {
			return conn, nil
		}
		errs = append(errs, err)
	}
	return nil, errors.Join(errs...)
}

// CheckProxied applies the address rules to host before a proxy connects to it.
// The proxy resolves host on its own, so this is best effort: a name that
// doesn't resolve here is assumed public.
func (g Guard) CheckProxied(ctx context.Context, host string) error {
	if g.hostAllowed(host) {
		return nil
	}
	addrs, err := g.resolve(ctx, host)
	if err != nil {
		markPublic(ctx)
		return nil
	}
	return g.checkAddrs(ctx, host, addrs)
}

func (g Guard) checkAddrs(ctx context.Context, host string, addrs []netip.Addr) error {
	internal := false
	for _, a := range addrs {
		if reason := blockedReason(a); reason != "" {
			return &BlockedError{Host: host, Addr: a, Reason: reason}
		}
		if isInternal(a) {
			internal = true
		}
	}
	chain, ok := ctx.Value(redirectChainKey{}).(*redirectChain)
	if !ok {
		return nil
	}
	chain.mu.Lock()
	defer chain.mu.Unlock()
	if internal && chain.public {
		return &BlockedError{Host: host, Addr: addrs[0], Reason: "internal address reached by a redirect from a public one"}
	}
	if !internal {
		chain.public = true
	}
	return nil
}

func markPublic(ctx context.Context) {
	if chain, ok := ctx.Value(redirectChainKey{}).(*redirectChain); ok {
		chain.mu.Lock()
		chain.public = true
		chain.mu.Unlock()
	}
}

func (g Guard) dialAddr(ctx context.Context, network, addr string) (net.Conn, error) {
	if g.dial != nil {
		return g.dial(ctx, network, addr)
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	return dialer.DialContext(ctx, network, addr)
}

func (g Guard) hostAllowed(host string) bool {
	for _, allowed := range g.AllowHosts {
		if strings.EqualFold(host, allowed) {
			return true
		}
	}
	return false
}

func (g Guard) resolve(ctx context.Context, host string) ([]netip.Addr, error) {
	if a, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{a}, nil
	}
	lookup := g.lookup
	if lookup == nil {
		lookup = func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		}
	}
	addrs, err := lookup(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("host %s did not resolve", host)
	}
	return addrs, nil
}

// blockedReason names why discovery never connects to a, or returns "".
func blockedReason(a netip.Addr) string {
	a = a.Unmap()
	switch {
	case a.IsLinkLocalUnicast(), a.IsLinkLocalMulticast():
		return "link-local address"
	case a.IsUnspecified():
		return "unspecified address"
	case a.IsMulticast():
		return "multicast address"
	}
	return ""
}

// isInternal reports whether a is reachable only from a local or private network.
func isInternal(a netip.Addr) bool {
	a = a.Unmap()
	return a.IsLoopback() || a.IsPrivate() || sharedAddressSpace.Contains(a)
}
