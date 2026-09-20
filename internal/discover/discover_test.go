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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestExtractOrigins(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "no urls",
			text: "just some prose with no links",
			want: nil,
		},
		{
			name: "dedupes by origin and keeps order",
			text: "see http://a.example/x and https://b.example:8080/y then http://a.example/z",
			want: []string{"http://a.example", "https://b.example:8080"},
		},
		{
			name: "trims trailing punctuation",
			text: "visit https://c.example/page, or https://d.example.",
			want: []string{"https://c.example", "https://d.example"},
		},
		{
			name: "ignores non-http schemes",
			text: "ftp://x.example and mailto:a@b.example but http://ok.example counts",
			want: []string{"http://ok.example"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ExtractOrigins(tt.text)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("ExtractOrigins() wrong result (-want +got) diff = %s", diff)
			}
		})
	}
}

const sampleCard = `{
  "name": "Nimbus Order Assistant",
  "description": "Checks live order status for Nimbus Roasters.",
  "version": "1.0.0",
  "capabilities": {"streaming": true},
  "defaultInputModes": ["text"],
  "defaultOutputModes": ["text"],
  "supportedInterfaces": [{"url": "http://example/a2a", "protocolBinding": "HTTP+JSON", "protocolVersion": "1.0"}],
  "skills": [{
    "id": "order-status",
    "name": "Order status lookup",
    "description": "Look up the live status of an order by its ID.",
    "tags": ["orders", "shipping"],
    "examples": ["Where is order NR-2041?"]
  }]
}`

func TestRunDiscoversAgent(t *testing.T) {
	t.Parallel()

	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/agent-card.json" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(sampleCard))
	}))
	defer agent.Close()

	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer plain.Close()

	input := []byte(`{"hook_event_name":"UserPromptSubmit","session_id":"s1","prompt":"check my order at ` +
		agent.URL + ` and also look at ` + plain.URL + `"}`)

	out, err := Run(context.Background(), RunConfig{
		Input: input,
		Guard: Guard{AllowPrivate: true},
	})
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}

	var got hookOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json.Unmarshal(Run output) error = %v, output = %q", err, out)
	}
	if got.HookSpecificOutput.HookEventName != "UserPromptSubmit" {
		t.Errorf("Run() hookEventName = %q, want %q", got.HookSpecificOutput.HookEventName, "UserPromptSubmit")
	}
	ctx := got.HookSpecificOutput.AdditionalContext
	for _, want := range []string{"Nimbus Order Assistant", "Order status lookup", "a2a send -a " + agent.URL, "untrusted"} {
		if !strings.Contains(ctx, want) {
			t.Errorf("Run() additionalContext missing %q; got:\n%s", want, ctx)
		}
	}
	if strings.Contains(ctx, plain.URL) {
		t.Errorf("Run() surfaced a non-A2A origin %q; got:\n%s", plain.URL, ctx)
	}
}

func TestRunNoAgentsIsSilent(t *testing.T) {
	t.Parallel()

	input := []byte(`{"hook_event_name":"UserPromptSubmit","prompt":"no urls here at all"}`)
	out, err := Run(context.Background(), RunConfig{Input: input, Guard: Guard{AllowPrivate: true}})
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if out != "" {
		t.Errorf("Run() = %q, want empty output when nothing is discovered", out)
	}
}

func TestRunMalformedInput(t *testing.T) {
	t.Parallel()

	_, err := Run(context.Background(), RunConfig{Input: []byte("{not json"), Guard: Guard{AllowPrivate: true}})
	if err == nil {
		t.Fatalf("Run() error = nil, want an error for malformed input")
	}
}
