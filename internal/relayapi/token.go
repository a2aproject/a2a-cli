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

package relayapi

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
)

// EnvNodeToken is the environment variable the node commands read the node
// token from.
const EnvNodeToken = "A2A_RELAY_TOKEN"

// APIBase turns an agent URL into its node API base.
func APIBase(agentURL string) string {
	u := strings.TrimSuffix(agentURL, "/")
	if strings.HasSuffix(u, "/relay") {
		return u
	}
	return u + "/relay"
}

// NodeClient returns a client which acts as the node of token. The token
// carries the URL of the agent server hosting the node.
func NodeClient(token string) (*Client, error) {
	base, _, ok := ParseNodeToken(token)
	if !ok {
		return nil, errors.New("not a node token")
	}
	return &Client{BaseURL: base, Token: token}, nil
}

// Secret resolves an env:VAR reference to the value of VAR, which must be set.
func Secret(v string) (string, error) {
	name, ok := strings.CutPrefix(v, "env:")
	if !ok {
		return v, nil
	}
	val := os.Getenv(name)
	if val == "" {
		return "", fmt.Errorf("environment variable %s is empty", name)
	}
	return val, nil
}

// A node token is a2rn.<base64url node API base>.<node ID>.<secret>, so a
// node needs a single secret to reach the agent server hosting it.
const nodeTokenPrefix = "a2rn."

// NewNodeToken returns a node token.
func NewNodeToken(apiBase, nodeID, secret string) string {
	return nodeTokenPrefix + base64.RawURLEncoding.EncodeToString([]byte(apiBase)) + "." + nodeID + "." + secret
}

// AgentURL returns the URL of the agent server hosting the node of token.
func AgentURL(token string) (string, bool) {
	base, _, ok := ParseNodeToken(token)
	if !ok {
		return "", false
	}
	return strings.TrimSuffix(base, "/relay"), true
}

// ParseNodeToken returns the node API base and node ID embedded in a node token.
func ParseNodeToken(token string) (apiBase, nodeID string, ok bool) {
	rest, ok := strings.CutPrefix(token, nodeTokenPrefix)
	if !ok {
		return "", "", false
	}
	parts := strings.Split(rest, ".")
	if len(parts) != 3 || parts[1] == "" || parts[2] == "" {
		return "", "", false
	}
	u, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", "", false
	}
	return string(u), parts[1], true
}
