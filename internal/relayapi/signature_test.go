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

package relayapi

import (
	"errors"
	"testing"
	"time"
)

func TestVerify(t *testing.T) {
	t.Parallel()
	signedAt := time.Unix(1_800_000_000, 0)
	body := []byte(`{"kind":"task"}`)
	header := Sign("secret", signedAt, body)
	tests := []struct {
		name    string
		secret  string
		header  string
		body    []byte
		now     time.Time
		maxSkew time.Duration
		wantErr error
	}{
		{name: "valid", secret: "secret", header: header, body: body, now: signedAt.Add(time.Minute), maxSkew: 5 * time.Minute},
		{name: "any time", secret: "secret", header: header, body: body, now: signedAt.Add(time.Hour)},
		{name: "wrong secret", secret: "other", header: header, body: body, now: signedAt, wantErr: ErrBadSignature},
		{name: "changed body", secret: "secret", header: header, body: []byte(`{}`), now: signedAt, wantErr: ErrBadSignature},
		{name: "too old", secret: "secret", header: header, body: body, now: signedAt.Add(time.Hour), maxSkew: 5 * time.Minute, wantErr: ErrBadSignature},
		{name: "malformed", secret: "secret", header: "v1=abc", body: body, now: signedAt, wantErr: ErrBadSignature},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := Verify(tc.secret, tc.header, tc.body, tc.now, tc.maxSkew); !errors.Is(err, tc.wantErr) {
				t.Fatalf("Verify() error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}
