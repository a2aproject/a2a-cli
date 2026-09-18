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

package fileparts

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/google/go-cmp/cmp"
)

func TestSaverSave(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		initialDir map[string]string
		events     []a2a.Event
		wantDir    map[string]string
	}{
		{
			name: "message raw part saved under its filename",
			events: []a2a.Event{
				&a2a.Message{ID: "m1", Parts: a2a.ContentParts{rawPart("hello", "note.txt", "text/plain")}},
			},
			wantDir: map[string]string{"note.txt": "hello"},
		},
		{
			name: "artifact name is the fallback and extension is inferred",
			events: []a2a.Event{
				&a2a.Task{Artifacts: []*a2a.Artifact{
					{ID: "a1", Name: "chart", Parts: a2a.ContentParts{rawPart("PNGBYTES", "", "image/png")}},
				}},
			},
			wantDir: map[string]string{"chart.png": "PNGBYTES"},
		},
		{
			name: "parts sharing a name in one owner are concatenated",
			events: []a2a.Event{
				&a2a.TaskArtifactUpdateEvent{Artifact: &a2a.Artifact{ID: "a1", Parts: a2a.ContentParts{
					rawPart("hello ", "doc.txt", ""),
					rawPart("world", "doc.txt", ""),
				}}},
			},
			wantDir: map[string]string{"doc.txt": "hello world"},
		},
		{
			name: "chunks of the same artifact are appended when append is set",
			events: []a2a.Event{
				&a2a.TaskArtifactUpdateEvent{Artifact: &a2a.Artifact{ID: "a1", Parts: a2a.ContentParts{rawPart("hello ", "doc.txt", "")}}},
				&a2a.TaskArtifactUpdateEvent{Append: true, Artifact: &a2a.Artifact{ID: "a1", Parts: a2a.ContentParts{rawPart("world", "doc.txt", "")}}},
			},
			wantDir: map[string]string{"doc.txt": "hello world"},
		},
		{
			name: "same artifact overwrites when append is not set",
			events: []a2a.Event{
				&a2a.TaskArtifactUpdateEvent{Artifact: &a2a.Artifact{ID: "a1", Parts: a2a.ContentParts{rawPart("first", "doc.txt", "")}}},
				&a2a.TaskArtifactUpdateEvent{Artifact: &a2a.Artifact{ID: "a1", Parts: a2a.ContentParts{rawPart("second", "doc.txt", "")}}},
			},
			wantDir: map[string]string{"doc.txt": "second"},
		},
		{
			name: "different artifacts with the same name are kept side by side",
			events: []a2a.Event{
				&a2a.TaskArtifactUpdateEvent{Artifact: &a2a.Artifact{ID: "a1", Parts: a2a.ContentParts{rawPart("v1", "report.txt", "")}}},
				&a2a.TaskArtifactUpdateEvent{Artifact: &a2a.Artifact{ID: "a2", Parts: a2a.ContentParts{rawPart("v2", "report.txt", "")}}},
			},
			wantDir: map[string]string{"report.txt": "v1", "report (1).txt": "v2"},
		},
		{
			name:       "pre-existing file is overwritten on first write",
			initialDir: map[string]string{"doc.txt": "stale"},
			events: []a2a.Event{
				&a2a.Message{ID: "m1", Parts: a2a.ContentParts{rawPart("fresh", "doc.txt", "")}},
			},
			wantDir: map[string]string{"doc.txt": "fresh"},
		},
		{
			name: "directory components in a filename are stripped",
			events: []a2a.Event{
				&a2a.Message{ID: "m1", Parts: a2a.ContentParts{rawPart("data", "../../evil.txt", "")}},
			},
			wantDir: map[string]string{"evil.txt": "data"},
		},
		{
			name: "non-raw parts are ignored",
			events: []a2a.Event{
				&a2a.Message{ID: "m1", Parts: a2a.ContentParts{
					a2a.NewTextPart("ignore me"),
					rawPart("keep", "raw.bin", ""),
				}},
			},
			wantDir: map[string]string{"raw.bin": "keep"},
		},
		{
			name: "status update message parts are saved under the fallback name",
			events: []a2a.Event{
				&a2a.TaskStatusUpdateEvent{Status: a2a.TaskStatus{
					Message: &a2a.Message{ID: "m1", Parts: a2a.ContentParts{rawPart("progress", "", "text/plain")}},
				}},
			},
			wantDir: map[string]string{"status-msg.txt": "progress"},
		},
		{
			name: "task artifacts and status message are all saved",
			events: []a2a.Event{
				&a2a.Task{
					Artifacts: []*a2a.Artifact{
						{ID: "a1", Name: "data.json", Parts: a2a.ContentParts{rawPart("{}", "", "application/json")}},
					},
					Status: a2a.TaskStatus{
						Message: &a2a.Message{ID: "m1", Parts: a2a.ContentParts{rawPart("done", "summary.txt", "")}},
					},
				},
			},
			wantDir: map[string]string{"data.json": "{}", "summary.txt": "done"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			for name, content := range tt.initialDir {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
					t.Fatalf("os.WriteFile(%q) error = %v", name, err)
				}
			}

			saver := NewSaver(dir)
			for i, e := range tt.events {
				if _, err := saver.Save(e); err != nil {
					t.Fatalf("saver.Save(event %d) error = %v", i, err)
				}
			}

			if diff := cmp.Diff(tt.wantDir, readDir(t, dir)); diff != "" {
				t.Fatalf("saver.Save() wrong directory (-want +got) diff = %s", diff)
			}
		})
	}
}

func TestSaverSaveReportsWrites(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	saver := NewSaver(dir)

	create, err := saver.Save(&a2a.TaskArtifactUpdateEvent{Artifact: &a2a.Artifact{ID: "a1", Parts: a2a.ContentParts{rawPart("hello ", "doc.txt", "")}}})
	if err != nil {
		t.Fatalf("saver.Save(create) error = %v", err)
	}
	appended, err := saver.Save(&a2a.TaskArtifactUpdateEvent{Append: true, Artifact: &a2a.Artifact{ID: "a1", Parts: a2a.ContentParts{rawPart("world", "doc.txt", "")}}})
	if err != nil {
		t.Fatalf("saver.Save(append) error = %v", err)
	}

	wantCreate := []Write{{Path: filepath.Join(dir, "doc.txt"), Bytes: 6, Written: true}}
	if diff := cmp.Diff(wantCreate, create); diff != "" {
		t.Fatalf("saver.Save(create) wrong result (-want +got) diff = %s", diff)
	}
	wantAppend := []Write{{Path: filepath.Join(dir, "doc.txt"), Bytes: 5, Written: false}}
	if diff := cmp.Diff(wantAppend, appended); diff != "" {
		t.Fatalf("saver.Save(append) wrong result (-want +got) diff = %s", diff)
	}
}

func TestSaverSaveOrder(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	saver := NewSaver(dir)

	saved, err := saver.Save(&a2a.Message{ID: "m1", Parts: a2a.ContentParts{
		rawPart("b", "b.txt", ""),
		rawPart("a", "a.txt", ""),
		rawPart("c", "c.txt", ""),
	}})
	if err != nil {
		t.Fatalf("saver.Save() error = %v", err)
	}

	want := []Write{
		{Path: filepath.Join(dir, "b.txt"), Bytes: 1, Written: true},
		{Path: filepath.Join(dir, "a.txt"), Bytes: 1, Written: true},
		{Path: filepath.Join(dir, "c.txt"), Bytes: 1, Written: true},
	}
	if diff := cmp.Diff(want, saved); diff != "" {
		t.Fatalf("saver.Save() wrong result (-want +got) diff = %s", diff)
	}
}

func TestResolveName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		part     *a2a.Part
		fallback string
		want     string
	}{
		{
			name: "filename with extension is kept as is",
			part: rawPart("x", "doc.pdf", "application/pdf"),
			want: "doc.pdf",
		},
		{
			name: "extension inferred from media type",
			part: rawPart("x", "chart", "image/png"),
			want: "chart.png",
		},
		{
			name: "media type parameters are ignored when inferring",
			part: rawPart("x", "data", "application/json; charset=utf-8"),
			want: "data.json",
		},
		{
			name: "media type is matched case-insensitively",
			part: rawPart("x", "img", "IMAGE/PNG"),
			want: "img.png",
		},
		{
			name: "unknown media type leaves the name without extension",
			part: rawPart("x", "blob", "application/x-custom"),
			want: "blob",
		},
		{
			name:     "fallback is used when the filename is empty",
			part:     rawPart("x", "", "text/plain"),
			fallback: "greeting",
			want:     "greeting.txt",
		},
		{
			name: "filepart is used when neither filename nor fallback is set",
			part: rawPart("x", "", ""),
			want: "filepart",
		},
		{
			name:     "dot filename falls back",
			part:     rawPart("x", ".", ""),
			fallback: "notes.txt",
			want:     "notes.txt",
		},
		{
			name: "parent directory filename does not escape",
			part: rawPart("x", "..", ""),
			want: "filepart",
		},
		{
			name: "directory components are stripped from the filename",
			part: rawPart("x", "../../etc/passwd", ""),
			want: "passwd",
		},
		{
			name: "surrounding whitespace is trimmed",
			part: rawPart("x", "  spaced.txt  ", ""),
			want: "spaced.txt",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := resolveName(tt.part, tt.fallback); got != tt.want {
				t.Fatalf("resolveName(%v, %q) = %q, want %q", tt.part, tt.fallback, got, tt.want)
			}
		})
	}
}

func readDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	got := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		got[filepath.Base(path)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatalf("filepath.WalkDir(%q) error = %v", dir, err)
	}
	return got
}

func rawPart(data, filename, mediaType string) *a2a.Part {
	p := a2a.NewRawPart([]byte(data))
	p.Filename = filename
	p.MediaType = mediaType
	return p
}
