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

// Package polling emulates a streaming event feed by repeatedly polling a task's
// state at a fixed interval until it reaches a terminal state.
package polling

import (
	"context"
	"fmt"
	"iter"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2aevent"
)

// maxSuccessiveFailures is the number of consecutive GetTask failures tolerated
// before polling gives up.
const maxSuccessiveFailures = 3

// WaitForTask polls the task identified by req at the given interval until it
// reaches a terminal state (completed/failed/canceled/rejected) or an
// interrupted state that needs the caller to act (input-required/auth-required),
// returning the final task. The first poll happens immediately.
func WaitForTask(ctx context.Context, client *a2aclient.Client, req *a2a.GetTaskRequest, interval time.Duration) (*a2a.Task, error) {
	successiveFailures := 0
	for {
		task, err := client.GetTask(ctx, req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			successiveFailures++
			if successiveFailures >= maxSuccessiveFailures {
				return nil, fmt.Errorf("successive polling failure threshold exceeded for task %q: %w", req.ID, err)
			}
		} else {
			successiveFailures = 0
			if task.Status.State.Terminal() ||
				task.Status.State == a2a.TaskStateInputRequired ||
				task.Status.State == a2a.TaskStateAuthRequired {
				return task, nil
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
}

// Stream sends the original message and then polls the resulting task at the
// given interval, yielding task events until it reaches a terminal state.
func Stream(ctx context.Context, client *a2aclient.Client, original *a2a.SendMessageRequest, interval time.Duration) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		req := *original
		if req.Config == nil {
			req.Config = &a2a.SendMessageConfig{ReturnImmediately: true}
		} else if !req.Config.ReturnImmediately {
			config := *original.Config
			req.Config = &config
			req.Config.ReturnImmediately = true
		}

		result, err := client.SendMessage(ctx, &req)
		if err != nil {
			yield(nil, fmt.Errorf("failed to send a message: %w", err))
			return
		}
		if !yield(result, nil) {
			return
		}
		if _, ok := result.(*a2a.Message); ok {
			return
		}
		prevState, ok := result.(*a2a.Task)
		if !ok {
			yield(nil, fmt.Errorf("unexpected type: %T", result))
			return
		}
		tid := prevState.ID

		successiveFailures := 0
		for !prevState.Status.State.Terminal() && prevState.Status.State != a2a.TaskStateInputRequired {
			select {
			case <-ctx.Done():
				yield(nil, ctx.Err())
				return
			case <-time.After(interval):
			}

			task, err := client.GetTask(ctx, &a2a.GetTaskRequest{ID: tid, Tenant: req.Tenant})
			if err != nil {
				successiveFailures++
				if successiveFailures >= maxSuccessiveFailures {
					yield(nil, fmt.Errorf("successive polling failure threshold exceeded for task %q", tid))
					return
				}
				continue
			}
			successiveFailures = 0

			var events []a2a.Event
			if task.Status.State.Terminal() || task.Status.State == a2a.TaskStateInputRequired {
				events = append(events, task)
			} else {
				events = a2aevent.Recover(prevState, task)
			}

			for _, event := range events {
				if !yield(event, nil) {
					return
				}
			}
			prevState = task
		}
	}
}
