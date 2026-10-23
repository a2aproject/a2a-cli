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

package network

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestProbeAgents(t *testing.T) {
	t.Parallel()
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/agent-card.json" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewEncoder(w).Encode(map[string]any{"name": "coder", "description": "opencode agent"}); err != nil {
			t.Error(err)
		}
	}))
	defer agent.Close()
	other := httptest.NewServer(http.NotFoundHandler())
	defer other.Close()

	got := ProbeAgents(t.Context(), http.DefaultClient, []PeerInfo{
		{Name: "laptop", URL: agent.URL, Online: true},
		{Name: "printer", URL: other.URL, Online: true},
	})
	want := []PeerInfo{{Name: "coder", URL: agent.URL, Description: "opencode agent", Online: true}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("ProbeAgents() wrong result (-want +got) diff = %s", diff)
	}
}

func TestSplitListener(t *testing.T) {
	t.Parallel()
	closed := false
	l := newSplitListener(&net.TCPAddr{}, func() error { closed = true; return nil })
	a, b := net.Pipe()
	go l.deliver(a)
	c, err := l.Accept()
	if err != nil || c != a {
		t.Fatalf("Accept() = %v, %v, want the delivered connection", c, err)
	}
	if err := l.Close(); err != nil || !closed {
		t.Fatalf("Close() = %v, closer called %v, want nil, true", err, closed)
	}
	if _, err := l.Accept(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("Accept() after Close() error = %v, want %v", err, net.ErrClosed)
	}
	l.deliver(b)
}
