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
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
)

// Settings are the start flags of `a2a server <harness>`. Set fields
// override the config; unset fields keep what earlier runs saved.
type Settings struct {
	Harness     string
	Name        string
	Description string
	Command     []string
	// Secrets are names of environment variables passed to the harness.
	// Their values are read from the CLI's environment and stored.
	Secrets []string
	Mounts  []string
	Env     []string
	Private string
	Public  string
	// Telegram and Discord are bot tokens: env:VAR, secret:NAME or the value.
	Telegram string
	Discord  string
	Email    *EmailSettings
	Sandbox  string
	Image    string
	Port     int

	AllowTools []string
	DenyTools  []string
	Approvers  []string

	TailscaleControlURL string
	NgrokURL            string
	// TailscaleAuthKey and NgrokAuthToken are env:VAR, secret:NAME or the value.
	TailscaleAuthKey string
	NgrokAuthToken   string
}

// EmailSettings configure the email channel. Password is env:VAR,
// secret:NAME or the value.
type EmailSettings struct {
	Address         string
	IMAP            string
	SMTP            string
	Username        string
	Password        string
	Mailbox         string
	TrustedAuthServ string
}

// Secret names under which flag values are stored.
const (
	SecretTelegram      = "TELEGRAM_BOT_TOKEN"
	SecretDiscord       = "DISCORD_BOT_TOKEN"
	SecretEmailPassword = "EMAIL_PASSWORD"
	SecretTailscaleKey  = "TS_AUTHKEY"
	SecretNgrokToken    = "NGROK_AUTHTOKEN"
)

// Apply merges the settings into cfg and returns the secrets to store.
func (s Settings) Apply(cfg *Config, defaultName string) (Secrets, error) {
	secrets := Secrets{}
	set := func(dst *string, v string) {
		if v != "" {
			*dst = v
		}
	}
	set(&cfg.Harness, s.Harness)
	set(&cfg.Name, s.Name)
	if cfg.Name == "" {
		cfg.Name = defaultName
	}
	cfg.Name = SanitizeName(cfg.Name)
	set(&cfg.Description, s.Description)
	set(&cfg.Sandbox, s.Sandbox)
	set(&cfg.Image, s.Image)
	setNetwork(&cfg.Network.Private, s.Private)
	setNetwork(&cfg.Network.Public, s.Public)
	set(&cfg.Network.TailscaleControlURL, s.TailscaleControlURL)
	set(&cfg.Network.NgrokURL, s.NgrokURL)
	if len(s.Command) > 0 {
		cfg.Command = s.Command
	}
	if len(s.Mounts) > 0 {
		for _, m := range s.Mounts {
			if _, err := ParseMount(m); err != nil {
				return nil, err
			}
		}
		cfg.Mounts = s.Mounts
	}
	if len(s.Env) > 0 {
		cfg.Env = s.Env
	}
	if s.Port != 0 {
		cfg.Port = s.Port
	}
	if len(s.AllowTools) > 0 {
		cfg.Approvals.Allow = s.AllowTools
	}
	if len(s.DenyTools) > 0 {
		cfg.Approvals.Deny = s.DenyTools
	}
	if len(s.Approvers) > 0 {
		cfg.Approvals.Approvers = s.Approvers
	}
	for _, name := range s.Secrets {
		v := os.Getenv(name)
		if v == "" {
			return nil, fmt.Errorf("--secret %s: the environment variable is not set", name)
		}
		secrets[name] = v
		if !slices.Contains(cfg.Secrets, name) {
			cfg.Secrets = append(cfg.Secrets, name)
		}
	}
	resolve := func(v, fallback string) (string, error) {
		ref, sec, err := ResolveInput(v, fallback)
		if err != nil {
			return "", err
		}
		maps.Copy(secrets, sec)
		return ref, nil
	}
	if s.Telegram != "" {
		ref, err := resolve(s.Telegram, SecretTelegram)
		if err != nil {
			return nil, fmt.Errorf("--telegram: %w", err)
		}
		cfg.Channels.Telegram = &TelegramConfig{Token: ref}
	}
	if s.Discord != "" {
		ref, err := resolve(s.Discord, SecretDiscord)
		if err != nil {
			return nil, fmt.Errorf("--discord: %w", err)
		}
		cfg.Channels.Discord = &DiscordConfig{Token: ref}
	}
	if e := s.Email; e != nil {
		if e.Address == "" || e.IMAP == "" || e.SMTP == "" || e.Password == "" || e.TrustedAuthServ == "" {
			return nil, fmt.Errorf("the email channel needs --email-address, --email-imap, --email-smtp, --email-password and --email-trusted-authserv")
		}
		ref, err := resolve(e.Password, SecretEmailPassword)
		if err != nil {
			return nil, fmt.Errorf("--email-password: %w", err)
		}
		cfg.Channels.Email = &EmailConfig{
			Address: e.Address, IMAP: e.IMAP, SMTP: e.SMTP, Username: e.Username, Password: ref,
			Mailbox: e.Mailbox, TrustedAuthServ: e.TrustedAuthServ,
		}
	}
	// Network providers read their secrets by well-known names.
	for name, v := range map[string]string{SecretTailscaleKey: s.TailscaleAuthKey, SecretNgrokToken: s.NgrokAuthToken} {
		if v == "" {
			continue
		}
		if ref, ok := strings.CutPrefix(v, SecretPrefix); ok && ref != name {
			return nil, fmt.Errorf("%s: the network provider reads the secret %s; pass the value or env:VAR instead of %s", name, name, v)
		}
		_, sec, err := ResolveInput(v, name)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		for _, val := range sec {
			secrets[name] = val
		}
	}
	if cfg.Harness == "" {
		return nil, fmt.Errorf("no harness: run `a2a server <harness>`")
	}
	return secrets, nil
}

// SettingsError is returned by [Dir.SaveSettings] for settings which can't
// be applied, as opposed to failures to read or write the agent directory.
type SettingsError struct{ Err error }

func (e *SettingsError) Error() string { return e.Err.Error() }

func (e *SettingsError) Unwrap() error { return e.Err }

// SaveSettings merges start flags into the agent's config and secrets.
func (d Dir) SaveSettings(s Settings) error {
	conf := &Config{}
	if d.Exists() {
		var err error
		if conf, err = d.LoadConfig(); err != nil {
			return err
		}
	}
	secrets, err := s.Apply(conf, d.DefaultName())
	if err != nil {
		return &SettingsError{Err: err}
	}
	if err := d.SaveSecrets(secrets); err != nil {
		return err
	}
	return d.SaveConfig(conf)
}

// setNetwork sets a provider; "none" removes it.
func setNetwork(dst *string, v string) {
	switch v {
	case "":
	case "none":
		*dst = ""
	default:
		*dst = v
	}
}
