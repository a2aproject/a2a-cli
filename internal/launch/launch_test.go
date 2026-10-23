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

package launch

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestReadFrom(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		content    string
		offset     int64
		wantLines  []string
		wantOffset int64
	}{
		{name: "from start", content: "a\nb\n", wantLines: []string{"a", "b"}, wantOffset: 4},
		{name: "after offset", content: "a\nb\n", offset: 2, wantLines: []string{"b"}, wantOffset: 4},
		{name: "partial line", content: "a\nb", wantLines: []string{"a"}, wantOffset: 2},
		{name: "replaced file", content: "new\n", offset: 100, wantLines: []string{"new"}, wantOffset: 4},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "server.log")
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}
			lines, offset, err := readFrom(path, tc.offset)
			if err != nil {
				t.Fatalf("readFrom() error = %v", err)
			}
			if diff := cmp.Diff(tc.wantLines, lines); diff != "" {
				t.Fatalf("readFrom() wrong result (-want +got) diff = %s", diff)
			}
			if offset != tc.wantOffset {
				t.Fatalf("readFrom() offset = %d, want %d", offset, tc.wantOffset)
			}
		})
	}
}
