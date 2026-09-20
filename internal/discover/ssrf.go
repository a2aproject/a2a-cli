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
	"fmt"
	"net"
	"strings"
)

// Guard decides whether a host may be probed for an Agent Card. Because probing
// auto-fetches a URL that merely appeared in the model's context, an unguarded
// probe is an SSRF primitive (cloud metadata endpoints, loopback, RFC1918). The
// zero Guard therefore denies any host that resolves to a private, loopback,
// link-local, or unspecified address.
type Guard struct {
	// AllowPrivate disables the private/loopback/link-local address checks. It
	// exists for local development and demos (e.g. probing localhost); leave it
	// off for any context that handles untrusted URLs.
	AllowPrivate bool
	// Allowlist is a set of exact hostnames that are always admitted, regardless
	// of the address they resolve to.
	Allowlist []string
	// lookupIP resolves a hostname to IPs; nil uses net.LookupIP. Injectable for tests.
	lookupIP func(host string) ([]net.IP, error)
}

// Allow reports whether host may be probed, and if not, a human-readable reason.
func (g Guard) Allow(host string) (bool, string) {
	if host == "" {
		return false, "empty host"
	}
	for _, allowed := range g.Allowlist {
		if strings.EqualFold(host, allowed) {
			return true, ""
		}
	}
	if g.AllowPrivate {
		return true, ""
	}

	for _, ip := range g.resolve(host) {
		if blocked, reason := blockedIP(ip); blocked {
			return false, fmt.Sprintf("host %s resolves to %s (%s); pass --allow-private or an allowlist to probe it", host, ip, reason)
		}
	}
	if len(g.resolve(host)) == 0 {
		return false, fmt.Sprintf("host %s did not resolve", host)
	}
	return true, ""
}

func (g Guard) resolve(host string) []net.IP {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}
	}
	if strings.EqualFold(host, "localhost") {
		return []net.IP{net.IPv4(127, 0, 0, 1)}
	}
	lookup := g.lookupIP
	if lookup == nil {
		lookup = net.LookupIP
	}
	ips, err := lookup(host)
	if err != nil {
		return nil
	}
	return ips
}

// blockedIP reports whether an IP is in a range that must not be auto-probed.
func blockedIP(ip net.IP) (bool, string) {
	switch {
	case ip.IsLoopback():
		return true, "loopback"
	case ip.IsPrivate():
		return true, "private"
	case ip.IsLinkLocalUnicast(), ip.IsLinkLocalMulticast():
		return true, "link-local"
	case ip.IsUnspecified():
		return true, "unspecified"
	}
	return false, ""
}
