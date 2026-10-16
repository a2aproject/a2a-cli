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

package relay

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Environment variables read by the relay client commands.
const (
	EnvRelayURL   = "A2A_RELAY_URL"
	EnvNodeToken  = "A2A_RELAY_TOKEN"
	EnvAdminToken = "A2A_RELAY_ADMIN_TOKEN"
)

// ClientConfig is what the relay client commands keep between runs. pair
// saves the node token and attach saves the admin token.
type ClientConfig struct {
	Relay      string `json:"relay,omitempty"`
	Token      string `json:"token,omitempty"`
	AdminToken string `json:"adminToken,omitempty"`
}

// ClientConfigPath returns ~/.config/a2a-cli/relay.json.
func ClientConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".config", "a2a-cli", "relay.json"), nil
}

// LoadClientConfig reads the client config. A missing file is an empty config.
func LoadClientConfig() (ClientConfig, error) {
	var cfg ClientConfig
	path, err := ClientConfigPath()
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parsing %s: %w", path, err)
	}
	return cfg, nil
}

// UpdateClientConfig applies fn to the client config and saves it. It
// returns the path of the file.
func UpdateClientConfig(fn func(*ClientConfig)) (string, error) {
	cfg, err := LoadClientConfig()
	if err != nil {
		return "", err
	}
	fn(&cfg)
	path, err := ClientConfigPath()
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, data, 0o600)
}

// ClientOptions select the relay and the credentials of a client command.
// Unset values fall back to the environment, then to the client config.
type ClientOptions struct {
	Relay      string
	Token      string
	AdminToken string
	// Node makes the owner act on this node with the admin token.
	Node string
}

// RelayURL returns the relay URL from the options, the environment, the node
// token or the client config.
func (o ClientOptions) RelayURL(cfg ClientConfig) string {
	if o.Relay != "" {
		return strings.TrimSuffix(o.Relay, "/")
	}
	if v := os.Getenv(EnvRelayURL); v != "" {
		return strings.TrimSuffix(v, "/")
	}
	if u, _, ok := ParseNodeToken(o.nodeToken(cfg)); ok {
		return u
	}
	return strings.TrimSuffix(cfg.Relay, "/")
}

func (o ClientOptions) nodeToken(cfg ClientConfig) string {
	if o.Token != "" {
		return Secret(o.Token)
	}
	if v := os.Getenv(EnvNodeToken); v != "" {
		return v
	}
	return cfg.Token
}

func (o ClientOptions) adminToken(cfg ClientConfig) string {
	if o.AdminToken != "" {
		return Secret(o.AdminToken)
	}
	if v := os.Getenv(EnvAdminToken); v != "" {
		return v
	}
	return cfg.AdminToken
}

// AdminClient returns a client that authenticates with the admin token.
func (o ClientOptions) AdminClient(cfg ClientConfig) (*Client, error) {
	base, token := o.RelayURL(cfg), o.adminToken(cfg)
	if base == "" || token == "" {
		return nil, fmt.Errorf("set --relay and --admin-token (or %s and %s)", EnvRelayURL, EnvAdminToken)
	}
	return &Client{BaseURL: base, Token: token, NodeID: o.Node}, nil
}

// NodeClient returns a client that acts as the node with its token, or as
// the owner on Node with the admin token.
func (o ClientOptions) NodeClient(cfg ClientConfig) (*Client, error) {
	if o.Node != "" {
		return o.AdminClient(cfg)
	}
	token := o.nodeToken(cfg)
	if token == "" {
		return nil, fmt.Errorf("no node token: run `%s pair <code>`, or set %s", CLI, EnvNodeToken)
	}
	base := o.RelayURL(cfg)
	if base == "" {
		return nil, fmt.Errorf("set --relay or %s", EnvRelayURL)
	}
	return &Client{BaseURL: base, Token: token}, nil
}

// Secret resolves an env:VAR reference to the value of VAR.
func Secret(v string) string {
	if name, ok := strings.CutPrefix(v, "env:"); ok {
		return os.Getenv(name)
	}
	return v
}
