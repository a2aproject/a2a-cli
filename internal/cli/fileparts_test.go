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
	"context"
	"iter"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

func TestSendSaveFileParts(t *testing.T) {
	t.Parallel()

	type fileContent struct {
		name    string
		content string
	}

	testCases := []struct {
		name       string
		makeCMD    func(t *testing.T, url string, dir string) []string
		executor   *filePartExecutor
		wantOutput string
		want       fileContent
	}{
		{
			name: "non-streaming saves the artifact file",
			executor: &filePartExecutor{
				artifactID: "a1",
				filename:   "greeting.txt",
				mediaType:  "text/plain",
				chunks:     []string{"hello world"},
			},
			makeCMD: func(t *testing.T, url, dir string) []string {
				return []string{"send", "-a", url, "--save-fileparts", dir, "trigger"}
			},
			want: fileContent{name: "greeting.txt", content: "hello world"},
		},
		{
			name: "streaming appends the artifact chunks",
			executor: &filePartExecutor{
				artifactID: "a1",
				filename:   "story.txt",
				mediaType:  "text/plain",
				chunks:     []string{"hello ", "world"},
			},
			makeCMD: func(t *testing.T, url, dir string) []string {
				return []string{"send", "-a", url, "--stream", "--save-fileparts", dir, "trigger"}
			},
			want: fileContent{name: "story.txt", content: "hello world"},
		},
		{
			name: "extension inferred and creation reported when verbose",
			executor: &filePartExecutor{
				artifactID: "a1",
				filename:   "chart",
				mediaType:  "image/png",
				chunks:     []string{"PNGBYTES"},
			},
			makeCMD: func(t *testing.T, url, dir string) []string {
				return []string{"send", "-a", url, "-v", "--save-fileparts", dir, "trigger"}
			},
			want:       fileContent{name: "chart.png", content: "PNGBYTES"},
			wantOutput: "chart.png",
		},
		{
			name: "get saves file parts",
			executor: &filePartExecutor{
				artifactID: "a1",
				filename:   "greeting.txt",
				mediaType:  "text/plain",
				chunks:     []string{"hello from task"},
			},
			makeCMD: func(t *testing.T, url, dir string) []string {
				taskID := sendTestMessage(t, url, "trigger")
				return []string{"task", "get", "-a", url, string(taskID), "--save-fileparts", dir}
			},
			want: fileContent{name: "greeting.txt", content: "hello from task"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			url := startTestServerWith(t, a2a.AgentCapabilities{Streaming: true}, tc.executor)
			cmd := tc.makeCMD(t, url, dir)

			_, stderr, err := runCMDCapturingStderr(t, cmd...)
			if err != nil {
				t.Fatalf("runCMDCapturingStderr() error = %v", err)
			}

			got, err := os.ReadFile(filepath.Join(dir, tc.want.name))
			if err != nil {
				t.Fatalf("os.ReadFile(%q) error = %v", tc.want.name, err)
			}
			if string(got) != tc.want.content {
				t.Fatalf("file %q content = %q, want %q", tc.want.name, string(got), tc.want.content)
			}
			if !strings.Contains(stderr, tc.wantOutput) {
				t.Fatalf("stderr = %q, want it to contain %q", stderr, tc.wantOutput)
			}
		})
	}

}

type filePartExecutor struct {
	a2asrv.AgentExecutor
	artifactID a2a.ArtifactID
	filename   string
	mediaType  string
	chunks     []string
}

func (e *filePartExecutor) Execute(_ context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		if execCtx.StoredTask == nil {
			if !yield(a2a.NewSubmittedTask(execCtx, execCtx.Message), nil) {
				return
			}
		}
		if !yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateWorking, nil), nil) {
			return
		}
		for i, chunk := range e.chunks {
			part := a2a.NewRawPart([]byte(chunk))
			part.MediaType = e.mediaType
			part.Filename = e.filename

			var evt *a2a.TaskArtifactUpdateEvent
			if i == 0 {
				evt = a2a.NewArtifactEvent(execCtx, part)
				evt.Artifact.ID = e.artifactID
			} else {
				evt = a2a.NewArtifactUpdateEvent(execCtx, e.artifactID, part)
				evt.Append = true
			}
			evt.LastChunk = i == len(e.chunks)-1
			if !yield(evt, nil) {
				return
			}
		}
		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCompleted, nil), nil)
	}
}
