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

import "time"

// retryRename calls rename until it succeeds, fails with an error that
// retryable rejects, or has been tried attempts times, waiting wait between
// tries, and returns its last error.
func retryRename(rename func(from, to string) error, retryable func(error) bool, from, to string, attempts int, wait time.Duration) error {
	var err error
	for i := range attempts {
		if err = rename(from, to); err == nil || !retryable(err) {
			return err
		}
		if i < attempts-1 {
			time.Sleep(wait)
		}
	}
	return err
}
