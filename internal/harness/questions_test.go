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

package harness

import (
	"slices"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

func TestParseAnswer(t *testing.T) {
	perm := Question{Kind: KindPermission, Options: PermissionOptions}
	choice := Question{Kind: KindQuestion, Options: []Option{{ID: "Red", Label: "Red"}, {ID: "Blue", Label: "Blue"}}}

	tests := []struct {
		name    string
		q       Question
		parts   []*a2a.Part
		want    Answer
		wantErr bool
	}{
		{name: "option id", q: perm, parts: []*a2a.Part{a2a.NewTextPart("always")}, want: Answer{Options: []string{OptionAlways}}},
		{name: "option label", q: perm, parts: []*a2a.Part{a2a.NewTextPart("Allow once")}, want: Answer{Options: []string{OptionOnce}}},
		{name: "yes", q: perm, parts: []*a2a.Part{a2a.NewTextPart("Yes!")}, want: Answer{Options: []string{OptionOnce}}},
		{name: "no", q: perm, parts: []*a2a.Part{a2a.NewTextPart("no")}, want: Answer{Options: []string{OptionReject}}},
		{name: "data part", q: perm, parts: []*a2a.Part{a2a.NewDataPart(map[string]any{"answer": "reject"})}, want: Answer{Options: []string{OptionReject}}},
		{name: "unclear permission reply", q: perm, parts: []*a2a.Part{a2a.NewTextPart("maybe later")}, wantErr: true},
		{name: "choice case-insensitive", q: choice, parts: []*a2a.Part{a2a.NewTextPart("blue")}, want: Answer{Options: []string{"Blue"}}},
		{name: "free-form", q: choice, parts: []*a2a.Part{a2a.NewTextPart("green")}, want: Answer{Text: "green"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseAnswer(tc.q, a2a.NewMessage(a2a.MessageRoleUser, tc.parts...))
			if (err != nil) != tc.wantErr {
				t.Fatalf("ParseAnswer() error = %v, wantErr %v", err, tc.wantErr)
			}
			if !slices.Equal(got.Options, tc.want.Options) || got.Text != tc.want.Text {
				t.Errorf("ParseAnswer() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestRules(t *testing.T) {
	r := Rules{Allow: []string{"read", "glob"}, Deny: []string{"web*"}}
	for tool, want := range map[string][]string{"read": {OptionOnce}, "webfetch": {OptionReject}, "bash": nil} {
		ans, ok, err := r.Answer(t.Context(), nil, Question{Kind: KindPermission, Tool: tool})
		if err != nil {
			t.Fatal(err)
		}
		if ok != (want != nil) || !slices.Equal(ans.Options, want) {
			t.Errorf("Rules.Answer(%s) = %v, %v; want %v", tool, ans.Options, ok, want)
		}
	}
}
