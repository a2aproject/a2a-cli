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
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestWriteSection(t *testing.T) {
	t.Parallel()
	section := sectionBegin + "\nnew\n" + sectionEnd + "\n"
	tests := []struct {
		name     string
		existing string
		want     string
	}{
		{name: "new file", want: section},
		{name: "keeps the user's text", existing: "# Mine\n", want: "# Mine\n\n" + section},
		{name: "replaces the section", existing: "# Mine\n\n" + sectionBegin + "\nold\n" + sectionEnd + "\n\n# After\n", want: "# Mine\n\n" + section + "\n# After\n"},
		{name: "section without an end", existing: sectionBegin + "\nold\n", want: section},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "dir", "AGENTS.md")
			if tc.existing != "" {
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(tc.existing), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := writeSection(path, "new\n"); err != nil {
				t.Fatalf("writeSection() error = %v", err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tc.want, string(got)); diff != "" {
				t.Fatalf("writeSection() wrong result (-want +got) diff = %s", diff)
			}
		})
	}
}
