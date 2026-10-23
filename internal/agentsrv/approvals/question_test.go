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

package approvals

import (
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/google/go-cmp/cmp"

	"github.com/a2aproject/a2a-cli/internal/store"
)

func permissionMessage() *a2a.Message {
	return a2a.NewMessage(a2a.MessageRoleAgent,
		a2a.NewTextPart("Run git push?"),
		a2a.NewDataPart(map[string]any{hitlKey: map[string]any{
			"id": "tool1", "type": TypePermission, "title": "git push",
			"options": []any{
				map[string]any{"id": "once", "label": "Allow"},
				map[string]any{"id": "always", "label": "Always allow"},
				map[string]any{"id": "reject"},
			},
		}}))
}

func TestFromStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status a2a.TaskStatus
		want   *Question
	}{
		{
			name:   "permission",
			status: a2a.TaskStatus{State: a2a.TaskStateInputRequired, Message: permissionMessage()},
			want: &Question{ID: "tool1", Type: TypePermission, Title: "git push", Text: "Run git push?",
				Options: []Option{{ID: "once", Label: "Allow"}, {ID: "always", Label: "Always allow"}, {ID: "reject", Label: "reject"}}},
		},
		{name: "working", status: a2a.TaskStatus{State: a2a.TaskStateWorking, Message: permissionMessage()}},
		{name: "plain input request", status: a2a.TaskStatus{State: a2a.TaskStateInputRequired, Message: a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart("Which branch?"))}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := FromStatus(tc.status)
			if diff := cmp.Diff(tc.want, got, cmp.FilterPath(func(p cmp.Path) bool { return p.Last().String() == ".Data" }, cmp.Ignore())); diff != "" {
				t.Fatalf("FromStatus() wrong result (-want +got) diff = %s", diff)
			}
		})
	}
}

func TestFromRecordKeepsText(t *testing.T) {
	t.Parallel()
	q := FromMessage(permissionMessage())
	rec := store.Question{Data: map[string]any{"id": q.ID, "type": q.Type, "title": q.Title, "options": q.Data["options"], "text": q.Text}}
	got := FromRecord(rec)
	if got.Display() != "Run git push?" || len(got.Options) != 3 {
		t.Fatalf("FromRecord() = %+v, want the text and three options", got)
	}
}

func TestMatch(t *testing.T) {
	t.Parallel()
	q := FromMessage(permissionMessage())
	tests := []struct {
		reply  string
		want   string
		wantOK bool
	}{
		{reply: "Allow", want: "once", wantOK: true},
		{reply: "always allow", want: "always", wantOK: true},
		{reply: "approve.", want: "once", wantOK: true},
		{reply: " Deny! ", want: "reject", wantOK: true},
		{reply: "reject", want: "reject", wantOK: true},
		{reply: "maybe"},
	}
	for _, tc := range tests {
		t.Run(tc.reply, func(t *testing.T) {
			t.Parallel()
			got, ok := q.Match(tc.reply)
			if ok != tc.wantOK || got.ID != tc.want {
				t.Fatalf("Match(%q) = %q, %v, want %q, %v", tc.reply, got.ID, ok, tc.want, tc.wantOK)
			}
		})
	}
	if got, want := q.Words(), "approve, always, deny"; got != want {
		t.Fatalf("Words() = %q, want %q", got, want)
	}
}

func TestDisplayPrincipal(t *testing.T) {
	t.Parallel()
	for id, want := range map[string]string{"": "A caller", "pair:anna": "anna", "owner": "owner"} {
		if got := DisplayPrincipal(id); got != want {
			t.Fatalf("DisplayPrincipal(%q) = %q, want %q", id, got, want)
		}
	}
}
