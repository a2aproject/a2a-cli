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
