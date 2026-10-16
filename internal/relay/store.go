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
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// node is the persisted record of an attached node. Secrets are kept as
// SHA-256 hashes.
type node struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Description    string            `json:"description,omitempty"`
	Webhook        string            `json:"webhook"`
	WebhookHeaders map[string]string `json:"webhookHeaders,omitempty"`
	Public         bool              `json:"public"`
	TokenHash      string            `json:"tokenHash,omitempty"`
	PairingHash    string            `json:"pairingHash,omitempty"`
	PairingExpires time.Time         `json:"pairingExpires,omitzero"`
	Grants         []grant           `json:"grants,omitempty"`
	CreatedAt      time.Time         `json:"createdAt"`
}

// grant lets a caller reach a node. Grants for other nodes (node:<id>) have
// no token.
type grant struct {
	Name      string    `json:"name"`
	TokenHash string    `json:"tokenHash,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

func (n *node) grantNames() []string {
	names := []string{}
	for _, g := range n.Grants {
		names = append(names, g.Name)
	}
	return names
}

func (n *node) grantByToken(token string) (grant, bool) {
	h := hash(token)
	for _, g := range n.Grants {
		if g.TokenHash != "" && subtle.ConstantTimeCompare([]byte(g.TokenHash), []byte(h)) == 1 {
			return g, true
		}
	}
	return grant{}, false
}

func (n *node) allows(name string) bool {
	return slices.ContainsFunc(n.Grants, func(g grant) bool { return g.Name == name })
}

// store keeps nodes in memory and in a JSON file, if it has a path. It is
// meant for a single relay instance.
type store struct {
	path  string
	mu    sync.Mutex
	nodes map[string]*node
}

func openStore(path string) (*store, error) {
	s := &store{path: path, nodes: map[string]*node{}}
	if path == "" {
		return s, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var nodes []*node
	if err := json.Unmarshal(data, &nodes); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	for _, n := range nodes {
		s.nodes[n.ID] = n
	}
	return s, nil
}

// saveLocked writes all nodes. The file is written in place rather than
// renamed, since the state directory can be a Cloud Storage mount.
func (s *store) saveLocked() error {
	if s.path == "" {
		return nil
	}
	nodes := make([]*node, 0, len(s.nodes))
	for _, n := range s.nodes {
		nodes = append(nodes, n)
	}
	slices.SortFunc(nodes, func(a, b *node) int { return strings.Compare(a.ID, b.ID) })
	data, err := json.MarshalIndent(nodes, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

// get returns a copy of the node.
func (s *store) get(id string) (node, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.nodes[id]
	if !ok {
		return node{}, false
	}
	return n.clone(), true
}

func (n *node) clone() node {
	c := *n
	c.Grants = slices.Clone(n.Grants)
	return c
}

func (s *store) create(n *node) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nodes[n.ID] = n
	return s.saveLocked()
}

// update applies fn to the node and saves it, unless fn fails.
func (s *store) update(id string, fn func(n *node) error) (node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.nodes[id]
	if !ok {
		return node{}, errNodeNotFound
	}
	c := n.clone()
	if err := fn(&c); err != nil {
		return node{}, err
	}
	*n = c
	return c, s.saveLocked()
}

// pairWith finds the node with the pairing code, consumes the code and sets
// the node token hash returned by mint, replacing any previous token.
func (s *store) pairWith(code string, now time.Time, mint func(n *node) string) (node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := hash(normalizeCode(code))
	for _, n := range s.nodes {
		if n.PairingHash == "" || subtle.ConstantTimeCompare([]byte(n.PairingHash), []byte(h)) != 1 {
			continue
		}
		if now.After(n.PairingExpires) {
			return node{}, errors.New("pairing code expired")
		}
		n.PairingHash, n.PairingExpires = "", time.Time{}
		n.TokenHash = mint(n)
		return n.clone(), s.saveLocked()
	}
	return node{}, errors.New("unknown pairing code")
}

var errNodeNotFound = errors.New("node not found")

// Tokens.
//
// A node token is a2rn.<base64url relay URL>.<node ID>.<secret>, so a node
// needs a single secret to reach its relay. A grant token is a2rg_<secret>
// and is looked up among the grants of the node being called.

const (
	nodeTokenPrefix  = "a2rn."
	grantTokenPrefix = "a2rg_"
)

func newNodeToken(relayURL, nodeID string) string {
	return nodeTokenPrefix + base64.RawURLEncoding.EncodeToString([]byte(relayURL)) + "." + nodeID + "." + randomString(32)
}

// ParseNodeToken returns the relay URL and node ID embedded in a node token.
func ParseNodeToken(token string) (relayURL, nodeID string, ok bool) {
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

func newGrantToken() string { return grantTokenPrefix + randomString(32) }

func newNodeID() string { return strings.ToLower(randomString(8)) }

// newPairingCode returns a code like K7QF-M2XD, easy to read out in a chat.
func newPairingCode() string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b[:4]) + "-" + string(b[4:])
}

func normalizeCode(code string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
}

func randomString(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	s := base64.RawURLEncoding.EncodeToString(b)
	// Keep IDs free of characters that are awkward in URLs and tokens.
	s = strings.NewReplacer("-", "x", "_", "y").Replace(s)
	return s[:n]
}

func hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
