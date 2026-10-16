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

// Package output renders agent cards, tasks, messages and streaming events as
// either indented JSON or human-readable text.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

// Mode selects how a Printer renders values.
type Mode string

// ModeJson renders values as indented JSON: a single indented document, or one
// indented record per streamed event.
const ModeJson Mode = "json"

// ModeJSONL renders values as JSON Lines: one compact JSON object per line.
const ModeJSONL Mode = "jsonl"

// ModeText renders values as human-readable text.
const ModeText Mode = "text"

var taskStateNames = map[a2a.TaskState]string{
	a2a.TaskStateSubmitted:     "submitted",
	a2a.TaskStateWorking:       "working",
	a2a.TaskStateCompleted:     "completed",
	a2a.TaskStateFailed:        "failed",
	a2a.TaskStateCanceled:      "canceled",
	a2a.TaskStateRejected:      "rejected",
	a2a.TaskStateInputRequired: "input-required",
	a2a.TaskStateAuthRequired:  "auth-required",
}

// Printer writes A2A values to an output writer in the configured Mode.
type Printer struct {
	Out  io.Writer
	Mode Mode

	// openArtifact is the artifact whose streamed text is being written inline
	// in text mode; its line is not terminated yet.
	openArtifact a2a.ArtifactID
	// atLineStart reports whether the inline text written so far ends with a newline.
	atLineStart bool
}

// NewPrinter returns a Printer that writes to out using the given Mode.
func NewPrinter(out io.Writer, mode Mode) *Printer {
	return &Printer{Out: out, Mode: mode}
}

// IsJSON reports whether the printer renders machine-readable JSON, either
// indented (ModeJson) or one compact object per line (ModeJSONL).
func (p *Printer) IsJSON() bool {
	return p.Mode == ModeJson || p.Mode == ModeJSONL
}

// jsonEncoder returns a JSON encoder configured for the current mode: indented
// for ModeJson, compact for ModeJSONL.
func (p *Printer) jsonEncoder() *json.Encoder {
	enc := json.NewEncoder(p.Out)
	enc.SetEscapeHTML(false)
	if p.Mode == ModeJson {
		enc.SetIndent("", "  ")
	}
	return enc
}

// PrintJSON writes v as a single JSON document, indented in ModeJson and
// compact in ModeJSONL.
func (p *Printer) PrintJSON(v any) error {
	return p.jsonEncoder().Encode(v)
}

// PrintTable writes rows as a column-aligned text table under headers.
func (p *Printer) PrintTable(headers []string, rows [][]string) error {
	tw := tabwriter.NewWriter(p.Out, 0, 4, 2, ' ', 0)
	if _, err := io.WriteString(tw, strings.Join(headers, "\t")+"\n"); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := io.WriteString(tw, strings.Join(row, "\t")+"\n"); err != nil {
			return err
		}
	}
	return tw.Flush()
}

// PrintCard writes an agent card in the configured Mode.
func (p *Printer) PrintCard(card *a2a.AgentCard) error {
	if p.IsJSON() {
		return p.PrintJSON(card)
	}
	_, err := io.WriteString(p.Out, formatCard(card))
	return err
}

// PrintEvent writes a streaming event in the configured Mode. In ModeJson each
// event is an indented record; in ModeJSONL each event is one compact object per
// line.
func (p *Printer) PrintEvent(event a2a.Event) error {
	if p.IsJSON() {
		return p.jsonEncoder().Encode(a2a.StreamResponse{Event: event})
	}

	if e, ok := event.(*a2a.TaskArtifactUpdateEvent); ok {
		return p.printArtifactChunk(e)
	}
	if err := p.Flush(); err != nil {
		return err
	}

	var s string
	switch e := event.(type) {
	case *a2a.TaskStatusUpdateEvent:
		state := shortState(e.Status.State)
		if e.Status.Message != nil {
			s = fmt.Sprintf("[status] %s: %s\n", state, MessageText(e.Status.Message))
		} else {
			s = fmt.Sprintf("[status] %s\n", state)
		}
	case *a2a.Task:
		s = formatTask(e) + formatResumeHint(e)
	case *a2a.Message:
		s = formatMessage(e)
	default:
		return nil
	}
	_, err := io.WriteString(p.Out, s)
	return err
}

// printArtifactChunk writes streamed artifact text inline: a new artifact
// starts a "[artifact] " line, and chunks appended to it continue that line
// until the last chunk or another event arrives.
func (p *Printer) printArtifactChunk(e *a2a.TaskArtifactUpdateEvent) error {
	text := artifactText(e.Artifact.Parts)
	continues := e.Append && e.Artifact.ID == p.openArtifact
	if !continues {
		if err := p.Flush(); err != nil {
			return err
		}
		prefix := "[artifact] "
		if e.Append {
			prefix = "[artifact+] "
		}
		text = prefix + text
	}
	if _, err := io.WriteString(p.Out, text); err != nil {
		return err
	}
	p.openArtifact = e.Artifact.ID
	if text != "" {
		p.atLineStart = strings.HasSuffix(text, "\n")
	}
	if e.LastChunk {
		return p.Flush()
	}
	return nil
}

// Flush terminates the line of an artifact streamed inline in text mode, if
// any. Call it when a stream of events ends.
func (p *Printer) Flush() error {
	if p.openArtifact == "" {
		return nil
	}
	p.openArtifact = ""
	if p.atLineStart {
		return nil
	}
	_, err := io.WriteString(p.Out, "\n")
	return err
}

// PrintTaskList writes a list of tasks in the configured Mode.
func (p *Printer) PrintTaskList(resp *a2a.ListTasksResponse) error {
	if p.IsJSON() {
		return p.PrintJSON(resp)
	}
	_, err := io.WriteString(p.Out, formatTaskList(resp))
	return err
}

// PrintPushConfig writes a single push-notification configuration in the
// configured Mode. In text mode the auth credentials are redacted.
func (p *Printer) PrintPushConfig(pc *a2a.PushConfig) error {
	if p.IsJSON() {
		return p.PrintJSON(pc)
	}
	_, err := io.WriteString(p.Out, formatPushConfig(pc))
	return err
}

// PrintPushConfigList writes a list of push-notification configurations in the
// configured Mode.
func (p *Printer) PrintPushConfigList(configs []*a2a.PushConfig) error {
	if p.IsJSON() {
		return p.PrintJSON(configs)
	}
	tw := tabwriter.NewWriter(p.Out, 0, 4, 2, ' ', 0)
	if _, err := io.WriteString(tw, "ID\tTASK\tURL\n"); err != nil {
		return err
	}
	for _, pc := range configs {
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\n", pc.ID, pc.TaskID, pc.URL); err != nil {
			return err
		}
	}
	return tw.Flush()
}

// PrintPushConfigDeleted confirms deletion of a push-notification configuration.
func (p *Printer) PrintPushConfigDeleted(taskID, configID string) error {
	if p.IsJSON() {
		return p.PrintJSON(map[string]any{"deleted": true, "taskId": taskID, "id": configID})
	}
	_, err := fmt.Fprintf(p.Out, "Deleted:  %s (task %s)\n", configID, taskID)
	return err
}

func formatPushConfig(pc *a2a.PushConfig) string {
	var sb strings.Builder
	if pc.ID != "" {
		fmt.Fprintf(&sb, "Config:   %s\n", pc.ID)
	}
	fmt.Fprintf(&sb, "Task:     %s\n", pc.TaskID)
	fmt.Fprintf(&sb, "URL:      %s\n", pc.URL)
	if pc.Token != "" {
		fmt.Fprintf(&sb, "Token:    %s\n", pc.Token)
	}
	if pc.Auth != nil {
		fmt.Fprintf(&sb, "Auth:     %s", pc.Auth.Scheme)
		if pc.Auth.Credentials != "" {
			sb.WriteString(" <redacted>")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func formatCard(card *a2a.AgentCard) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Name:         %s\n", card.Name)
	if card.Description != "" {
		fmt.Fprintf(&sb, "Description:  %s\n", card.Description)
	}
	fmt.Fprintf(&sb, "Version:      %s\n", card.Version)

	if len(card.SupportedInterfaces) > 0 {
		sb.WriteString("Interfaces:\n")
		for _, iface := range card.SupportedInterfaces {
			sb.WriteString(formatInterface(iface))
		}
	}

	fmt.Fprintf(&sb, "Streaming:    %v\n", card.Capabilities.Streaming)

	if len(card.Skills) > 0 {
		sb.WriteString("Skills:\n")
		for _, s := range card.Skills {
			fmt.Fprintf(&sb, "  %-20s %s\n", s.ID, s.Name)
		}
	}

	return sb.String()
}

func formatInterface(iface *a2a.AgentInterface) string {
	binding := string(iface.ProtocolBinding)
	if iface.ProtocolVersion != "" {
		binding = fmt.Sprintf("%s (A2A v%s)", binding, iface.ProtocolVersion)
	}
	line := fmt.Sprintf("  %-22s %s", binding, iface.URL)
	if iface.Tenant != "" {
		line += fmt.Sprintf("  (tenant: %s)", iface.Tenant)
	}
	return line + "\n"
}

func formatTask(task *a2a.Task) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Task:     %s\n", task.ID)
	if task.ContextID != "" {
		fmt.Fprintf(&sb, "Context:  %s\n", task.ContextID)
	}
	fmt.Fprintf(&sb, "Status:   %s", shortState(task.Status.State))
	if task.Status.Timestamp != nil {
		fmt.Fprintf(&sb, " (%s)", task.Status.Timestamp.Format("2006-01-02T15:04:05Z07:00"))
	}
	fmt.Fprintf(&sb, "\n")
	if task.Status.Message != nil {
		fmt.Fprintf(&sb, "  %s\n", MessageText(task.Status.Message))
	}

	if len(task.Artifacts) > 0 {
		sb.WriteString("Artifacts:\n")
		for _, art := range task.Artifacts {
			label := string(art.ID)
			if art.Name != "" {
				label = art.Name
			}
			fmt.Fprintf(&sb, "  [%s] %s\n", label, artifactText(art.Parts))
		}
	}

	if len(task.History) > 0 {
		sb.WriteString("History:\n")
		for _, msg := range task.History {
			role := "user"
			if msg.Role == a2a.MessageRoleAgent {
				role = "agent"
			}
			fmt.Fprintf(&sb, "  [%s] %s\n", role, MessageText(msg))
		}
	}

	return sb.String()
}

// formatResumeHint returns a copy-pasteable command to continue or reply to a task.
func formatResumeHint(task *a2a.Task) string {
	if task.ID == "" {
		return ""
	}
	if task.Status.State != a2a.TaskStateInputRequired && task.Status.State != a2a.TaskStateAuthRequired {
		return ""
	}
	return fmt.Sprintf("\n\nResume:     a2a send --task-id %s %q\n", task.ID, "<reply>")
}

func formatMessage(msg *a2a.Message) string {
	role := "user"
	if msg.Role == a2a.MessageRoleAgent {
		role = "agent"
	}
	return fmt.Sprintf("[%s] %s\n", role, MessageText(msg))
}

func formatTaskList(resp *a2a.ListTasksResponse) string {
	var sb strings.Builder
	tw := tabwriter.NewWriter(&sb, 0, 4, 2, ' ', 0)
	_, _ = io.WriteString(tw, "ID\tSTATUS\tCONTEXT\n")
	for _, t := range resp.Tasks {
		_, _ = io.WriteString(tw, fmt.Sprintf("%s\t%s\t%s\n", t.ID, shortState(t.Status.State), t.ContextID))
	}
	_ = tw.Flush()
	if resp.NextPageToken != "" {
		fmt.Fprintf(&sb, "\nNext page token: %s\n", resp.NextPageToken)
	}
	return sb.String()
}

// MessageText returns the concatenated textual representation of a message's parts.
func MessageText(msg *a2a.Message) string {
	return partsText(msg.Parts)
}

// partsText renders parts separated by spaces.
func partsText(parts a2a.ContentParts) string {
	return joinParts(parts, false)
}

// artifactText renders artifact parts. Adjacent text parts are concatenated
// as is, because streamed chunks appended to an artifact are pieces of one text.
func artifactText(parts a2a.ContentParts) string {
	return joinParts(parts, true)
}

func joinParts(parts a2a.ContentParts, concatText bool) string {
	var sb strings.Builder
	for i, p := range parts {
		if i > 0 && !(concatText && isText(parts[i-1]) && isText(p)) {
			sb.WriteString(" ")
		}
		sb.WriteString(partText(p))
	}
	return sb.String()
}

func isText(p *a2a.Part) bool {
	_, ok := p.Content.(a2a.Text)
	return ok
}

func partText(p *a2a.Part) string {
	if t := p.Text(); t != "" {
		return t
	}
	if p.URL() != "" || p.Raw() != nil {
		return filePartText(p)
	}
	if p.Data() != nil {
		b, err := json.Marshal(p.Data())
		if err != nil {
			return fmt.Sprintf("[data: %v]", err)
		}
		return string(b)
	}
	return ""
}

// filePartText renders a file part by name, media type and size or URL.
func filePartText(p *a2a.Part) string {
	var sb strings.Builder
	sb.WriteString("file:")
	if p.Filename != "" {
		sb.WriteString(" ")
		sb.WriteString(p.Filename)
	}
	var attrs []string
	if p.MediaType != "" {
		attrs = append(attrs, p.MediaType)
	}
	if raw := p.Raw(); raw != nil {
		attrs = append(attrs, formatSize(len(raw)))
	}
	if u := p.URL(); u != "" {
		attrs = append(attrs, string(u))
	}
	if len(attrs) > 0 {
		fmt.Fprintf(&sb, " (%s)", strings.Join(attrs, ", "))
	}
	return sb.String()
}

func formatSize(n int) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for size := int64(n) / unit; size >= unit; size /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func shortState(state a2a.TaskState) string {
	if name, ok := taskStateNames[state]; ok {
		return name
	}
	return string(state)
}
