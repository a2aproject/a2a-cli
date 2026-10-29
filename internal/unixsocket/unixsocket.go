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

// Package unixsocket routes HTTP requests through a Unix socket.
package unixsocket

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// Spec is a parsed "<path>" or "<host>=<path>" value.
type Spec struct {
	// Host limits the socket to requests for one host; empty for every host.
	Host string
	Path string
}

// Parse parses "<path>" or "<host>=<path>". The text before the first '='
// is a host only when it contains no '/', so paths containing '=' work.
func Parse(spec string) (Spec, error) {
	if spec == "" {
		return Spec{}, fmt.Errorf("unix socket: empty value")
	}
	host, path, ok := strings.Cut(spec, "=")
	if !ok || host == "" || strings.Contains(host, "/") {
		return Spec{Path: spec}, nil
	}
	if strings.Contains(host, ":") {
		return Spec{}, fmt.Errorf("unix socket %q: host %q must not include a port", spec, host)
	}
	if path == "" {
		return Spec{}, fmt.Errorf("unix socket %q: no path", spec)
	}
	return Spec{Host: strings.ToLower(host), Path: path}, nil
}

// Matches reports whether requests to host go through the socket.
func (s Spec) Matches(host string) bool {
	return s.Host == "" || strings.EqualFold(s.Host, host)
}

// Transport returns an HTTP transport that dials the socket for matching
// hosts and bypasses the proxy for them.
func (s Spec) Transport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	proxy := t.Proxy
	t.Proxy = func(r *http.Request) (*url.URL, error) {
		if s.Matches(r.URL.Hostname()) || proxy == nil {
			return nil, nil
		}
		return proxy(r)
	}
	var d net.Dialer
	t.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if h, _, err := net.SplitHostPort(addr); err == nil && s.Matches(h) {
			return d.DialContext(ctx, "unix", s.Path)
		}
		return d.DialContext(ctx, network, addr)
	}
	return t
}
