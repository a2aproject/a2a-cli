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

package agentdir

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/a2aproject/a2a-cli/internal/fsutil"
)

// Config is the agent definition in .a2a/config.yaml. It holds no secrets:
// values like bot tokens are references of the form secret:<NAME>, resolved
// from the secrets file.
type Config struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Harness     string `yaml:"harness"`
	// Command replaces the harness profile's command line, for any ACP agent.
	Command []string `yaml:"command,omitempty"`
	// Image overrides the container image of the harness profile.
	Image string `yaml:"image,omitempty"`
	// Sandbox is where the harness runs: docker (default) or local.
	Sandbox string `yaml:"sandbox,omitempty"`
	// Mounts are src[:dst][:ro] specs. The first one is the workspace.
	// Empty means the directory holding .a2a.
	Mounts []string `yaml:"mounts,omitempty"`
	// Secrets are the names of secrets passed to the harness as environment variables.
	Secrets []string `yaml:"secrets,omitempty"`
	// Env are extra NAME=value pairs for the harness.
	Env []string `yaml:"env,omitempty"`
	// Port is the localhost port. Zero picks a free port on first run.
	Port int `yaml:"port,omitempty"`

	Network   NetworkConfig   `yaml:"network,omitempty"`
	Channels  ChannelsConfig  `yaml:"channels,omitempty"`
	Approvals ApprovalsConfig `yaml:"approvals,omitempty"`
	Limits    LimitsConfig    `yaml:"limits,omitempty"`
}

// NetworkConfig selects the providers which make the agent reachable beyond
// localhost.
type NetworkConfig struct {
	// Private is tailscale, lan or empty.
	Private string `yaml:"private,omitempty"`
	// Public is tailscale, ngrok or empty.
	Public string `yaml:"public,omitempty"`

	TailscaleControlURL string `yaml:"tailscale_control_url,omitempty"`
	// NgrokURL is a reserved ngrok domain, such as https://coder.ngrok.app.
	NgrokURL string `yaml:"ngrok_url,omitempty"`
	// LANPort is the port of the LAN listener. Zero picks one on first run.
	LANPort int `yaml:"lan_port,omitempty"`
	// Identities maps network logins of other users to roles.
	Identities map[string]string `yaml:"identities,omitempty"`
}

// ChannelsConfig configures the messengers the agent is reachable from.
type ChannelsConfig struct {
	Telegram *TelegramConfig `yaml:"telegram,omitempty"`
	Discord  *DiscordConfig  `yaml:"discord,omitempty"`
	Email    *EmailConfig    `yaml:"email,omitempty"`
}

// TelegramConfig configures the Telegram bot of the agent.
type TelegramConfig struct {
	Token string `yaml:"token"`
}

// DiscordConfig configures the Discord bot of the agent.
type DiscordConfig struct {
	Token string `yaml:"token"`
}

// EmailConfig configures the mailbox of the agent.
type EmailConfig struct {
	// Address is the agent's own address.
	Address string `yaml:"address"`
	// IMAP and SMTP are host:port. Port 993 and 465 use implicit TLS, others STARTTLS.
	IMAP     string `yaml:"imap"`
	SMTP     string `yaml:"smtp"`
	Username string `yaml:"username,omitempty"`
	Password string `yaml:"password"`
	Mailbox  string `yaml:"mailbox,omitempty"`
	// TrustedAuthServ is the authserv-id of the Authentication-Results header
	// added by the receiving mail server. Only its verdicts are trusted.
	TrustedAuthServ string `yaml:"trusted_authserv"`
}

// Channel kinds.
const (
	ChannelTelegram = "telegram"
	ChannelDiscord  = "discord"
	ChannelEmail    = "email"
)

// SetChannel configures the channel of kind. A bot token is a secret
// reference; an empty token or nil email removes the channel.
func (c *Config) SetChannel(kind, token string, email *EmailConfig) error {
	switch kind {
	case ChannelTelegram:
		c.Channels.Telegram = nil
		if token != "" {
			c.Channels.Telegram = &TelegramConfig{Token: token}
		}
	case ChannelDiscord:
		c.Channels.Discord = nil
		if token != "" {
			c.Channels.Discord = &DiscordConfig{Token: token}
		}
	case ChannelEmail:
		c.Channels.Email = email
	default:
		return fmt.Errorf("unknown channel %q: want telegram, discord or email", kind)
	}
	return nil
}

// ChannelToken returns the bot token reference of a telegram or discord
// channel, or "" if it isn't configured.
func (c *Config) ChannelToken(kind string) string {
	switch {
	case kind == ChannelTelegram && c.Channels.Telegram != nil:
		return c.Channels.Telegram.Token
	case kind == ChannelDiscord && c.Channels.Discord != nil:
		return c.Channels.Discord.Token
	}
	return ""
}

// ApprovalsConfig decides who answers tool permission requests.
type ApprovalsConfig struct {
	// Allow and Deny are ACP tool kind globs answered without asking.
	Allow []string `yaml:"allow,omitempty"`
	Deny  []string `yaml:"deny,omitempty"`
	// Approvers are principal IDs, besides owners, who may answer approvals.
	Approvers []string `yaml:"approvers,omitempty"`
}

// LimitsConfig are container resource limits.
type LimitsConfig struct {
	Memory string `yaml:"memory,omitempty"`
	CPUs   string `yaml:"cpus,omitempty"`
	PIDs   int    `yaml:"pids,omitempty"`
}

// LoadConfig reads config.yaml.
func (d Dir) LoadConfig() (*Config, error) {
	data, err := os.ReadFile(d.ConfigPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%s does not exist: %w", d.ConfigPath(), ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", d.ConfigPath(), err)
	}
	return &cfg, nil
}

// SaveConfig writes config.yaml.
func (d Dir) SaveConfig(cfg *Config) error {
	if err := d.Init(); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	header := "# Agent definition for `a2a server`. Secrets are kept in state/secrets, never here.\n"
	return fsutil.WriteAtomic(d.ConfigPath(), append([]byte(header), data...), 0o644)
}

// Mount is a parsed src[:dst][:ro] spec.
type Mount struct {
	Src      string
	Dst      string
	ReadOnly bool
}

// ParseMount parses a src[:dst][:ro] spec. A missing dst mirrors src.
func ParseMount(spec string) (Mount, error) {
	parts := strings.Split(spec, ":")
	m := Mount{}
	if n := len(parts); n > 1 && parts[n-1] == "ro" {
		m.ReadOnly = true
		parts = parts[:n-1]
	}
	switch len(parts) {
	case 1:
		m.Src = parts[0]
	case 2:
		m.Src, m.Dst = parts[0], parts[1]
	default:
		return Mount{}, fmt.Errorf("invalid mount %q: want src[:dst][:ro]", spec)
	}
	if m.Src == "" {
		return Mount{}, fmt.Errorf("invalid mount %q: empty source", spec)
	}
	src, err := expandHome(m.Src)
	if err != nil {
		return Mount{}, err
	}
	if m.Src, err = filepath.Abs(src); err != nil {
		return Mount{}, err
	}
	if m.Dst == "" {
		m.Dst = m.Src
	}
	if !filepath.IsAbs(m.Dst) {
		return Mount{}, fmt.Errorf("invalid mount %q: destination must be absolute", spec)
	}
	return m, nil
}

// String formats m as a spec.
func (m Mount) String() string {
	s := m.Src
	if m.Dst != m.Src {
		s += ":" + m.Dst
	}
	if m.ReadOnly {
		s += ":ro"
	}
	return s
}

// Mounts returns the parsed mounts of the agent. The first one is the
// workspace, which defaults to the directory holding .a2a.
func (d Dir) Mounts(cfg *Config) ([]Mount, error) {
	if len(cfg.Mounts) == 0 {
		return []Mount{{Src: d.Root, Dst: d.Root}}, nil
	}
	out := make([]Mount, 0, len(cfg.Mounts))
	for _, spec := range cfg.Mounts {
		m, err := ParseMount(spec)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func expandHome(p string) (string, error) {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, strings.TrimPrefix(p, "~")), nil
}
