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

// Command server runs the progressive-discovery demo: one origin that serves
// both an ordinary marketing website (at /) and an A2A agent (its Agent Card at
// /.well-known/agent-card.json, its REST interface under /a2a). It lets a
// harness fetch the site like any web page and then discover, at the same
// origin, an agent that can do the very task the user asked for.
package main

import (
	"context"
	"flag"
	"fmt"
	"iter"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

func main() {
	addr := flag.String("addr", "localhost:8080", "host:port to listen on")
	publicURL := flag.String("public-url", "", "origin to advertise in the Agent Card (defaults to http://<addr>)")
	flag.Parse()

	origin := *publicURL
	if origin == "" {
		origin = "http://" + *addr
	}

	card := buildCard(origin)
	handler := a2asrv.NewHandler(&orderAgent{}, a2asrv.WithCapabilityChecks(&a2a.AgentCapabilities{Streaming: false}))

	mux := http.NewServeMux()
	mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(card))
	mux.Handle("/a2a/", http.StripPrefix("/a2a", a2asrv.NewRESTHandler(handler)))
	mux.HandleFunc("/", homePage)

	log.Printf("Nimbus demo: serving %s (bound to %s)", origin, *addr)
	log.Printf("  website:    %s/", origin)
	log.Printf("  agent card: %s%s", origin, a2asrv.WellKnownAgentCardPath)
	log.Printf("  a2a (REST): %s/a2a", origin)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}

// buildCard describes the Nimbus order assistant. Its single skill is exactly
// the capability the demo asks for: looking up live order status, which the
// harness cannot do on its own because the data lives on this server.
func buildCard(origin string) *a2a.AgentCard {
	return &a2a.AgentCard{
		Name:        "Nimbus Order Assistant",
		Description: "Answers questions about Nimbus Roasters orders — looks up live order status and delivery estimates by order ID.",
		Version:     "1.0.0",
		Provider: &a2a.AgentProvider{
			Org: "Nimbus Roasters",
			URL: origin,
		},
		Capabilities:       a2a.AgentCapabilities{Streaming: false},
		DefaultInputModes:  []string{"text"},
		DefaultOutputModes: []string{"text"},
		SupportedInterfaces: []*a2a.AgentInterface{
			a2a.NewAgentInterface(origin+"/a2a", a2a.TransportProtocolHTTPJSON),
		},
		Skills: []a2a.AgentSkill{{
			ID:          "order-status",
			Name:        "Order status lookup",
			Description: "Given a Nimbus order ID (like NR-2041), report where the order is and when it will arrive.",
			Tags:        []string{"orders", "shipping", "support"},
			Examples: []string{
				"Where is my order NR-2041?",
				"status of NR-1088",
			},
		}},
	}
}

// order is a row in the demo's canned order database.
type order struct {
	status string
	eta    string
}

var orderDB = map[string]order{
	"NR-2041": {status: "Roasted and shipped on Mar 3 via UPS (1Z-NIMBUS-2041)", eta: "arriving tomorrow by 8pm"},
	"NR-1088": {status: "Roasting now — your Ethiopia Guji ships within 24 hours", eta: "estimated delivery in 3–4 days"},
	"NR-3300": {status: "Delivered on Mar 1, left at front door", eta: "already delivered"},
}

var orderIDPattern = regexp.MustCompile(`(?i)NR-?\d{3,5}`)

// orderAgent is the A2A executor: it reads the order ID from the incoming
// message and returns that order's live status.
type orderAgent struct{}

func (a *orderAgent) Execute(_ context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		if execCtx.StoredTask == nil {
			if !yield(a2a.NewSubmittedTask(execCtx, execCtx.Message), nil) {
				return
			}
		}
		if !yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateWorking, nil), nil) {
			return
		}

		reply := a.lookup(messageText(execCtx.Message))
		evt := a2a.NewArtifactEvent(execCtx, a2a.NewTextPart(reply))
		evt.LastChunk = true
		if !yield(evt, nil) {
			return
		}
		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCompleted, nil), nil)
	}
}

func (a *orderAgent) Cancel(_ context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCanceled, nil), nil)
	}
}

func (a *orderAgent) lookup(text string) string {
	id := strings.ToUpper(orderIDPattern.FindString(text))
	if id == "" {
		return "I can look up any Nimbus order — just give me the order ID, e.g. NR-2041."
	}
	id = strings.Replace(id, "NR", "NR-", 1)
	id = strings.ReplaceAll(id, "--", "-")
	o, ok := orderDB[id]
	if !ok {
		return fmt.Sprintf("I couldn't find order %s. Double-check the ID on your confirmation email.", id)
	}
	return fmt.Sprintf("Order %s: %s. It's %s.", id, o.status, o.eta)
}

func messageText(m *a2a.Message) string {
	if m == nil {
		return ""
	}
	var parts []string
	for _, p := range m.Parts {
		if t := p.Text(); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, " ")
}

func homePage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := fmt.Fprint(w, homeHTML); err != nil {
		log.Printf("write home page: %v", err)
	}
}

const homeHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>Nimbus Roasters — small-batch coffee, shipped fresh</title>
  <meta name="description" content="Nimbus Roasters ships small-batch coffee fresh to your door. Track your order with our assistant.">
</head>
<body>
  <h1>Nimbus Roasters</h1>
  <p>Small-batch coffee, roasted to order and shipped the same week.</p>

  <h2>This week's roasts</h2>
  <ul>
    <li>Ethiopia Guji — bright, floral, stone fruit</li>
    <li>Colombia Huila — caramel, cocoa, balanced</li>
    <li>Sumatra Mandheling — earthy, full-bodied</li>
  </ul>

  <h2>Track your order</h2>
  <p>
    Already ordered? Our order assistant can tell you exactly where your bag is.
    Have your order ID ready (it looks like <code>NR-2041</code>).
  </p>
  <p><a href="/.well-known/agent-card.json">Order assistant &rarr;</a></p>

  <footer><p>&copy; Nimbus Roasters. Freshness guaranteed.</p></footer>
</body>
</html>
`
