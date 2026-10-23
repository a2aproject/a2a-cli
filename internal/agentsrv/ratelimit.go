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
	"net"
	"sync"
	"time"
)

// failureLimiter allows at most max failures per key in each window, to
// slow down guessing of pairing codes. Behind a tunnel every caller shares
// the tunnel's address, so the limit then applies to all callers together.
type failureLimiter struct {
	max    int
	window time.Duration
	now    func() time.Time

	mu   sync.Mutex
	hits map[string]*failureWindow
}

type failureWindow struct {
	start time.Time
	count int
}

func newFailureLimiter(max int, window time.Duration) *failureLimiter {
	return &failureLimiter{max: max, window: window, now: time.Now, hits: map[string]*failureWindow{}}
}

// allow reports whether key may try again.
func (l *failureLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	w := l.hits[key]
	return w == nil || l.now().Sub(w.start) >= l.window || w.count < l.max
}

// fail records a failure of key.
func (l *failureLimiter) fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	w := l.hits[key]
	if w == nil || now.Sub(w.start) >= l.window {
		w = &failureWindow{start: now}
		l.hits[key] = w
	}
	w.count++
	if len(l.hits) > 1024 {
		for k, v := range l.hits {
			if now.Sub(v.start) >= l.window {
				delete(l.hits, k)
			}
		}
	}
}

// remoteHost is the host of a request's remote address.
func remoteHost(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}
