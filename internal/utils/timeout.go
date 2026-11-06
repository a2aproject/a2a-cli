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

package utils

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DebounceFunc resets a timeout when called.
type DebounceFunc func()

type debounceKey struct{}

// WithInactivityTimeout derives a context from parent that is canceled when
// DebounceFunc is not called within inactivity timeout d. A response body read
// through ActivityTransport for a request with this context also resets it.
func WithInactivityTimeout(parent context.Context, d time.Duration) (context.Context, DebounceFunc) {
	if d <= 0 {
		return parent, func() {}
	}
	ctx, cancel := context.WithCancelCause(parent)

	resetCh := make(chan struct{}, 1)

	go func() {
		timer := time.NewTimer(d)
		defer timer.Stop()
		for {
			select {
			case <-parent.Done():
				return

			case <-resetCh:
				timer.Reset(d)

			case <-timer.C:
				cancel(fmt.Errorf("no activity for %v: %w", d, context.DeadlineExceeded))
				return
			}
		}
	}()

	debounce := DebounceFunc(func() {
		select {
		case resetCh <- struct{}{}:
		default:
		}
	})
	return context.WithValue(ctx, debounceKey{}, debounce), debounce
}

// ActivityTransport wraps base, or http.DefaultTransport if nil, so that bytes
// read from a response body count as activity for the inactivity timeout of
// the request's context. An SSE stream then stays alive on the keep-alive
// comments a server sends while an agent works, which the SSE parser drops
// without yielding an event.
func ActivityTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return activityTransport{base: base}
}

type activityTransport struct {
	base http.RoundTripper
}

func (t activityTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	debounce, ok := req.Context().Value(debounceKey{}).(DebounceFunc)
	if err != nil || !ok {
		return resp, err
	}
	debounce()
	resp.Body = activityBody{ReadCloser: resp.Body, debounce: debounce}
	return resp, nil
}

type activityBody struct {
	io.ReadCloser
	debounce DebounceFunc
}

func (b activityBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.debounce()
	}
	return n, err
}
