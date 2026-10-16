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

package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/a2aproject/a2a-cli/internal/relay"
)

func startTestRelay(t *testing.T) (relayURL, hookURL string, wakes <-chan relay.Envelope) {
	t.Helper()
	srv, err := relay.New(relay.Config{AdminToken: "admin-token", WebhookRetries: []time.Duration{0}})
	if err != nil {
		t.Fatalf("relay.New() error = %v", err)
	}
	rs := httptest.NewServer(srv)
	t.Cleanup(rs.Close)

	ch := make(chan relay.Envelope, 10)
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env relay.Envelope
		if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		ch <- env
	}))
	t.Cleanup(hook.Close)
	return rs.URL, hook.URL, ch
}

func nextWake(t *testing.T, wakes <-chan relay.Envelope, kind string) relay.Envelope {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case env := <-wakes:
			if env.Kind == kind {
				return env
			}
		case <-timeout:
			t.Fatalf("no %s wake within 5s", kind)
		}
	}
}

func runJSON[T any](t *testing.T, args ...string) T {
	t.Helper()
	out, err := runCMD(t, append(args, "-o", "json")...)
	if err != nil {
		t.Fatalf("runCMD(%v) error = %v", args, err)
	}
	var v T
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", out, err)
	}
	return v
}

func TestServerRelay_AttachPairReply(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(relay.EnvRelayURL, "")
	t.Setenv(relay.EnvNodeToken, "")
	t.Setenv(relay.EnvAdminToken, "")
	relayURL, hookURL, wakes := startTestRelay(t)

	attached := runJSON[relay.PairingResponse](t, "server", "relay", "attach", hookURL,
		"--relay", relayURL, "--admin-token", "admin-token", "--name", "grok")
	paired := runJSON[relay.PairResponse](t, "server", "relay", "pair", attached.PairingCode, "--relay", relayURL)
	if paired.Node.ID != attached.Node.ID {
		t.Fatalf("pair node = %q, want %q", paired.Node.ID, attached.Node.ID)
	}

	status := runJSON[relay.NodeInfo](t, "server", "relay", "status")
	if status.Name != "grok" || !status.Paired {
		t.Fatalf("server relay status = %+v, want paired node grok", status)
	}
	grant := runJSON[relay.GrantResponse](t, "server", "relay", "grant", "alice")

	if _, err := runCMD(t, "send", "-a", attached.Node.URL, "--auth", "Bearer "+grant.Token, "--async", "hello"); err != nil {
		t.Fatalf("send error = %v", err)
	}
	wake := nextWake(t, wakes, relay.KindTask)
	if wake.Text != "hello" || wake.Sender.String() != "grant:alice" {
		t.Fatalf("wake = %+v, want text hello from grant:alice", wake)
	}

	res := runJSON[relay.EventResult](t, "server", "relay", "reply", wake.TaskID, "--final", "hi alice")
	if res.State != "completed" {
		t.Fatalf("server relay reply state = %q, want completed", res.State)
	}
	retry := runJSON[relay.EventResult](t, "server", "relay", "reply", wake.TaskID, "--final", "hi alice")
	if !retry.Duplicate {
		t.Fatalf("server relay reply retry = %+v, want duplicate", retry)
	}

	var out string
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		var err error
		out, err = runCMD(t, "server", "relay", "task", wake.TaskID)
		if err != nil {
			t.Fatalf("server relay task error = %v", err)
		}
		if strings.Contains(out, ": completed") {
			break
		}
	}
	if !strings.Contains(out, ": completed") || !strings.Contains(out, "hi alice") {
		t.Fatalf("server relay task output = %q, want completed task with the answer", out)
	}
}
