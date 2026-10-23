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

// Package store keeps the state of an agent server in SQLite: tasks, ACP
// sessions, linked users, pairing codes and tokens, contacts, webhook nodes,
// channel conversations, questions and the handles of agents URLs.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	_ "modernc.org/sqlite" // registers the sqlite driver
)

// ErrNotFound is returned when a record doesn't exist.
var ErrNotFound = errors.New("not found")

// Store is the SQLite database of one agent server.
type Store struct {
	db  *sql.DB
	now func() time.Time
}

// Open opens or creates the database at path. ":memory:" opens a private
// in-memory database, for tests.
func Open(path string) (*Store, error) {
	u := url.URL{Scheme: "file", OmitHost: true, Path: path, RawQuery: "_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"}
	dsn := u.String()
	if path == ":memory:" {
		dsn = "file::memory:?_pragma=foreign_keys(1)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// SQLite allows one writer; a single connection also keeps an in-memory
	// database alive and shared.
	db.SetMaxOpenConns(1)
	s := &Store{db: db, now: time.Now}
	if err := s.migrate(context.Background()); err != nil {
		return nil, errors.Join(fmt.Errorf("migrating %s: %w", path, err), db.Close())
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// migrations bring the database from the version in PRAGMA user_version,
// which is its index here, to the next. They run in a transaction each and
// must never change once released; add a new one instead.
var migrations = []func(ctx context.Context, tx *sql.Tx) error{
	// 1: the prototype schema. Databases created before user_version was
	// kept are at 0 with some of these tables, so it is idempotent.
	func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, schemaV1); err != nil {
			return err
		}
		return addColumn(ctx, tx, "links", "name", "TEXT NOT NULL DEFAULT ''")
	},
	// 2: handles and pending questions move out of the kv table, which keeps
	// only channel state, and remote questions and the messages that show
	// questions are kept, so both survive a restart.
	func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, schemaV2)
		return err
	},
}

const schemaV1 = `
CREATE TABLE IF NOT EXISTS kv (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS sessions (context_id TEXT PRIMARY KEY, session_id TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS tasks (
	id TEXT NOT NULL, scope TEXT NOT NULL, context_id TEXT NOT NULL, owner TEXT NOT NULL,
	version INTEGER NOT NULL, state TEXT NOT NULL, status_time INTEGER NOT NULL,
	updated INTEGER NOT NULL, data TEXT NOT NULL,
	PRIMARY KEY (scope, id));
CREATE INDEX IF NOT EXISTS tasks_context ON tasks (scope, context_id);
CREATE INDEX IF NOT EXISTS tasks_updated ON tasks (scope, updated DESC, id);
CREATE TABLE IF NOT EXISTS users (id TEXT PRIMARY KEY, roles TEXT NOT NULL, created INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS links (
	channel TEXT NOT NULL, identity TEXT NOT NULL, user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	conv TEXT NOT NULL DEFAULT '', created INTEGER NOT NULL,
	PRIMARY KEY (channel, identity));
CREATE TABLE IF NOT EXISTS convs (
	channel TEXT NOT NULL, conv TEXT NOT NULL, context_id TEXT NOT NULL, updated INTEGER NOT NULL,
	PRIMARY KEY (channel, conv));
CREATE INDEX IF NOT EXISTS convs_context ON convs (context_id);
CREATE TABLE IF NOT EXISTS codes (
	hash TEXT PRIMARY KEY, purpose TEXT NOT NULL, name TEXT NOT NULL, role TEXT NOT NULL,
	expires INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS tokens (
	hash TEXT PRIMARY KEY, name TEXT NOT NULL, principal TEXT NOT NULL, roles TEXT NOT NULL,
	url TEXT NOT NULL, created INTEGER NOT NULL, last_used INTEGER NOT NULL DEFAULT 0);
CREATE INDEX IF NOT EXISTS tokens_name ON tokens (name);
CREATE TABLE IF NOT EXISTS contacts (
	name TEXT PRIMARY KEY, url TEXT NOT NULL, token TEXT NOT NULL DEFAULT '', via TEXT NOT NULL,
	created INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS nodes (
	id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT NOT NULL DEFAULT '',
	webhook TEXT NOT NULL, headers TEXT NOT NULL DEFAULT '{}', secret TEXT NOT NULL,
	token_hash TEXT NOT NULL DEFAULT '', created INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS seen (key TEXT PRIMARY KEY, at INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS questions (
	id TEXT PRIMARY KEY, token TEXT NOT NULL UNIQUE, task_id TEXT NOT NULL, context_id TEXT NOT NULL,
	contact TEXT NOT NULL DEFAULT '', data TEXT NOT NULL, created INTEGER NOT NULL,
	answered_by TEXT NOT NULL DEFAULT '', answer TEXT NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS watches (
	contact TEXT NOT NULL, task_id TEXT NOT NULL, remote_context_id TEXT NOT NULL,
	origin_context_id TEXT NOT NULL, on_behalf_of TEXT NOT NULL, created INTEGER NOT NULL,
	PRIMARY KEY (contact, task_id));
`

const schemaV2 = `
CREATE TABLE handles (
	handle TEXT PRIMARY KEY, context_id TEXT NOT NULL, on_behalf_of TEXT NOT NULL, hops INTEGER NOT NULL,
	UNIQUE (context_id, on_behalf_of, hops));
INSERT OR IGNORE INTO handles
	SELECT substr(key, 8), json_extract(value, '$.c'), coalesce(json_extract(value, '$.u'), ''), coalesce(json_extract(value, '$.h'), 0)
	FROM kv WHERE key LIKE 'handle:%';
CREATE TABLE pending_questions (
	channel TEXT NOT NULL, conv TEXT NOT NULL, question_id TEXT NOT NULL,
	PRIMARY KEY (channel, conv));
INSERT OR IGNORE INTO pending_questions
	SELECT substr(rest, 1, instr(rest, ':') - 1), substr(rest, instr(rest, ':') + 1), value
	FROM (SELECT substr(key, 9) AS rest, value FROM kv WHERE key LIKE 'pending:%' AND value != '');
DELETE FROM kv WHERE key LIKE 'handle:%' OR key LIKE 'handle-of:%' OR key LIKE 'pending:%';
ALTER TABLE kv RENAME TO channel_state;
ALTER TABLE questions ADD COLUMN hitl_id TEXT NOT NULL DEFAULT '';
UPDATE questions SET hitl_id = coalesce(json_extract(data, '$.id'), '');
DELETE FROM questions WHERE contact != '' AND rowid NOT IN (
	SELECT min(rowid) FROM questions WHERE contact != '' GROUP BY contact, task_id, hitl_id);
CREATE UNIQUE INDEX questions_remote ON questions (contact, task_id, hitl_id) WHERE contact != '';
CREATE TABLE question_messages (
	question_id TEXT NOT NULL REFERENCES questions(id) ON DELETE CASCADE,
	channel TEXT NOT NULL, conv TEXT NOT NULL, message_id TEXT NOT NULL, text TEXT NOT NULL,
	PRIMARY KEY (question_id, channel, message_id));
`

func (s *Store) migrate(ctx context.Context) error {
	var version int
	if err := s.db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	if version > len(migrations) {
		return fmt.Errorf("the database is at version %d, newer than this a2a supports (%d); upgrade a2a", version, len(migrations))
	}
	for v := version; v < len(migrations); v++ {
		if err := s.runMigration(ctx, v); err != nil {
			return fmt.Errorf("to version %d: %w", v+1, err)
		}
	}
	return nil
}

func (s *Store) runMigration(ctx context.Context, from int) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, tx.Rollback())
		}
	}()
	if err := migrations[from](ctx, tx); err != nil {
		return err
	}
	// PRAGMA takes no bound parameters; the version is an int.
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", from+1)); err != nil {
		return err
	}
	return tx.Commit()
}

// addColumn adds a column to a table created before the column existed.
func addColumn(ctx context.Context, tx *sql.Tx, table, column, decl string) error {
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	_, err := tx.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, decl))
	return err
}

// ErrExists is returned when a record to create already exists.
var ErrExists = errors.New("already exists")

// ChannelState keeps small state of the channels, such as the last message
// seen, across restarts. Keys start with the channel name.
type ChannelState struct{ s *Store }

// ChannelState returns the state of the channels.
func (s *Store) ChannelState() ChannelState { return ChannelState{s: s} }

// Get returns the value of key, or false if it is not set.
func (cs ChannelState) Get(ctx context.Context, key string) (string, bool, error) {
	var v string
	err := cs.s.db.QueryRowContext(ctx, `SELECT value FROM channel_state WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return v, err == nil, err
}

// Set stores the value of key.
func (cs ChannelState) Set(ctx context.Context, key, value string) error {
	_, err := cs.s.db.ExecContext(ctx, `INSERT INTO channel_state (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// seenRetention is how long dedup keys are kept.
const seenRetention = 7 * 24 * time.Hour

// FirstSeen records key and reports whether it was new. Channels use it to
// drop redelivered messages.
func (s *Store) FirstSeen(ctx context.Context, key string) (bool, error) {
	now := s.now()
	res, err := s.db.ExecContext(ctx, `INSERT INTO seen (key, at) VALUES (?, ?) ON CONFLICT DO NOTHING`, key, now.UnixNano())
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM seen WHERE at < ?`, now.Add(-seenRetention).UnixNano()); err != nil {
		return false, err
	}
	return n == 1, nil
}

func joinRoles(roles []string) string { return strings.Join(roles, ",") }

func splitRoles(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}
