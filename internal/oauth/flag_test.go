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

package oauth

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestClientsFlagSetConfig(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		raw     any
		want    []Registration
		wantErr bool
	}{
		{
			name: "json string",
			raw:  `[{"authority":"https://login.example.com","client_id":"a"},{"authority":"https://auth.example.org","client_id":"b"}]`,
			want: []Registration{{Authority: "https://login.example.com", ClientID: "a"}, {Authority: "https://auth.example.org", ClientID: "b"}},
		},
		{
			name: "entries",
			raw: []any{
				map[string]any{"authority": "https://login.example.com", "client_id": "a", "scopes": []any{"x", "y"}},
				map[string]any{"authority": "https://auth.example.org", "client_id": DefaultClientID},
			},
			want: []Registration{
				{Authority: "https://login.example.com", ClientID: "a", Scopes: []string{"x", "y"}},
				{Authority: "https://auth.example.org", ClientID: DefaultClientID},
			},
		},
		{name: "string entry", raw: []any{`{"authority":"https://login.example.com","client_id":"a"}`}, wantErr: true},
		{name: "unknown field", raw: []any{map[string]any{"authority": "https://login.example.com", "clientId": "a"}}, wantErr: true},
		{name: "no client_id", raw: []any{map[string]any{"authority": "https://login.example.com"}}, wantErr: true},
		{name: "plain http authority", raw: []any{map[string]any{"authority": "http://login.example.com", "client_id": "a"}}, wantErr: true},
		{name: "map", raw: map[string]any{"authority": "https://login.example.com", "client_id": "a"}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var f ClientsFlag
			err := f.SetConfig(tc.raw)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ClientsFlag.SetConfig(%v) error = %v, want error %v", tc.raw, err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if diff := cmp.Diff(tc.want, f.Registrations()); diff != "" {
				t.Fatalf("ClientsFlag.SetConfig(%v) wrong result (-want +got) diff = %s", tc.raw, diff)
			}
		})
	}
}

func TestClientsFlagSet(t *testing.T) {
	t.Parallel()
	var f ClientsFlag
	for _, v := range []string{
		`{"authority":"https://login.example.com","client_id":"a"}`,
		`[{"authority":"https://auth.example.org","client_id":"b","scopes":["x"]}]`,
	} {
		if err := f.Set(v); err != nil {
			t.Fatalf("ClientsFlag.Set(%q) error = %v", v, err)
		}
	}
	if got, want := f.String(), "https://login.example.com=a,https://auth.example.org=b"; got != want {
		t.Fatalf("ClientsFlag.String() = %q, want %q", got, want)
	}
	want := []string{
		`{"authority":"https://login.example.com","client_id":"a"}`,
		`{"authority":"https://auth.example.org","client_id":"b","scopes":["x"]}`,
	}
	if diff := cmp.Diff(want, f.GetSlice()); diff != "" {
		t.Fatalf("ClientsFlag.GetSlice() wrong result (-want +got) diff = %s", diff)
	}
	if err := f.Set("not json"); err == nil {
		t.Fatalf("ClientsFlag.Set(%q) error = nil, want an error", "not json")
	}
}
