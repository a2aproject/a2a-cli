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

package agentsrv

import (
	"testing"
	"time"
)

func TestFailureLimiter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		failures int
		elapsed  time.Duration
		key      string
		want     bool
	}{
		{name: "below the limit", failures: 2, key: "a", want: true},
		{name: "at the limit", failures: 3, key: "a", want: false},
		{name: "other key", failures: 3, key: "b", want: true},
		{name: "next window", failures: 3, elapsed: time.Minute, key: "a", want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			now := time.Unix(1000, 0)
			l := newFailureLimiter(3, time.Minute)
			l.now = func() time.Time { return now }
			for range tc.failures {
				l.fail("a")
			}
			now = now.Add(tc.elapsed)
			if got := l.allow(tc.key); got != tc.want {
				t.Fatalf("allow(%q) = %v, want %v", tc.key, got, tc.want)
			}
		})
	}
}
