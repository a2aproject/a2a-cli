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

package oauthstore

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/a2aproject/a2a-go/v2/a2aclient/oauth"
)

func TestClientsFlagSetConfig(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		raw     any
		want    []oauth.Registration
		wantErr bool
	}{
		{
			name: "json string",
			raw:  `[{"authority":"https://login.example.com","client_id":"a"},{"authority":"https://auth.example.org","client_id":"b"}]`,
			want: []oauth.Registration{{Authority: "https://login.example.com", ClientID: "a"}, {Authority: "https://auth.example.org", ClientID: "b"}},
		},
		{
			name: "entries",
			raw: []any{
				map[string]any{"authority": "https://login.example.com", "client_id": "a", "scopes": []any{"x", "y"}},
				map[string]any{"authority": "https://auth.example.org", "client_id": DefaultClientID},
			},
			want: []oauth.Registration{
				{Authority: "https://login.example.com", ClientID: "a", Scopes: []string{"x", "y"}},
				{Authority: "https://auth.example.org", ClientID: DefaultClientID},
			},
		},
		{
			name: "omit_resource",
			raw:  []any{map[string]any{"authority": "https://login.microsoftonline.com/t/v2.0", "client_id": "a", "omit_resource": true}},
			want: []oauth.Registration{{Authority: "https://login.microsoftonline.com/t/v2.0", ClientID: "a", OmitResource: true}},
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
		`{"authority":"https://login.microsoftonline.com/t/v2.0","client_id":"c","omit_resource":true}`,
	} {
		if err := f.Set(v); err != nil {
			t.Fatalf("ClientsFlag.Set(%q) error = %v", v, err)
		}
	}
	if got, want := f.String(), "https://login.example.com=a,https://auth.example.org=b,https://login.microsoftonline.com/t/v2.0=c"; got != want {
		t.Fatalf("ClientsFlag.String() = %q, want %q", got, want)
	}
	want := []string{
		`{"authority":"https://login.example.com","client_id":"a"}`,
		`{"authority":"https://auth.example.org","client_id":"b","scopes":["x"]}`,
		`{"authority":"https://login.microsoftonline.com/t/v2.0","client_id":"c","omit_resource":true}`,
	}
	if diff := cmp.Diff(want, f.GetSlice()); diff != "" {
		t.Fatalf("ClientsFlag.GetSlice() wrong result (-want +got) diff = %s", diff)
	}
	for _, v := range []string{
		`{"authority":"https://login.example.com","client_id":"a","omit-resource":true}`,
		`[{"authority":"https://login.example.com","client_id":"a","scope":["x"]}]`,
	} {
		if err := f.Set(v); err == nil {
			t.Errorf("ClientsFlag.Set(%q) error = nil, want an error for the unknown field", v)
		}
	}
	if err := f.Set("not json"); err == nil {
		t.Fatalf("ClientsFlag.Set(%q) error = nil, want an error", "not json")
	}
}
