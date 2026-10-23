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

package agentsrv

import (
	"context"
	"fmt"

	"github.com/a2aproject/a2a-cli/internal/agentdir"
	"github.com/a2aproject/a2a-cli/internal/network"
)

// Network provider names.
const (
	ProviderTailscale = "tailscale"
	ProviderLAN       = "lan"
	ProviderNgrok     = "ngrok"
)

// Secrets read by network providers.
const (
	SecretTailscaleAuthKey = "TS_AUTHKEY"
	SecretNgrokAuthToken   = "NGROK_AUTHTOKEN"
)

// setupNetwork creates the private and public providers from the config.
// They start listening in listenNetwork.
func (s *Server) setupNetwork() error {
	cfg := s.config()
	var ts *network.Tailscale
	tailscale := func() *network.Tailscale {
		if ts == nil {
			ts = &network.Tailscale{
				Hostname:   cfg.Name,
				StateDir:   s.dir.TSNetDir(),
				ControlURL: cfg.Network.TailscaleControlURL,
				AuthKey:    s.secrets[SecretTailscaleAuthKey],
				Logf:       s.logf,
			}
		}
		return ts
	}
	switch cfg.Network.Private {
	case "":
	case ProviderTailscale:
		s.private = tailscale()
	case ProviderLAN:
		s.private = &network.LAN{AgentName: cfg.Name, Port: cfg.Network.LANPort}
	default:
		return fmt.Errorf("unknown private network %q: want tailscale or lan", cfg.Network.Private)
	}
	switch cfg.Network.Public {
	case "":
	case ProviderTailscale:
		t := tailscale()
		t.Funnel = true
		s.public = t
	case ProviderNgrok:
		s.public = &network.Ngrok{AuthToken: s.secrets[SecretNgrokAuthToken], URL: cfg.Network.NgrokURL, Description: "a2a server " + cfg.Name}
	default:
		return fmt.Errorf("unknown public network %q: want tailscale or ngrok", cfg.Network.Public)
	}
	if s.private != nil {
		s.closers = append(s.closers, func(context.Context) error { return s.private.Close() })
	}
	if s.public != nil && s.public != s.private {
		s.closers = append(s.closers, func(context.Context) error { return s.public.Close() })
	}
	return nil
}

// listenNetwork opens the private and public listeners.
func (s *Server) listenNetwork(ctx context.Context, errc chan<- error) error {
	if s.private != nil {
		ln, u, err := s.private.ListenPrivate(ctx)
		if err != nil {
			return fmt.Errorf("private network (%s): %w", s.private.Name(), err)
		}
		if lan, ok := s.private.(*network.LAN); ok && s.config().Network.LANPort == 0 {
			if err := s.updateConfig(func(c *agentdir.Config) error { c.Network.LANPort = lan.BoundPort(); return nil }); err != nil {
				return err
			}
		}
		s.urlMu.Lock()
		s.privateURL = u.String()
		s.urlMu.Unlock()
		s.serve(ctx, ln, network.ListenerPrivate, u.String(), errc)
	}
	if s.public != nil {
		ln, u, err := s.public.ListenPublic(ctx)
		if err != nil {
			return fmt.Errorf("public network (%s): %w", s.public.Name(), err)
		}
		s.urlMu.Lock()
		s.publicURL = u.String()
		s.urlMu.Unlock()
		s.serve(ctx, ln, network.ListenerPublic, u.String(), errc)
	}
	return nil
}
