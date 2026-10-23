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
	"encoding/json"
	"errors"
	"slices"
	"time"
)

// SessionStore implements the a2aacp session store: A2A context → ACP session.
type SessionStore struct{ s *Store }

// Sessions returns the ACP session store.
func (s *Store) Sessions() SessionStore { return SessionStore{s: s} }

// Get returns the ACP session of an A2A context.
func (ss SessionStore) Get(ctx context.Context, contextID string) (string, bool, error) {
	var id string
	err := ss.s.db.QueryRowContext(ctx, `SELECT session_id FROM sessions WHERE context_id = ?`, contextID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return id, err == nil, err
}

// Put records the ACP session of an A2A context.
func (ss SessionStore) Put(ctx context.Context, contextID, sessionID string) error {
	_, err := ss.s.db.ExecContext(ctx, `INSERT INTO sessions (context_id, session_id) VALUES (?, ?)
		ON CONFLICT (context_id) DO UPDATE SET session_id = excluded.session_id`, contextID, sessionID)
	return err
}

// User is a person known to the agent, identified across channels.
type User struct {
	ID      string
	Roles   []string
	Created time.Time
}

// Link ties a channel identity to a user. Conv is the conversation of the
// identity's direct chat, where the agent can reach the user unprompted.
type Link struct {
	Channel  string
	Identity string
	UserID   string
	Conv     string
	// Name is the pairing the link was made with, so that revoking the
	// pairing removes it even when it links to the owner.
	Name string
}

// EnsureUser creates the user if it doesn't exist and returns it.
func (s *Store) EnsureUser(ctx context.Context, id string, roles []string) (User, error) {
	if _, err := s.db.ExecContext(ctx, `INSERT INTO users (id, roles, created) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`,
		id, joinRoles(roles), s.now().UnixNano()); err != nil {
		return User{}, err
	}
	return s.User(ctx, id)
}

// User returns a user.
func (s *Store) User(ctx context.Context, id string) (User, error) {
	var (
		roles   string
		created int64
	)
	err := s.db.QueryRowContext(ctx, `SELECT roles, created FROM users WHERE id = ?`, id).Scan(&roles, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	return User{ID: id, Roles: splitRoles(roles), Created: unixTime(created)}, nil
}

// Users lists users with role, or all users if role is empty.
func (s *Store) Users(ctx context.Context, role string) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, roles, created FROM users ORDER BY created`)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	var out []User
	for rows.Next() {
		var (
			u       User
			roles   string
			created int64
		)
		if err := rows.Scan(&u.ID, &roles, &created); err != nil {
			return nil, err
		}
		u.Roles, u.Created = splitRoles(roles), unixTime(created)
		if role == "" || hasRole(u.Roles, role) {
			out = append(out, u)
		}
	}
	return out, rows.Err()
}

func hasRole(roles []string, role string) bool { return slices.Contains(roles, role) }

// PutLink links a channel identity to a user, replacing an existing link.
func (s *Store) PutLink(ctx context.Context, l Link) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO links (channel, identity, user_id, conv, name, created) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (channel, identity) DO UPDATE SET user_id = excluded.user_id, conv = excluded.conv, name = excluded.name`,
		l.Channel, l.Identity, l.UserID, l.Conv, l.Name, s.now().UnixNano())
	return err
}

// LinkOf returns the link of a channel identity.
func (s *Store) LinkOf(ctx context.Context, channel, identity string) (Link, error) {
	l := Link{Channel: channel, Identity: identity}
	err := s.db.QueryRowContext(ctx, `SELECT user_id, conv FROM links WHERE channel = ? AND identity = ?`,
		channel, identity).Scan(&l.UserID, &l.Conv)
	if errors.Is(err, sql.ErrNoRows) {
		return Link{}, ErrNotFound
	}
	return l, err
}

// LinksOf returns the links of a user.
func (s *Store) LinksOf(ctx context.Context, userID string) ([]Link, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT channel, identity, conv FROM links WHERE user_id = ? ORDER BY created`, userID)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	var out []Link
	for rows.Next() {
		l := Link{UserID: userID}
		if err := rows.Scan(&l.Channel, &l.Identity, &l.Conv); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ContextOf returns the A2A context of a channel conversation.
func (s *Store) ContextOf(ctx context.Context, channel, conv string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT context_id FROM convs WHERE channel = ? AND conv = ?`, channel, conv).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return id, err
}

// SetContext binds a channel conversation to an A2A context.
func (s *Store) SetContext(ctx context.Context, channel, conv, contextID string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO convs (channel, conv, context_id, updated) VALUES (?, ?, ?, ?)
		ON CONFLICT (channel, conv) DO UPDATE SET context_id = excluded.context_id, updated = excluded.updated`,
		channel, conv, contextID, s.now().UnixNano())
	return err
}

// ConvOf returns the channel conversation bound to an A2A context.
func (s *Store) ConvOf(ctx context.Context, contextID string) (channel, conv string, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT channel, conv FROM convs WHERE context_id = ? ORDER BY updated DESC LIMIT 1`,
		contextID).Scan(&channel, &conv)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrNotFound
	}
	return channel, conv, err
}

// Code is a pending one-time code. Purpose is pair (exchanged for a token or
// a channel link) or node (exchanged for a node token).
type Code struct {
	Purpose string
	// Name is the principal name for pair codes, or the node ID.
	Name    string
	Role    string
	Expires time.Time
}

// PutCode stores a code by its hash.
func (s *Store) PutCode(ctx context.Context, code string, c Code) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO codes (hash, purpose, name, role, expires) VALUES (?, ?, ?, ?, ?)`,
		Hash(code), c.Purpose, c.Name, c.Role, c.Expires.UnixNano())
	return err
}

// ErrCodeExpired is returned when a code was found but has expired.
var ErrCodeExpired = errors.New("the code has expired")

// TakeCode consumes a code of the given purpose, or of any purpose when
// purpose is empty. A code can be used once.
func (s *Store) TakeCode(ctx context.Context, code, purpose string) (Code, error) {
	var c Code
	var expires int64
	err := s.db.QueryRowContext(ctx, `DELETE FROM codes WHERE hash = ? AND (? = '' OR purpose = ?) RETURNING purpose, name, role, expires`,
		Hash(code), purpose, purpose).Scan(&c.Purpose, &c.Name, &c.Role, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return Code{}, ErrNotFound
	}
	if err != nil {
		return Code{}, err
	}
	c.Expires = unixTime(expires)
	if s.now().After(c.Expires) {
		return Code{}, ErrCodeExpired
	}
	return c, nil
}

// Token is a bearer token issued by pairing, stored by its hash.
type Token struct {
	Name      string
	Principal string
	Roles     []string
	// URL is the agent URL the token was issued at.
	URL      string
	Created  time.Time
	LastUsed time.Time
}

// PutToken stores a token by its hash.
func (s *Store) PutToken(ctx context.Context, token string, t Token) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO tokens (hash, name, principal, roles, url, created) VALUES (?, ?, ?, ?, ?, ?)`,
		Hash(token), t.Name, t.Principal, joinRoles(t.Roles), t.URL, s.now().UnixNano())
	return err
}

// LookupToken returns the token record and records its use.
func (s *Store) LookupToken(ctx context.Context, token string) (Token, error) {
	var (
		t                 Token
		roles             string
		created, lastUsed int64
	)
	h := Hash(token)
	err := s.db.QueryRowContext(ctx, `SELECT name, principal, roles, url, created, last_used FROM tokens WHERE hash = ?`, h).
		Scan(&t.Name, &t.Principal, &roles, &t.URL, &created, &lastUsed)
	if errors.Is(err, sql.ErrNoRows) {
		return Token{}, ErrNotFound
	}
	if err != nil {
		return Token{}, err
	}
	t.Roles, t.Created, t.LastUsed = splitRoles(roles), unixTime(created), unixTime(lastUsed)
	if _, err := s.db.ExecContext(ctx, `UPDATE tokens SET last_used = ? WHERE hash = ?`, s.now().UnixNano(), h); err != nil {
		return Token{}, err
	}
	return t, nil
}

// Tokens lists issued tokens.
func (s *Store) Tokens(ctx context.Context) ([]Token, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name, principal, roles, url, created, last_used FROM tokens ORDER BY created`)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	var out []Token
	for rows.Next() {
		var (
			t                 Token
			roles             string
			created, lastUsed int64
		)
		if err := rows.Scan(&t.Name, &t.Principal, &roles, &t.URL, &created, &lastUsed); err != nil {
			return nil, err
		}
		t.Roles, t.Created, t.LastUsed = splitRoles(roles), unixTime(created), unixTime(lastUsed)
		out = append(out, t)
	}
	return out, rows.Err()
}

// RevokeTokens deletes the tokens and channel links made with the pairing
// name, and the user of principal with its links. It returns the number of
// tokens and links deleted.
func (s *Store) RevokeTokens(ctx context.Context, name, principal string) (n int, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, tx.Rollback())
		}
	}()
	res, err := tx.ExecContext(ctx, `DELETE FROM tokens WHERE name = ?`, name)
	if err != nil {
		return 0, err
	}
	tokens, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	res, err = tx.ExecContext(ctx, `DELETE FROM links WHERE name = ? OR user_id = ?`, name, principal)
	if err != nil {
		return 0, err
	}
	links, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, principal); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int(tokens + links), nil
}

// Contact is another agent this agent may call.
type Contact struct {
	Name string
	URL  string
	// Token is the outbound a2p_ token, empty for contacts reached over the
	// private network, which recognize the caller by its network identity.
	Token string
	// Via is private (the network provider's HTTP client) or https.
	Via     string
	Created time.Time
}

// PutContact creates or replaces a contact.
func (s *Store) PutContact(ctx context.Context, c Contact) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO contacts (name, url, token, via, created) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (name) DO UPDATE SET url = excluded.url, token = excluded.token, via = excluded.via`,
		c.Name, c.URL, c.Token, c.Via, s.now().UnixNano())
	return err
}

// Contact returns a contact by name.
func (s *Store) Contact(ctx context.Context, name string) (Contact, error) {
	c := Contact{Name: name}
	var created int64
	err := s.db.QueryRowContext(ctx, `SELECT url, token, via, created FROM contacts WHERE name = ?`, name).
		Scan(&c.URL, &c.Token, &c.Via, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return Contact{}, ErrNotFound
	}
	c.Created = unixTime(created)
	return c, err
}

// Contacts lists contacts.
func (s *Store) Contacts(ctx context.Context) ([]Contact, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name, url, token, via, created FROM contacts ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	var out []Contact
	for rows.Next() {
		var (
			c       Contact
			created int64
		)
		if err := rows.Scan(&c.Name, &c.URL, &c.Token, &c.Via, &created); err != nil {
			return nil, err
		}
		c.Created = unixTime(created)
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeleteContact removes a contact.
func (s *Store) DeleteContact(ctx context.Context, name string) error {
	return s.deleteOne(ctx, `DELETE FROM contacts WHERE name = ?`, name)
}

// Node is a webhook node hosted by the agent.
type Node struct {
	ID          string
	Name        string
	Description string
	Webhook     string
	Headers     map[string]string
	// Secret signs wakes with HMAC-SHA256.
	Secret    string
	TokenHash string
	Created   time.Time
}

// PutNode creates or replaces a node.
func (s *Store) PutNode(ctx context.Context, n Node) error {
	headers, err := json.Marshal(n.Headers)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO nodes (id, name, description, webhook, headers, secret, token_hash, created)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (id) DO UPDATE SET name = excluded.name, description = excluded.description,
		webhook = excluded.webhook, headers = excluded.headers, secret = excluded.secret, token_hash = excluded.token_hash`,
		n.ID, n.Name, n.Description, n.Webhook, string(headers), n.Secret, n.TokenHash, s.now().UnixNano())
	return err
}

// Node returns a node by ID.
func (s *Store) Node(ctx context.Context, id string) (Node, error) {
	n := Node{ID: id}
	var (
		headers string
		created int64
	)
	err := s.db.QueryRowContext(ctx, `SELECT name, description, webhook, headers, secret, token_hash, created FROM nodes WHERE id = ?`, id).
		Scan(&n.Name, &n.Description, &n.Webhook, &headers, &n.Secret, &n.TokenHash, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return Node{}, ErrNotFound
	}
	if err != nil {
		return Node{}, err
	}
	n.Created = unixTime(created)
	return n, json.Unmarshal([]byte(headers), &n.Headers)
}

// Nodes lists nodes.
func (s *Store) Nodes(ctx context.Context) ([]Node, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM nodes ORDER BY id`)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, errors.Join(err, rows.Close())
		}
		ids = append(ids, id)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, err
	}
	out := make([]Node, 0, len(ids))
	for _, id := range ids {
		n, err := s.Node(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

// DeleteNode removes a node and its pending codes.
func (s *Store) DeleteNode(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM codes WHERE purpose = 'node' AND name = ?`, id); err != nil {
		return err
	}
	return s.deleteOne(ctx, `DELETE FROM nodes WHERE id = ?`, id)
}

// Question is a pending agent question relayed to a channel or a web page.
// Contact is set for questions of a remote agent's task.
type Question struct {
	// ID is short, for channel button payloads.
	ID string
	// Token is long and unguessable, for /hitl/<token> pages.
	Token     string
	TaskID    string
	ContextID string
	Contact   string
	// Data is the hitl data part of the question.
	Data       map[string]any
	Created    time.Time
	AnsweredBy string
	Answer     string
}

// PutQuestion stores a question.
func (s *Store) PutQuestion(ctx context.Context, q Question) error {
	data, err := json.Marshal(q.Data)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO questions (id, token, task_id, context_id, contact, data, created) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		q.ID, q.Token, q.TaskID, q.ContextID, q.Contact, string(data), s.now().UnixNano())
	return err
}

// QuestionByID returns a question by its short ID.
func (s *Store) QuestionByID(ctx context.Context, id string) (Question, error) {
	return s.question(ctx, `id = ?`, id)
}

// QuestionByToken returns a question by its page token.
func (s *Store) QuestionByToken(ctx context.Context, token string) (Question, error) {
	return s.question(ctx, `token = ?`, token)
}

func (s *Store) question(ctx context.Context, where string, arg string) (Question, error) {
	var (
		q       Question
		data    string
		created int64
	)
	err := s.db.QueryRowContext(ctx, `SELECT id, token, task_id, context_id, contact, data, created, answered_by, answer FROM questions WHERE `+where, arg).
		Scan(&q.ID, &q.Token, &q.TaskID, &q.ContextID, &q.Contact, &data, &created, &q.AnsweredBy, &q.Answer)
	if errors.Is(err, sql.ErrNoRows) {
		return Question{}, ErrNotFound
	}
	if err != nil {
		return Question{}, err
	}
	q.Created = unixTime(created)
	return q, json.Unmarshal([]byte(data), &q.Data)
}

// ErrAnswered is returned when a question was already answered.
var ErrAnswered = errors.New("the question was already answered")

// AnswerQuestion records the answer once.
func (s *Store) AnswerQuestion(ctx context.Context, id, by, answer string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE questions SET answered_by = ?, answer = ? WHERE id = ? AND answered_by = ''`, by, answer, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	if _, err := s.QuestionByID(ctx, id); err != nil {
		return err
	}
	return ErrAnswered
}

// Watch is a remote task started by a2a.send that outlived the call. When
// it finishes, its result is posted into the originating context.
type Watch struct {
	Contact         string
	TaskID          string
	RemoteContextID string
	OriginContextID string
	OnBehalfOf      string
	Created         time.Time
}

// PutWatch stores a watch.
func (s *Store) PutWatch(ctx context.Context, w Watch) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO watches (contact, task_id, remote_context_id, origin_context_id, on_behalf_of, created)
		VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`,
		w.Contact, w.TaskID, w.RemoteContextID, w.OriginContextID, w.OnBehalfOf, s.now().UnixNano())
	return err
}

// Watches lists watches.
func (s *Store) Watches(ctx context.Context) ([]Watch, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT contact, task_id, remote_context_id, origin_context_id, on_behalf_of, created FROM watches`)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	var out []Watch
	for rows.Next() {
		var (
			w       Watch
			created int64
		)
		if err := rows.Scan(&w.Contact, &w.TaskID, &w.RemoteContextID, &w.OriginContextID, &w.OnBehalfOf, &created); err != nil {
			return nil, err
		}
		w.Created = unixTime(created)
		out = append(out, w)
	}
	return out, rows.Err()
}

// DeleteWatch removes a watch.
func (s *Store) DeleteWatch(ctx context.Context, contact, taskID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM watches WHERE contact = ? AND task_id = ?`, contact, taskID)
	return err
}

func (s *Store) deleteOne(ctx context.Context, query string, args ...any) error {
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
