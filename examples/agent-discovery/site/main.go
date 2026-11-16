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

// Command acme-docs serves a made-up developer documentation site that also
// publishes an A2A agent: a docs page at /, the agent card at
// /.well-known/agent-card.json and the agent itself at /a2a.
//
//	go run ./examples/agent-discovery/site --addr 127.0.0.1:8080 --url http://docs.acme.test:8080
package main

import (
	"context"
	"flag"
	"fmt"
	"iter"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

const page = `<!doctype html>
<html lang="en">
<head><title>Acme Payments API: Rate limits</title></head>
<body>
<h1>Acme Payments API</h1>
<h2>Rate limits</h2>
<p>Requests over your key's limit get HTTP 429. The limits, the retry policy
and the idempotency rules for retried charges depend on your account and API
version, and are not listed on this page.</p>
<p>Ask the Acme Docs Assistant, the A2A agent of this site, for the policy that
applies to you and for client code that follows it.</p>
</body>
</html>
`

// answer is the agent's reply. A real assistant would call a model; this one
// gives the same answer every time, so the demo is reproducible.
const answer = `Retry 429 responses from the Acme Payments API like this:

1. Wait for the number of seconds in the Retry-After header before the next attempt.
2. Without the header, back off exponentially from 1s, doubling up to 32s, with jitter.
3. Give up after 5 attempts and surface the error.
4. Only retry POST /v2/charges with the same Idempotency-Key, so a retried charge is never applied twice.

Source: https://docs.acme.test/payments/rate-limits`

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "Address to listen on")
	url := flag.String("url", "http://docs.acme.test:8080", "Public URL of the site, put in the agent card")
	flag.Parse()

	card := &a2a.AgentCard{
		Name:        "Acme Docs Assistant",
		Description: "Answers questions about the Acme Payments API from Acme's official developer documentation.",
		Version:     "1.0.0",
		Provider:    &a2a.AgentProvider{Org: "Acme Payments", URL: *url},
		SupportedInterfaces: []*a2a.AgentInterface{
			a2a.NewAgentInterface(strings.TrimSuffix(*url, "/")+"/a2a", a2a.TransportProtocolJSONRPC),
		},
		DefaultInputModes:  []string{"text/plain"},
		DefaultOutputModes: []string{"text/plain"},
		Skills: []a2a.AgentSkill{{
			ID:          "payments-api-help",
			Name:        "Payments API integration help",
			Description: "Explains rate limits, retries, idempotency keys, error codes and v1 to v2 migration for the Acme Payments API, with working code.",
			Tags:        []string{"payments", "api", "rate-limits", "retries"},
			Examples:    []string{"How should my client retry 429 responses?"},
		}},
	}

	mux := http.NewServeMux()
	mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(card))
	mux.Handle("/a2a", a2asrv.NewJSONRPCHandler(a2asrv.NewHandler(assistant{})))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if _, err := fmt.Fprint(w, page); err != nil {
			log.Printf("writing page: %v", err)
		}
	})

	log.Printf("serving %s on %s", *url, *addr)
	if err := http.ListenAndServe(*addr, logCard(mux)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// logCard logs the requests for the agent card, so the demo shows the probe.
func logCard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == a2asrv.WellKnownAgentCardPath {
			log.Printf("%s %s (agent card)", r.Method, r.URL.Path)
		}
		if r.URL.Path == "/a2a" {
			log.Printf("%s %s (A2A request)", r.Method, r.URL.Path)
		}
		next.ServeHTTP(w, r)
	})
}

type assistant struct{}

func (assistant) Execute(_ context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		if execCtx.StoredTask == nil && !yield(a2a.NewSubmittedTask(execCtx, execCtx.Message), nil) {
			return
		}
		evt := a2a.NewArtifactEvent(execCtx, a2a.NewTextPart(answer))
		evt.LastChunk = true
		if !yield(evt, nil) {
			return
		}
		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCompleted, nil), nil)
	}
}

func (assistant) Cancel(_ context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCanceled, nil), nil)
	}
}
