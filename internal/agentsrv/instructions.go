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

package agentsrv

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"github.com/a2aproject/a2a-cli/skills"
)

// agentsLinePrefix starts the line added to every prompt with the agents the
// harness can call and the agents URL of the conversation.
const agentsLinePrefix = "[a2a agents]"

// instructionFiles are the global instruction files of the known harnesses,
// relative to the harness home. Each gets the same section.
var instructionFiles = []string{
	filepath.Join(".config", "opencode", "AGENTS.md"),
	filepath.Join(".claude", "CLAUDE.md"),
}

const (
	sectionBegin = "<!-- a2a-server:begin (managed by a2a server; edits are overwritten) -->"
	sectionEnd   = "<!-- a2a-server:end -->"
)

// AgentRef is an agent this agent can call.
type AgentRef struct {
	Name        string
	Kind        string
	Description string
}

// callableAgents returns the contacts and hosted nodes.
func (p *Peers) callableAgents(ctx context.Context) ([]AgentRef, error) {
	contacts, err := p.s.store.Contacts(ctx)
	if err != nil {
		return nil, err
	}
	nodes, err := p.s.store.Nodes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AgentRef, 0, len(contacts)+len(nodes))
	for _, c := range contacts {
		out = append(out, AgentRef{Name: c.Name, Kind: "contact"})
	}
	for _, n := range nodes {
		out = append(out, AgentRef{Name: n.ID, Kind: "webhook node", Description: n.Description})
	}
	return out, nil
}

// writeInstructions puts the a2a-agents skill and the agents the harness can
// call into the global instruction files in the harness home. A local
// harness uses your own home, which is left alone.
func (s *Server) writeInstructions(ctx context.Context) error {
	if s.config().Sandbox == SandboxLocal {
		return nil
	}
	agents, err := s.peers.callableAgents(ctx)
	if err != nil {
		return err
	}
	var sb strings.Builder
	sb.WriteString(strings.TrimSpace(skills.A2AAgents))
	sb.WriteString("\n\nThe agents you can call:\n\n")
	if len(agents) == 0 {
		sb.WriteString("- none yet\n")
	}
	for _, a := range agents {
		fmt.Fprintf(&sb, "- %s (%s)", a.Name, a.Kind)
		if a.Description != "" {
			sb.WriteString(": " + a.Description)
		}
		sb.WriteString("\n")
	}
	var errs []error
	for _, f := range instructionFiles {
		errs = append(errs, writeSection(filepath.Join(s.dir.HomeDir(), f), sb.String()))
	}
	return errors.Join(errs...)
}

// refreshInstructions rewrites the instruction files after the agents the
// harness can call changed.
func (s *Server) refreshInstructions(ctx context.Context) {
	if err := s.writeInstructions(ctx); err != nil {
		s.logf("! updating the harness instructions: %v", err)
	}
}

// writeSection replaces the managed section of the file at path, keeping
// whatever else the file holds.
func writeSection(path, body string) error {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	text := string(data)
	section := sectionBegin + "\n" + body + sectionEnd + "\n"
	if i := strings.Index(text, sectionBegin); i >= 0 {
		end := len(text)
		if j := strings.Index(text[i:], sectionEnd); j >= 0 {
			end = i + j + len(sectionEnd)
			end += len(text[end:]) - len(strings.TrimPrefix(text[end:], "\n"))
		}
		text = text[:i] + section + text[end:]
	} else {
		if text != "" && !strings.HasSuffix(text, "\n\n") {
			text = strings.TrimRight(text, "\n") + "\n\n"
		}
		text += section
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(text), 0o600)
}

// withAgentsLine returns execCtx with the agents line added before the
// caller's message, for a new prompt. Answers to the harness's own
// questions are left alone, because the harness parses them.
func (s *Server) withAgentsLine(ctx context.Context, execCtx *a2asrv.ExecutorContext) *a2asrv.ExecutorContext {
	if execCtx.Message == nil || (execCtx.StoredTask != nil && execCtx.StoredTask.Status.State == a2a.TaskStateInputRequired) {
		return execCtx
	}
	agents, err := s.peers.callableAgents(ctx)
	if err != nil || len(agents) == 0 {
		return execCtx
	}
	claims := callClaims{ContextID: execCtx.ContextID}
	if p, ok := principalOf(ctx); ok {
		claims.OnBehalfOf, claims.Hops = p.User(), p.Hops
	} else if execCtx.User != nil {
		claims.OnBehalfOf = execCtx.User.Name
	}
	u, err := s.peers.agentsURL(ctx, claims)
	if err != nil {
		return execCtx
	}
	names := make([]string, len(agents))
	for i, a := range agents {
		names[i] = a.Name
	}
	line := fmt.Sprintf("%s Agents URL for this conversation: %s (agents: %s; call one with a2a send -a %s/<name>)",
		agentsLinePrefix, u, strings.Join(names, ", "), u)
	ec := *execCtx
	msg := *execCtx.Message
	msg.Parts = append([]*a2a.Part{a2a.NewTextPart(line)}, msg.Parts...)
	ec.Message = &msg
	return &ec
}
