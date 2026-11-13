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

// Package cardcache persists fetched agent cards, and failed fetches, on disk so
// contacts and agent discovery don't refetch a card every time they need it.
package cardcache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"

	"github.com/a2aproject/a2a-cli/internal/atomicfile"
)

const wellKnownPath = "/.well-known/agent-card.json"

// FoundTTL is how long a fetched card is used before it is fetched again.
const FoundTTL = 24 * time.Hour

// Entry is the outcome of fetching the card at Ref.
type Entry struct {
	Ref       string    `json:"ref"`
	FetchedAt time.Time `json:"fetchedAt"`
	// Card is nil when no agent card was found at Ref.
	Card  *a2a.AgentCard `json:"card,omitempty"`
	Error string         `json:"error,omitempty"`
}

// Found reports whether the entry holds a card.
func (e Entry) Found() bool { return e.Card != nil }

// Fresh reports whether the entry is younger than ttl at now.
func (e Entry) Fresh(now time.Time, ttl time.Duration) bool {
	return now.Sub(e.FetchedAt) < ttl
}

// Cache is a directory of cached card entries. A nil *Cache caches nothing.
type Cache struct {
	dir string
}

// DefaultDir returns the a2a-cli cache directory under the user cache directory.
func DefaultDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolving cache directory: %w", err)
	}
	return filepath.Join(base, "a2a-cli"), nil
}

// New returns a cache that stores entries under dir/cards.
func New(dir string) *Cache {
	return &Cache{dir: filepath.Join(dir, "cards")}
}

// Get returns the cached entry for ref.
func (c *Cache) Get(ref string) (Entry, bool) {
	if c == nil {
		return Entry{}, false
	}
	data, err := os.ReadFile(c.path(ref))
	if err != nil {
		return Entry{}, false
	}
	var e Entry
	if err := json.Unmarshal(data, &e); err != nil {
		return Entry{}, false
	}
	return e, true
}

// Put stores e under its Ref.
func (c *Cache) Put(e Entry) error {
	if c == nil {
		return nil
	}
	if e.Ref == "" {
		return errors.New("cache entry without a ref")
	}
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("encoding card cache entry: %w", err)
	}
	if err := atomicfile.Write(c.path(e.Ref), data, 0o600); err != nil {
		return fmt.Errorf("writing card cache entry: %w", err)
	}
	return nil
}

func (c *Cache) path(ref string) string {
	sum := sha256.Sum256([]byte(Key(ref)))
	return filepath.Join(c.dir, hex.EncodeToString(sum[:16])+".json")
}

// Key normalizes a card reference so an origin and its well-known card URL
// share one entry.
func Key(ref string) string {
	u, err := url.Parse(ref)
	if err != nil || u.Host == "" {
		return ref
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	if u.Path == "" || u.Path == "/" {
		u.Path = wellKnownPath
	}
	u.RawPath = ""
	u.Fragment = ""
	return u.String()
}
