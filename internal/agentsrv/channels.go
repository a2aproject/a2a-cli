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
	"fmt"

	"github.com/a2aproject/a2a-cli/internal/agentdir"
	"github.com/a2aproject/a2a-cli/internal/channel"
	"github.com/a2aproject/a2a-cli/internal/channel/discord"
	"github.com/a2aproject/a2a-cli/internal/channel/email"
	"github.com/a2aproject/a2a-cli/internal/channel/telegram"
)

// Channel kinds.
const (
	ChannelTelegram = "telegram"
	ChannelDiscord  = "discord"
	ChannelEmail    = "email"
)

// buildChannels creates the channels of the config.
func (s *Server) buildChannels() ([]channel.Channel, error) {
	cfg := s.config()
	var out []channel.Channel
	for kind, set := range map[string]bool{
		ChannelTelegram: cfg.Channels.Telegram != nil,
		ChannelDiscord:  cfg.Channels.Discord != nil,
		ChannelEmail:    cfg.Channels.Email != nil,
	} {
		if !set {
			continue
		}
		ch, err := s.buildChannel(kind)
		if err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	return out, nil
}

func (s *Server) buildChannel(kind string) (channel.Channel, error) {
	cfg := s.config()
	s.cfgMu.Lock()
	secrets := s.secrets
	s.cfgMu.Unlock()
	switch kind {
	case ChannelTelegram:
		if cfg.Channels.Telegram == nil {
			return nil, fmt.Errorf("telegram is not configured")
		}
		tok, err := secrets.Resolve(cfg.Channels.Telegram.Token)
		if err != nil {
			return nil, fmt.Errorf("telegram: %w", err)
		}
		return telegram.New(tok), nil
	case ChannelDiscord:
		if cfg.Channels.Discord == nil {
			return nil, fmt.Errorf("discord is not configured")
		}
		tok, err := secrets.Resolve(cfg.Channels.Discord.Token)
		if err != nil {
			return nil, fmt.Errorf("discord: %w", err)
		}
		return discord.New(tok, s.store), nil
	case ChannelEmail:
		ec := cfg.Channels.Email
		if ec == nil {
			return nil, fmt.Errorf("email is not configured")
		}
		pw, err := secrets.Resolve(ec.Password)
		if err != nil {
			return nil, fmt.Errorf("email: %w", err)
		}
		return email.New(email.Config{
			Address: ec.Address, IMAP: ec.IMAP, SMTP: ec.SMTP, Username: ec.Username, Password: pw,
			Mailbox: ec.Mailbox, TrustedAuthServ: ec.TrustedAuthServ,
		}, s.store), nil
	}
	return nil, fmt.Errorf("unknown channel %q: want telegram, discord or email", kind)
}

// setChannelConfig sets or, with an empty request, removes a channel in the config.
func setChannelConfig(c *agentdir.Config, req ChannelRequest) {
	switch req.Kind {
	case ChannelTelegram:
		c.Channels.Telegram = nil
		if req.Token != "" {
			c.Channels.Telegram = &agentdir.TelegramConfig{Token: req.Token}
		}
	case ChannelDiscord:
		c.Channels.Discord = nil
		if req.Token != "" {
			c.Channels.Discord = &agentdir.DiscordConfig{Token: req.Token}
		}
	case ChannelEmail:
		c.Channels.Email = req.Email
	}
}
