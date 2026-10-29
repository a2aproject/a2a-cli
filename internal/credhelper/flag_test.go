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

package credhelper

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestFlagSetConfig(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		raw     any
		want    []Helper
		wantErr bool
	}{
		{
			name: "path string",
			raw:  "example.com=/usr/bin/helper",
			want: []Helper{{Spec: "example.com=/usr/bin/helper", Scope: "example.com", Command: []string{"/usr/bin/helper"}}},
		},
		{
			name: "entries",
			raw: []any{
				map[string]any{"command": []any{"helper", "--flag"}},
				map[string]any{"scope": "*.Example.com", "command": []any{"corp-helper"}},
			},
			want: []Helper{
				{Spec: "helper --flag", Command: []string{"helper", "--flag"}},
				{Spec: "*.Example.com=corp-helper", Scope: "*.example.com", Command: []string{"corp-helper"}},
			},
		},
		{name: "string entry", raw: []any{"/usr/bin/helper"}, wantErr: true},
		{name: "unknown field", raw: []any{map[string]any{"cmd": []any{"helper"}}}, wantErr: true},
		{name: "no command", raw: []any{map[string]any{"scope": "example.com"}}, wantErr: true},
		{name: "scope with a port", raw: []any{map[string]any{"scope": "example.com:443", "command": []any{"helper"}}}, wantErr: true},
		{name: "scope with a path", raw: []any{map[string]any{"scope": "example.com/a2a", "command": []any{"helper"}}}, wantErr: true},
		{name: "map", raw: map[string]any{"command": []any{"helper"}}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var f Flag
			err := f.SetConfig(tc.raw)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Flag.SetConfig(%v) error = %v, want error %v", tc.raw, err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if diff := cmp.Diff(tc.want, f.Helpers()); diff != "" {
				t.Fatalf("Flag.SetConfig(%v) wrong result (-want +got) diff = %s", tc.raw, diff)
			}
		})
	}
}
