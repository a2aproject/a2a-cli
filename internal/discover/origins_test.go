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

package discover

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestExtractOrigins(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		texts []string
		want  []string
	}{
		{
			name:  "no urls",
			texts: []string{"just some prose"},
			want:  nil,
		},
		{
			name:  "dedupes by origin and keeps order",
			texts: []string{"see http://a.example/x and https://b.example:8080/y", "then http://A.example/z"},
			want:  []string{"http://a.example", "https://b.example:8080"},
		},
		{
			name:  "trims trailing punctuation",
			texts: []string{"visit https://c.example/page, or https://d.example."},
			want:  []string{"https://c.example", "https://d.example"},
		},
		{
			name:  "stops at markdown and quotes",
			texts: []string{"[link](https://e.example/x) and \"https://f.example\""},
			want:  []string{"https://e.example", "https://f.example"},
		},
		{
			name:  "ignores non-http schemes",
			texts: []string{"ftp://x.example mailto:a@b.example http://ok.example"},
			want:  []string{"http://ok.example"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ExtractOrigins(tc.texts...)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("ExtractOrigins() wrong result (-want +got) diff = %s", diff)
			}
		})
	}
}

func TestExtractOriginsFromJSONStrings(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{"url":"https:\/\/escaped.example\/a","results":[{"link":"https://nested.example/b\nnext line"}]}`)

	got := ExtractOrigins(jsonStrings(raw)...)

	want := []string{"https://escaped.example", "https://nested.example"}
	if diff := cmp.Diff(want, got, sortStrings); diff != "" {
		t.Fatalf("ExtractOrigins(jsonStrings()) wrong result (-want +got) diff = %s", diff)
	}
}
