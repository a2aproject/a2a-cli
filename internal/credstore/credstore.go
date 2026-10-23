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

// Package credstore keeps the pairing tokens `a2a pair add` obtained: caller
// tokens, which later commands attach to requests for the same agent, and
// node tokens, which the `a2a relay` commands use.
package credstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/a2aproject/a2a-cli/internal/fsutil"
	"github.com/a2aproject/a2a-cli/internal/pairapi"
)

// KindNode marks the token of a webhook node; other credentials are caller tokens.
const KindNode = pairapi.KindNode

// Credential is a token for one agent URL, or for one webhook node.
type Credential struct {
	URL       string    `json:"url"`
	Kind      string    `json:"kind,omitempty"`
	Token     string    `json:"token"`
	Agent     string    `json:"agent,omitempty"`
	Principal string    `json:"principal,omitempty"`
	Created   time.Time `json:"created"`
}

// Store is a credentials file, readable only by its owner.
type Store struct {
	Path string
}

// Default returns the store at ~/.config/a2a-cli/credentials.json.
func Default() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolving home directory: %w", err)
	}
	return At(home), nil
}

// At returns the store under the home directory home.
func At(home string) *Store {
	return &Store{Path: filepath.Join(home, ".config", "a2a-cli", "credentials.json")}
}

// List returns the stored credentials.
func (s *Store) List() ([]Credential, error) {
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Credential
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", s.Path, err)
	}
	return out, nil
}

func (s *Store) save(creds []Credential) error {
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	return fsutil.WriteAtomic(s.Path, data, 0o600)
}

// update applies fn to the stored credentials under a lock, so that
// concurrent commands don't lose each other's changes.
func (s *Store) update(fn func([]Credential) ([]Credential, bool)) error {
	return fsutil.WithLock(s.Path, func() error {
		creds, err := s.List()
		if err != nil {
			return err
		}
		next, changed := fn(creds)
		if !changed {
			return nil
		}
		return s.save(next)
	})
}

// Put stores a credential, replacing one of the same kind for the same URL.
func (s *Store) Put(c Credential) error {
	c.URL = Normalize(c.URL)
	return s.update(func(creds []Credential) ([]Credential, bool) {
		creds = slices.DeleteFunc(creds, func(x Credential) bool { return x.URL == c.URL && x.Kind == c.Kind })
		return append(creds, c), true
	})
}

// Delete removes the credentials for url. It reports whether one existed.
func (s *Store) Delete(u string) (bool, error) {
	found := false
	err := s.update(func(creds []Credential) ([]Credential, bool) {
		n := len(creds)
		creds = slices.DeleteFunc(creds, func(x Credential) bool { return x.URL == Normalize(u) })
		found = len(creds) != n
		return creds, found
	})
	return found, err
}

// Lookup returns the caller credential whose URL is target or the longest
// prefix of it, so a token for an agent also covers its card and endpoints.
func (s *Store) Lookup(target string) (Credential, bool, error) {
	creds, err := s.List()
	if err != nil {
		return Credential{}, false, err
	}
	t := Normalize(target)
	var best Credential
	for _, c := range creds {
		if c.Kind == KindNode {
			continue
		}
		if (t == c.URL || strings.HasPrefix(t, c.URL+"/")) && len(c.URL) > len(best.URL) {
			best = c
		}
	}
	return best, best.URL != "", nil
}

// Node returns the only saved node token. A machine hosting several nodes
// has to name the token explicitly.
func (s *Store) Node() (Credential, error) {
	creds, err := s.List()
	if err != nil {
		return Credential{}, err
	}
	creds = slices.DeleteFunc(creds, func(c Credential) bool { return c.Kind != KindNode })
	switch len(creds) {
	case 0:
		return Credential{}, ErrNoNode
	case 1:
		return creds[0], nil
	default:
		return Credential{}, fmt.Errorf("%d node tokens saved in %s; pass the one to use with --token", len(creds), s.Path)
	}
}

// ErrNoNode is returned by [Store.Node] when no node token is saved.
var ErrNoNode = errors.New("no node token saved")

// Normalize drops the query, the trailing slash and the well-known card
// path, and lowercases the scheme and host.
func Normalize(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return strings.TrimSuffix(raw, "/")
	}
	u.Scheme, u.Host = strings.ToLower(u.Scheme), strings.ToLower(u.Host)
	u.RawQuery, u.Fragment, u.User = "", "", nil
	u.Path = strings.TrimSuffix(u.Path, "/.well-known/agent-card.json")
	u.Path = strings.TrimSuffix(u.Path, "/.well-known/agent.json")
	u.Path = strings.TrimSuffix(u.Path, "/")
	u.RawPath = ""
	return u.String()
}

// Pair exchanges a pairing code at agentURL for a credential.
func Pair(ctx context.Context, hc *http.Client, agentURL, code string) (Credential, error) {
	base := Normalize(agentURL)
	pr, err := pairapi.Pair(ctx, hc, base, code)
	if err != nil {
		return Credential{}, err
	}
	c := Credential{URL: base, Token: pr.Token, Agent: pr.Agent, Principal: pr.Principal, Created: time.Now().UTC()}
	if pr.Kind == KindNode {
		c.Kind, c.URL = KindNode, Normalize(pr.URL)
	}
	return c, nil
}
