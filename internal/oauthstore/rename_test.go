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
	"testing"
)

func TestRetryRename(t *testing.T) {
	t.Parallel()
	errBusy := errors.New("busy")
	errOther := errors.New("other")
	retryable := func(err error) bool { return errors.Is(err, errBusy) }
	tests := []struct {
		name      string
		results   []error
		wantCalls int
		wantErr   error
	}{
		{name: "succeeds at once", results: []error{nil}, wantCalls: 1},
		{name: "succeeds after sharing violations", results: []error{errBusy, errBusy, nil}, wantCalls: 3},
		{name: "other errors are not retried", results: []error{errOther}, wantCalls: 1, wantErr: errOther},
		{name: "gives up after the attempts", results: []error{errBusy, errBusy, errBusy, errBusy}, wantCalls: 3, wantErr: errBusy},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			rename := func(from, to string) error {
				if from != "tmp" || to != "grants.json" {
					t.Errorf("rename(%q, %q), want rename(tmp, grants.json)", from, to)
				}
				err := tc.results[calls]
				calls++
				return err
			}
			err := retryRename(rename, retryable, "tmp", "grants.json", 3, 0)
			if !errors.Is(err, tc.wantErr) || (tc.wantErr == nil && err != nil) {
				t.Fatalf("retryRename() error = %v, want %v", err, tc.wantErr)
			}
			if calls != tc.wantCalls {
				t.Fatalf("retryRename() renamed %d times, want %d", calls, tc.wantCalls)
			}
		})
	}
}
