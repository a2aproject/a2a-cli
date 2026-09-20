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

package discover

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Cache persists probe results and per-session injection state so a hook that
// fires on every prompt and tool call stays cheap and quiet: probes are not
// repeated within a TTL, and an agent already surfaced in a session is not
// surfaced again.
type Cache struct {
	dir    string
	posTTL time.Duration
	negTTL time.Duration
}

type probeEntry struct {
	FetchedAt time.Time `json:"fetchedAt"`
	Found     bool      `json:"found"`
}

// NewCache returns a Cache rooted under the user cache directory. A zero-value
// Cache (dir == "") is valid and simply caches nothing.
func NewCache() Cache {
	base, err := os.UserCacheDir()
	if err != nil {
		return Cache{}
	}
	return Cache{
		dir:    filepath.Join(base, "a2a-cli", "discover"),
		posTTL: 30 * time.Minute,
		negTTL: 10 * time.Minute,
	}
}

// Fresh reports whether origin was probed recently enough to skip re-probing,
// along with whether that probe found an agent.
func (c Cache) Fresh(origin string) (found bool, fresh bool) {
	if c.dir == "" {
		return false, false
	}
	var e probeEntry
	if !readJSON(c.probePath(origin), &e) {
		return false, false
	}
	ttl := c.negTTL
	if e.Found {
		ttl = c.posTTL
	}
	if time.Since(e.FetchedAt) > ttl {
		return false, false
	}
	return e.Found, true
}

// Record stores the outcome of probing origin.
func (c Cache) Record(origin string, found bool) {
	if c.dir == "" {
		return
	}
	writeJSON(c.probePath(origin), probeEntry{FetchedAt: time.Now(), Found: found})
}

// InjectedInSession reports whether origin has already been surfaced in session.
func (c Cache) InjectedInSession(session, origin string) bool {
	if c.dir == "" || session == "" {
		return false
	}
	return readInjected(c.sessionPath(session))[origin]
}

// MarkInjected records that origins were surfaced in session.
func (c Cache) MarkInjected(session string, origins []string) {
	if c.dir == "" || session == "" || len(origins) == 0 {
		return
	}
	set := readInjected(c.sessionPath(session))
	for _, o := range origins {
		set[o] = true
	}
	writeJSON(c.sessionPath(session), set)
}

func (c Cache) probePath(origin string) string {
	return filepath.Join(c.dir, "probes", hash(origin)+".json")
}

func (c Cache) sessionPath(session string) string {
	return filepath.Join(c.dir, "sessions", hash(session)+".json")
}

func hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:16])
}

func readInjected(path string) map[string]bool {
	set := make(map[string]bool)
	readJSON(path, &set)
	return set
}

func readJSON(path string, v any) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return json.Unmarshal(data, v) == nil
}

func writeJSON(path string, v any) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o644)
}
