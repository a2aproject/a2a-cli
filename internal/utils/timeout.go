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
	"time"
)

// DebounceFunc resets a timeout when called.
type DebounceFunc func()

// WithInactivityTimeout derives a context from parent that is canceled when
// DebounceFunc is not called within inactivity timeout d.
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
				cancel(fmt.Errorf("no activity for %v", d.sho))
				return
			}
		}
	}()

	return ctx, func() {
		select {
		case resetCh <- struct{}{}:
		default:
		}
	}
}
