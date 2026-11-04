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
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseRegistrations(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		values  []string
		want    []Registration
		wantErr bool
	}{
		{
			name:   "object",
			values: []string{`{"authority":"https://login.example.com","client_id":"0f1e2d3c","scopes":["a2a.invoke"]}`},
			want:   []Registration{{Authority: "https://login.example.com", ClientID: "0f1e2d3c", Scopes: []string{"a2a.invoke"}}},
		},
		{
			name: "array and object",
			values: []string{
				`[{"authority":"https://login.example.com","client_id":"a"},{"authority":"https://auth.example.org","client_id":"https://a2a-protocol.org/oauth/a2a-cli.json"}]`,
				`{"authority":"https://idp.example.net","client_id":"c"}`,
			},
			want: []Registration{
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

func TestSelectClient(t *testing.T) {
	t.Parallel()
	regs := []Registration{
		{Authority: "https://login.example.com/", ClientID: "configured"},
		{Authority: "https://dup.example.com", ClientID: "one"},
		{Authority: "https://dup.example.com", ClientID: "two"},
	}
	tests := []struct {
		name      string
		endpoints Endpoints
		noDefault bool
		want      ClientChoice
		wantErr   bool
		noClient  bool
	}{
		{
			name:      "configured registration, trailing slash ignored",
			endpoints: Endpoints{Issuer: "https://login.example.com", CIMDSupported: true},
			want:      ClientChoice{Registration: regs[0], Source: ClientSourceConfig},
		},
		{
			name:      "CIMD fallback",
			endpoints: Endpoints{Issuer: "https://other.example.com", CIMDSupported: true},
			want:      ClientChoice{Registration: Registration{Authority: "https://other.example.com", ClientID: DefaultClientID}, Source: ClientSourceCIMD},
		},
		{
			name:      "no client",
			endpoints: Endpoints{Issuer: "https://other.example.com"},
			wantErr:   true,
			noClient:  true,
		},
		{
			name:      "CIMD fallback disabled",
			endpoints: Endpoints{Issuer: "https://other.example.com", CIMDSupported: true},
			noDefault: true,
			wantErr:   true,
			noClient:  true,
		},
		{
			name:      "ambiguous",
			endpoints: Endpoints{Issuer: "https://dup.example.com"},
			wantErr:   true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			defaultID := DefaultClientID
			if tc.noDefault {
				defaultID = ""
			}
			got, err := selectClient(regs, defaultID, tc.endpoints)
			if (err != nil) != tc.wantErr {
				t.Fatalf("selectClient() error = %v, want error %v", err, tc.wantErr)
			}
			var nc *NoClientError
			if errors.As(err, &nc) != tc.noClient {
				t.Fatalf("selectClient() error = %v, want NoClientError %v", err, tc.noClient)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("selectClient() wrong result (-want +got) diff = %s", diff)
			}
		})
	}
}
