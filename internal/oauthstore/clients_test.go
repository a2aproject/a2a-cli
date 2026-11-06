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
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/a2aproject/a2a-go/v2/a2aclient/oauth"
)

func TestParseRegistrations(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		values  []string
		want    []oauth.Registration
		wantErr bool
	}{
		{
			name:   "object",
			values: []string{`{"authority":"https://login.example.com","client_id":"0f1e2d3c","scopes":["a2a.invoke"]}`},
			want:   []oauth.Registration{{Authority: "https://login.example.com", ClientID: "0f1e2d3c", Scopes: []string{"a2a.invoke"}}},
		},
		{
			name: "array and object",
			values: []string{
				`[{"authority":"https://login.example.com","client_id":"a"},{"authority":"https://auth.example.org","client_id":"https://a2a-protocol.org/oauth/a2a-cli.json"}]`,
				`{"authority":"https://idp.example.net","client_id":"c"}`,
			},
			want: []oauth.Registration{
				{Authority: "https://login.example.com", ClientID: "a"},
				{Authority: "https://auth.example.org", ClientID: "https://a2a-protocol.org/oauth/a2a-cli.json"},
				{Authority: "https://idp.example.net", ClientID: "c"},
			},
		},
		{name: "missing client_id", values: []string{`{"authority":"https://login.example.com"}`}, wantErr: true},
		{name: "plain http authority", values: []string{`{"authority":"http://login.example.com","client_id":"a"}`}, wantErr: true},
		{name: "not json", values: []string{`login.example.com=a`}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseRegistrations(tc.values)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ParseRegistrations() error = %v, want error %v", err, tc.wantErr)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("ParseRegistrations() wrong result (-want +got) diff = %s", diff)
			}
		})
	}
}

func TestNoClientMessage(t *testing.T) {
	t.Parallel()
	got, ok := NoClientMessage(fmt.Errorf("discovering: %w", &oauth.NoClientError{Issuers: []string{"https://login.example.com"}}))
	want := `no client in oauth-clients matches the authorization server https://login.example.com; add {"authority":"https://login.example.com","client_id":"<client id or CIMD URL>"} to oauth-clients`
	if !ok || got != want {
		t.Fatalf("NoClientMessage() = %q, %v, want %q, true", got, ok, want)
	}
	if _, ok := NoClientMessage(errors.New("other")); ok {
		t.Fatal("NoClientMessage() of another error ok = true, want false")
	}
}
