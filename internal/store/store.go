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
// channel conversations and pending questions.
package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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

const schema = `
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
	conv TEXT NOT NULL DEFAULT '', created INTEGER NOT NULL, name TEXT NOT NULL DEFAULT '',
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

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return err
	}
	return s.addColumn(ctx, "links", "name", "TEXT NOT NULL DEFAULT ''")
}

// addColumn adds a column to a table created before the column existed.
func (s *Store) addColumn(ctx context.Context, table, column, decl string) error {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, decl))
	return err
}

// Get returns a value from the key-value table.
func (s *Store) Get(ctx context.Context, key string) (string, bool, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM kv WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return v, err == nil, err
}

// Set stores a value in the key-value table.
func (s *Store) Set(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO kv (key, value) VALUES (?, ?)
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

// Hash returns the SHA-256 hex digest under which codes and tokens are stored.
func Hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// RandomString returns n random characters from alphabet, which must be
// shorter than 256 characters.
func RandomString(n int, alphabet string) string {
	// Bytes at or above limit are skipped so that every character is equally likely.
	limit := 256 - 256%len(alphabet)
	out := make([]byte, 0, n)
	buf := make([]byte, n)
	for len(out) < n {
		if _, err := rand.Read(buf); err != nil {
			panic(err)
		}
		for _, b := range buf {
			if int(b) < limit && len(out) < n {
				out = append(out, alphabet[int(b)%len(alphabet)])
			}
		}
	}
	return string(out)
}

// Alphanumeric is an alphabet for tokens and IDs.
const Alphanumeric = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func joinRoles(roles []string) string { return strings.Join(roles, ",") }

func splitRoles(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}
