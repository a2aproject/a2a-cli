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

package network

import (
	"context"
	"fmt"
	"io"
	stdlog "log"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/hashicorp/mdns"
)

// mdnsService is the DNS-SD service type agent servers advertise on the LAN.
const mdnsService = "_a2a._tcp"

// LAN is a [Provider] for one local network: zero setup, no identity, so
// callers pair. Agents advertise themselves with mDNS for discovery.
type LAN struct {
	// AgentName is the advertised agent name.
	AgentName string
	// Port is the listen port; zero picks one.
	Port int

	mu   sync.Mutex
	mdns *mdns.Server
	port int
}

var _ Provider = (*LAN)(nil)

// Name implements [Provider].
func (l *LAN) Name() string { return "lan" }

// ListenPrivate implements [Provider]. Traffic is plain HTTP, so bearer
// tokens are only as private as the network.
func (l *LAN) ListenPrivate(context.Context) (net.Listener, *url.URL, error) {
	ip, err := lanIP()
	if err != nil {
		return nil, nil, err
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(ip.String(), strconv.Itoa(l.Port)))
	if err != nil {
		return nil, nil, fmt.Errorf("lan: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	l.mu.Lock()
	l.port = port
	l.mu.Unlock()
	if err := l.advertise(ip, port); err != nil {
		slog.Warn("mDNS advertisement failed, peers won't discover this agent", "error", err)
	}
	return ln, &url.URL{Scheme: "http", Host: net.JoinHostPort(ip.String(), strconv.Itoa(port))}, nil
}

// BoundPort returns the port the private listener got.
func (l *LAN) BoundPort() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.port
}

func (l *LAN) advertise(ip net.IP, port int) error {
	svc, err := mdns.NewMDNSService(l.AgentName, mdnsService, "", "", port, []net.IP{ip}, []string{"path=/"})
	if err != nil {
		return err
	}
	srv, err := mdns.NewServer(&mdns.Config{Zone: svc, Logger: stdlog.New(io.Discard, "", 0)})
	if err != nil {
		return err
	}
	l.mu.Lock()
	l.mdns = srv
	l.mu.Unlock()
	return nil
}

// ListenPublic implements [Provider].
func (l *LAN) ListenPublic(context.Context) (net.Listener, *url.URL, error) {
	return nil, nil, ErrUnsupported
}

// WhoIs implements [Provider].
func (l *LAN) WhoIs(context.Context, string) (*Peer, error) { return nil, ErrNoIdentity }

// Peers implements [Provider].
func (l *LAN) Peers(ctx context.Context) ([]PeerInfo, error) {
	entries := make(chan *mdns.ServiceEntry, 32)
	params := mdns.DefaultParams(mdnsService)
	params.Entries = entries
	params.Timeout = 2 * time.Second
	params.DisableIPv6 = true
	params.Logger = stdlog.New(io.Discard, "", 0)
	var candidates []PeerInfo
	done := make(chan struct{})
	go func() {
		defer close(done)
		for e := range entries {
			if e.AddrV4 == nil || (e.Port == l.BoundPort() && isLocalIP(e.AddrV4)) {
				continue
			}
			name := e.Name
			if i := len(mdnsService) + len(".local.") + 1; len(name) > i {
				name = name[:len(name)-i]
			}
			candidates = append(candidates, PeerInfo{
				Name:   name,
				URL:    "http://" + net.JoinHostPort(e.AddrV4.String(), strconv.Itoa(e.Port)),
				Online: true,
			})
		}
	}()
	err := mdns.QueryContext(ctx, params)
	close(entries)
	<-done
	if err != nil {
		return nil, err
	}
	return ProbeAgents(ctx, http.DefaultClient, candidates), nil
}

// HTTPClient implements [Provider].
func (l *LAN) HTTPClient() *http.Client { return http.DefaultClient }

// Close implements [Provider].
func (l *LAN) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.mdns == nil {
		return nil
	}
	return l.mdns.Shutdown()
}

// lanIP returns the address of the interface used for outbound traffic.
func lanIP() (net.IP, error) {
	// UDP "dial" sends nothing; it only selects the route.
	c, err := net.Dial("udp4", "192.0.2.1:9")
	if err != nil {
		return nil, fmt.Errorf("lan: no network: %w", err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			slog.Debug("closing route probe", "error", err)
		}
	}()
	return c.LocalAddr().(*net.UDPAddr).IP, nil
}

func isLocalIP(ip net.IP) bool {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.Equal(ip) {
			return true
		}
	}
	return false
}
