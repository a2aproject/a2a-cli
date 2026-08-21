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

package flagparse

import (
	"fmt"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

var stateAliases = map[a2a.TaskState]string{
	a2a.TaskStateSubmitted:     "submitted",
	a2a.TaskStateWorking:       "working",
	a2a.TaskStateCompleted:     "completed",
	a2a.TaskStateFailed:        "failed",
	a2a.TaskStateCanceled:      "canceled",
	a2a.TaskStateRejected:      "rejected",
	a2a.TaskStateInputRequired: "input-required",
	a2a.TaskStateAuthRequired:  "auth-required",
}

// TaskState resolves a task state string or alias to an a2a.TaskState.
func TaskState(s string) (a2a.TaskState, error) {
	lower := strings.ToLower(s)
	for state, alias := range stateAliases {
		if state == a2a.TaskState(s) {
			return state, nil
		}
		if alias == lower {
			return state, nil
		}
	}
	return "", fmt.Errorf("unknown task state %q", s)
}
