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
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

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
