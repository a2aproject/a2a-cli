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

package unixsocket

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		spec    string
		want    Spec
		wantErr bool
	}{
		{spec: "/tmp/a.sock", want: Spec{Path: "/tmp/a.sock"}},
		{spec: "agent.local=/tmp/a.sock", want: Spec{Host: "agent.local", Path: "/tmp/a.sock"}},
		{spec: "Agent.Local=/tmp/a.sock", want: Spec{Host: "agent.local", Path: "/tmp/a.sock"}},
		{spec: "/tmp/a=b.sock", want: Spec{Path: "/tmp/a=b.sock"}},
		{spec: "a.sock", want: Spec{Path: "a.sock"}},
		{spec: "localhost:8080=/tmp/a.sock", wantErr: true},
		{spec: "agent.local=", wantErr: true},
		{spec: "", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.spec, func(t *testing.T) {
			t.Parallel()
			got, err := Parse(tc.spec)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Parse(%q) error = %v, want error %v", tc.spec, err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("Parse(%q) wrong result (-want +got) diff = %s", tc.spec, diff)
			}
		})
	}
}

func TestMatches(t *testing.T) {
	t.Parallel()
	tests := []struct {
		spec Spec
		host string
		want bool
	}{
		{spec: Spec{Path: "/s"}, host: "anything.example.com", want: true},
		{spec: Spec{Host: "agent.local", Path: "/s"}, host: "AGENT.local", want: true},
		{spec: Spec{Host: "agent.local", Path: "/s"}, host: "other.local", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.host, func(t *testing.T) {
			t.Parallel()
			if got := tc.spec.Matches(tc.host); got != tc.want {
				t.Fatalf("%+v.Matches(%q) = %v, want %v", tc.spec, tc.host, got, tc.want)
			}
		})
	}
}
