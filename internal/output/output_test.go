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

package output

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

func TestPrintTable(t *testing.T) {
	t.Parallel()

	headers := []string{"NAME", "VERSION", "PATH"}

	tests := []struct {
		name string
		rows [][]string
		want string
	}{
		{
			name: "aligns headers and rows into columns",
			rows: [][]string{
				{"foo", "1.0.0", "/bin/foo"},
				{"barbar", "-", "/usr/local/bin/barbar"},
			},
			want: "NAME    VERSION  PATH\n" +
				"foo     1.0.0    /bin/foo\n" +
				"barbar  -        /usr/local/bin/barbar\n",
		},
		{
			name: "no rows writes only the header",
			rows: nil,
			want: "NAME  VERSION  PATH\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			p := NewPrinter(&buf, ModeText)
			if err := p.PrintTable(headers, tt.rows); err != nil {
				t.Fatalf("PrintTable() error = %v, want nil", err)
			}
			if diff := cmp.Diff(tt.want, buf.String()); diff != "" {
				t.Fatalf("PrintTable() wrong result (-want +got) diff = %s", diff)
			}
		})
	}
}

func TestMessageTextFileParts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		part *a2a.Part
		want string
	}{
		{
			name: "named and typed bytes",
			part: &a2a.Part{Content: a2a.Raw([]byte("sixteen bytes!!!")), Filename: "report.txt", MediaType: "text/plain"},
			want: "file: report.txt (text/plain, 16 B)",
		},
		{
			name: "unnamed bytes still show size",
			part: &a2a.Part{Content: a2a.Raw([]byte("sixteen bytes!!!"))},
			want: "file: (16 B)",
		},
		{
			name: "typed bytes without a name still show type and size",
			part: &a2a.Part{Content: a2a.Raw([]byte("sixteen bytes!!!")), MediaType: "application/octet-stream"},
			want: "file: (application/octet-stream, 16 B)",
		},
		{
			name: "url with name and type",
			part: &a2a.Part{Content: a2a.URL("https://example.com/report.pdf"), Filename: "report.pdf", MediaType: "application/pdf"},
			want: "file: report.pdf (application/pdf, https://example.com/report.pdf)",
		},
		{
			name: "url without name or type",
			part: &a2a.Part{Content: a2a.URL("https://example.com/blob")},
			want: "file: (https://example.com/blob)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := MessageText(&a2a.Message{Parts: a2a.ContentParts{tt.part}})
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("MessageText() wrong result (-want +got) diff = %s", diff)
			}
		})
	}
}

func TestPrintEventStreamsArtifactTextInline(t *testing.T) {
	t.Parallel()

	info := a2a.TaskInfo{TaskID: "t1", ContextID: "c1"}
	chunk := func(id a2a.ArtifactID, text string, appendChunk, last bool) *a2a.TaskArtifactUpdateEvent {
		e := a2a.NewArtifactUpdateEvent(info, id, a2a.NewTextPart(text))
		e.Append, e.LastChunk = appendChunk, last
		return e
	}

	tests := []struct {
		name   string
		events []a2a.Event
		want   string
	}{
		{
			name: "appended chunks continue the line until a status update",
			events: []a2a.Event{
				chunk("a1", "Hel", false, false),
				chunk("a1", "lo wor", true, false),
				chunk("a1", "ld", true, false),
				a2a.NewStatusUpdateEvent(info, a2a.TaskStateCompleted, nil),
			},
			want: "[artifact] Hello world\n[status] completed\n",
		},
		{
			name: "last chunk ends the line without doubling a trailing newline",
			events: []a2a.Event{
				chunk("a1", "line one\n", false, false),
				chunk("a1", "line two\n", true, true),
				a2a.NewStatusUpdateEvent(info, a2a.TaskStateCompleted, nil),
			},
			want: "[artifact] line one\nline two\n[status] completed\n",
		},
		{
			name: "a new artifact starts a new line",
			events: []a2a.Event{
				chunk("a1", "first", false, false),
				chunk("a2", "second", false, false),
			},
			want: "[artifact] first\n[artifact] second\n",
		},
		{
			name: "an append to another artifact is labeled",
			events: []a2a.Event{
				chunk("a1", "first", false, false),
				chunk("a2", " more", true, false),
			},
			want: "[artifact] first\n[artifact+]  more\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			p := NewPrinter(&buf, ModeText)
			for _, e := range tt.events {
				if err := p.PrintEvent(e); err != nil {
					t.Fatalf("PrintEvent() error = %v, want nil", err)
				}
			}
			if err := p.Flush(); err != nil {
				t.Fatalf("Flush() error = %v, want nil", err)
			}
			if diff := cmp.Diff(tt.want, buf.String()); diff != "" {
				t.Fatalf("PrintEvent() wrong result (-want +got) diff = %s", diff)
			}
		})
	}
}

func TestTaskArtifactTextConcatenatesChunks(t *testing.T) {
	t.Parallel()

	task := &a2a.Task{
		ID:     "t1",
		Status: a2a.TaskStatus{State: a2a.TaskStateCompleted},
		Artifacts: []*a2a.Artifact{{
			ID:   "a1",
			Name: "response",
			Parts: a2a.ContentParts{
				a2a.NewTextPart("Hel"),
				a2a.NewTextPart("lo"),
				a2a.NewDataPart(map[string]any{"k": "v"}),
			},
		}},
	}
	var buf bytes.Buffer
	if err := NewPrinter(&buf, ModeText).PrintEvent(task); err != nil {
		t.Fatalf("PrintEvent() error = %v, want nil", err)
	}
	want := "Task:     t1\nStatus:   completed\nArtifacts:\n  [response] Hello {\"k\":\"v\"}\n"
	if diff := cmp.Diff(want, buf.String()); diff != "" {
		t.Fatalf("PrintEvent() wrong result (-want +got) diff = %s", diff)
	}
}
