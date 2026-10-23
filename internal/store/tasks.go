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

package store

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv/taskstore"
)

// Viewer identifies the caller of a task store operation. all is true for
// callers who see every task, such as owners.
type Viewer func(ctx context.Context) (user string, all bool, err error)

// TaskStore is a [taskstore.Store] for one scope: the agent itself, or one
// webhook node. Callers see the tasks they created, unless the [Viewer]
// reports that they see all tasks.
type TaskStore struct {
	s      *Store
	scope  string
	viewer Viewer
}

var _ taskstore.Store = (*TaskStore)(nil)

// Tasks returns the task store of scope.
func (s *Store) Tasks(scope string, viewer Viewer) *TaskStore {
	return &TaskStore{s: s, scope: scope, viewer: viewer}
}

func statusTime(t *a2a.Task) int64 {
	if t.Status.Timestamp != nil {
		return t.Status.Timestamp.UnixNano()
	}
	return 0
}

// Create implements [taskstore.Store].
func (ts *TaskStore) Create(ctx context.Context, task *a2a.Task) (taskstore.TaskVersion, error) {
	user, _, err := ts.viewer(ctx)
	if err != nil {
		return taskstore.TaskVersionMissing, err
	}
	data, err := json.Marshal(task)
	if err != nil {
		return taskstore.TaskVersionMissing, err
	}
	res, err := ts.s.db.ExecContext(ctx, `INSERT INTO tasks (id, scope, context_id, owner, version, state, status_time, updated, data)
		VALUES (?, ?, ?, ?, 1, ?, ?, ?, ?) ON CONFLICT DO NOTHING`,
		string(task.ID), ts.scope, task.ContextID, user, string(task.Status.State), statusTime(task), ts.s.now().UnixNano(), string(data))
	if err != nil {
		return taskstore.TaskVersionMissing, err
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return taskstore.TaskVersionMissing, errors.Join(taskstore.ErrTaskAlreadyExists, err)
	}
	return 1, nil
}

// Update implements [taskstore.Store].
func (ts *TaskStore) Update(ctx context.Context, req *taskstore.UpdateRequest) (taskstore.TaskVersion, error) {
	stored, err := ts.Get(ctx, req.Task.ID)
	if err != nil {
		return taskstore.TaskVersionMissing, err
	}
	if req.PrevVersion != taskstore.TaskVersionMissing && stored.Version != req.PrevVersion {
		return taskstore.TaskVersionMissing, taskstore.ErrConcurrentModification
	}
	data, err := json.Marshal(req.Task)
	if err != nil {
		return taskstore.TaskVersionMissing, err
	}
	next := stored.Version + 1
	res, err := ts.s.db.ExecContext(ctx, `UPDATE tasks SET version = ?, state = ?, status_time = ?, updated = ?, data = ?
		WHERE scope = ? AND id = ? AND version = ?`,
		int64(next), string(req.Task.Status.State), statusTime(req.Task), ts.s.now().UnixNano(), string(data),
		ts.scope, string(req.Task.ID), int64(stored.Version))
	if err != nil {
		return taskstore.TaskVersionMissing, err
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return taskstore.TaskVersionMissing, errors.Join(taskstore.ErrConcurrentModification, err)
	}
	return next, nil
}

// Get implements [taskstore.Store].
func (ts *TaskStore) Get(ctx context.Context, taskID a2a.TaskID) (*taskstore.StoredTask, error) {
	user, all, err := ts.viewer(ctx)
	if err != nil {
		return nil, err
	}
	var (
		owner, data string
		version     int64
	)
	err = ts.s.db.QueryRowContext(ctx, `SELECT owner, version, data FROM tasks WHERE scope = ? AND id = ?`,
		ts.scope, string(taskID)).Scan(&owner, &version, &data)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !all && owner != user) {
		return nil, a2a.ErrTaskNotFound
	}
	if err != nil {
		return nil, err
	}
	task := &a2a.Task{}
	if err := json.Unmarshal([]byte(data), task); err != nil {
		return nil, fmt.Errorf("decoding task %s: %w", taskID, err)
	}
	return &taskstore.StoredTask{Task: task, Version: taskstore.TaskVersion(version), User: owner}, nil
}

const (
	defaultPageSize   = 50
	defaultMaxHistory = 100
)

// List implements [taskstore.Store].
func (ts *TaskStore) List(ctx context.Context, req *a2a.ListTasksRequest) (*a2a.ListTasksResponse, error) {
	user, all, err := ts.viewer(ctx)
	if err != nil {
		return nil, err
	}
	pageSize := req.PageSize
	if pageSize == 0 {
		pageSize = defaultPageSize
	}
	if pageSize < 1 || pageSize > 100 {
		return nil, fmt.Errorf("page size must be between 1 and 100 inclusive, got %d: %w", pageSize, a2a.ErrInvalidRequest)
	}

	where := []string{"scope = ?"}
	args := []any{ts.scope}
	if !all {
		where = append(where, "owner = ?")
		args = append(args, user)
	}
	if req.ContextID != "" {
		where = append(where, "context_id = ?")
		args = append(args, req.ContextID)
	}
	if req.Status != a2a.TaskStateUnspecified {
		where = append(where, "state = ?")
		args = append(args, string(req.Status))
	}
	if req.StatusTimestampAfter != nil {
		where = append(where, "status_time >= ?")
		args = append(args, req.StatusTimestampAfter.UnixNano())
	}
	filter := strings.Join(where, " AND ")

	var total int
	if err := ts.s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE "+filter, args...).Scan(&total); err != nil {
		return nil, err
	}

	pageWhere, pageArgs := filter, args
	if req.PageToken != "" {
		updated, id, err := decodePageToken(req.PageToken)
		if err != nil {
			return nil, err
		}
		pageWhere += " AND (updated < ? OR (updated = ? AND id > ?))"
		pageArgs = append(append([]any{}, args...), updated, updated, id)
	}
	rows, err := ts.s.db.QueryContext(ctx, "SELECT id, updated, data FROM tasks WHERE "+pageWhere+
		" ORDER BY updated DESC, id LIMIT ?", append(pageArgs, pageSize+1)...)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	history := defaultMaxHistory
	if req.HistoryLength != nil {
		history = *req.HistoryLength
	}
	resp := &a2a.ListTasksResponse{Tasks: []*a2a.Task{}, TotalSize: total, PageSize: pageSize}
	var lastUpdated int64
	for rows.Next() {
		var (
			id, data string
			updated  int64
		)
		if err := rows.Scan(&id, &updated, &data); err != nil {
			return nil, err
		}
		if len(resp.Tasks) == pageSize {
			last := resp.Tasks[len(resp.Tasks)-1]
			resp.NextPageToken = encodePageToken(lastUpdated, string(last.ID))
			break
		}
		task := &a2a.Task{}
		if err := json.Unmarshal([]byte(data), task); err != nil {
			return nil, fmt.Errorf("decoding task %s: %w", id, err)
		}
		switch {
		case history <= 0:
			task.History = []*a2a.Message{}
		case len(task.History) > history:
			task.History = task.History[len(task.History)-history:]
		}
		if !req.IncludeArtifacts {
			task.Artifacts = nil
		}
		lastUpdated = updated
		resp.Tasks = append(resp.Tasks, task)
	}
	return resp, rows.Err()
}

func encodePageToken(updated int64, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(updated, 10) + "|" + id))
}

func decodePageToken(tok string) (int64, string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil {
		return 0, "", fmt.Errorf("invalid page token: %w", a2a.ErrInvalidRequest)
	}
	ts, id, ok := strings.Cut(string(raw), "|")
	updated, err := strconv.ParseInt(ts, 10, 64)
	if !ok || err != nil {
		return 0, "", fmt.Errorf("invalid page token: %w", a2a.ErrInvalidRequest)
	}
	return updated, id, nil
}

// FailUnfinished marks every task of the scope which isn't in a terminal
// state as failed with reason. Executions don't survive a restart, so such
// tasks would otherwise never finish.
func (ts *TaskStore) FailUnfinished(ctx context.Context, reason string) (int, error) {
	rows, err := ts.s.db.QueryContext(ctx, `SELECT id, version, data FROM tasks WHERE scope = ? AND state NOT IN (?, ?, ?, ?)`,
		ts.scope, string(a2a.TaskStateCompleted), string(a2a.TaskStateFailed), string(a2a.TaskStateCanceled), string(a2a.TaskStateRejected))
	if err != nil {
		return 0, err
	}
	type row struct {
		id      string
		version int64
		data    string
	}
	var pending []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.version, &r.data); err != nil {
			return 0, errors.Join(err, rows.Close())
		}
		pending = append(pending, r)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return 0, err
	}
	now := ts.s.now()
	for _, r := range pending {
		task := &a2a.Task{}
		if err := json.Unmarshal([]byte(r.data), task); err != nil {
			return 0, err
		}
		msg := a2a.NewMessageForTask(a2a.MessageRoleAgent, task, a2a.NewTextPart(reason))
		task.Status = a2a.TaskStatus{State: a2a.TaskStateFailed, Message: msg, Timestamp: &now}
		data, err := json.Marshal(task)
		if err != nil {
			return 0, err
		}
		if _, err := ts.s.db.ExecContext(ctx, `UPDATE tasks SET version = ?, state = ?, status_time = ?, updated = ?, data = ? WHERE scope = ? AND id = ?`,
			r.version+1, string(a2a.TaskStateFailed), now.UnixNano(), now.UnixNano(), string(data), ts.scope, r.id); err != nil {
			return 0, err
		}
	}
	return len(pending), nil
}

// LatestInContext returns the most recently updated task of a context.
func (ts *TaskStore) LatestInContext(ctx context.Context, contextID string) (*a2a.Task, error) {
	var data string
	err := ts.s.db.QueryRowContext(ctx, `SELECT data FROM tasks WHERE scope = ? AND context_id = ? ORDER BY updated DESC LIMIT 1`,
		ts.scope, contextID).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	task := &a2a.Task{}
	return task, json.Unmarshal([]byte(data), task)
}

// TaskCounts returns the number of tasks of the scope per state.
func (ts *TaskStore) TaskCounts(ctx context.Context) (map[a2a.TaskState]int, error) {
	rows, err := ts.s.db.QueryContext(ctx, `SELECT state, COUNT(*) FROM tasks WHERE scope = ? GROUP BY state`, ts.scope)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	out := map[a2a.TaskState]int{}
	for rows.Next() {
		var (
			state string
			n     int
		)
		if err := rows.Scan(&state, &n); err != nil {
			return nil, err
		}
		out[a2a.TaskState(state)] = n
	}
	return out, rows.Err()
}

func closeRows(rows *sql.Rows) {
	if err := rows.Close(); err != nil {
		slog.Warn("closing rows", "error", err)
	}
}

func unixTime(ns int64) time.Time {
	if ns == 0 {
		return time.Time{}
	}
	return time.Unix(0, ns)
}
