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

package credstore

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/a2aproject/a2a-cli/internal/pairapi"
)

func TestLookup(t *testing.T) {
	t.Parallel()
	s := &Store{Path: filepath.Join(t.TempDir(), "credentials.json")}
	for _, c := range []Credential{
		{URL: "https://assistant.tail1234.ts.net/", Token: "a2p_root"},
		{URL: "https://assistant.tail1234.ts.net/n/grok", Token: "a2p_node"},
	} {
		if err := s.Put(c); err != nil {
			t.Fatalf("Put() error = %v", err)
		}
	}
	tests := []struct {
		target string
		want   string
	}{
		{target: "https://assistant.tail1234.ts.net", want: "a2p_root"},
		{target: "https://Assistant.tail1234.ts.net/.well-known/agent-card.json", want: "a2p_root"},
		{target: "https://assistant.tail1234.ts.net/n/grok/.well-known/agent-card.json", want: "a2p_node"},
		{target: "https://assistant.tail1234.ts.net/n/grokx", want: "a2p_root"},
		{target: "https://other.example.com", want: ""},
	}
	for _, tc := range tests {
		got, _, err := s.Lookup(tc.target)
		if err != nil {
			t.Fatalf("Lookup() error = %v", err)
		}
		if got.Token != tc.want {
			t.Errorf("Lookup(%q) = %q, want %q", tc.target, got.Token, tc.want)
		}
	}
	info, err := os.Stat(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("credentials mode = %v, want 0600", info.Mode().Perm())
	}
	if ok, err := s.Delete("https://assistant.tail1234.ts.net/n/grok/"); err != nil || !ok {
		t.Fatalf("Delete() = %v, %v, want true, nil", ok, err)
	}
}

func TestPair(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || r.URL.Path != "/pair" {
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		if req["code"] != "K7Q2-9XF4" {
			w.WriteHeader(http.StatusForbidden)
			if err := json.NewEncoder(w).Encode(map[string]string{"error": "invalid or expired pairing code"}); err != nil {
				t.Error(err)
			}
			return
		}
		if err := json.NewEncoder(w).Encode(pairapi.Response{Token: "a2p_x", Principal: "pair:grok", Agent: "assistant"}); err != nil {
			t.Error(err)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	c, err := Pair(ctx, srv.Client(), srv.URL+"/", "K7Q2-9XF4")
	if err != nil {
		t.Fatalf("Pair() error = %v", err)
	}
	if c.Token != "a2p_x" || c.Agent != "assistant" || c.URL != srv.URL {
		t.Fatalf("Pair() = %+v, want token a2p_x for %s", c, srv.URL)
	}
	if _, err := Pair(ctx, srv.Client(), srv.URL, "WRONG"); err == nil {
		t.Fatalf("Pair() with a wrong code error = nil, want error")
	}
}

func TestPairNode(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := pairapi.Response{Kind: KindNode, Token: "a2rn.x", Principal: "node:grok", Agent: "Grok", URL: "http://" + r.Host + "/n/grok"}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Error(err)
		}
	}))
	defer srv.Close()
	got, err := Pair(context.Background(), srv.Client(), srv.URL, "K7Q2-9XF4")
	if err != nil {
		t.Fatalf("Pair() error = %v", err)
	}
	want := Credential{URL: srv.URL + "/n/grok", Kind: KindNode, Token: "a2rn.x", Agent: "Grok", Principal: "node:grok", Created: got.Created}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("Pair() wrong result (-want +got) diff = %s", diff)
	}
}

func TestNodeTokensAreKeptApart(t *testing.T) {
	t.Parallel()
	s := &Store{Path: filepath.Join(t.TempDir(), "credentials.json")}
	if _, err := s.Node(); !errors.Is(err, ErrNoNode) {
		t.Fatalf("Node() error = %v, want %v", err, ErrNoNode)
	}
	for _, c := range []Credential{
		{URL: "https://assistant.example.com/n/grok", Token: "a2p_caller"},
		{URL: "https://assistant.example.com/n/grok", Kind: KindNode, Token: "a2rn.grok"},
	} {
		if err := s.Put(c); err != nil {
			t.Fatalf("Put() error = %v", err)
		}
	}
	caller, _, err := s.Lookup("https://assistant.example.com/n/grok")
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if caller.Token != "a2p_caller" {
		t.Fatalf("Lookup() = %q, want %q", caller.Token, "a2p_caller")
	}
	node, err := s.Node()
	if err != nil {
		t.Fatalf("Node() error = %v", err)
	}
	if node.Token != "a2rn.grok" {
		t.Fatalf("Node() = %q, want %q", node.Token, "a2rn.grok")
	}
	if err := s.Put(Credential{URL: "https://other.example.com/n/bot", Kind: KindNode, Token: "a2rn.bot"}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if _, err := s.Node(); err == nil {
		t.Fatalf("Node() with two node tokens error = nil, want error")
	}
}
