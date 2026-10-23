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

// Package auth turns requests to an agent server into principals: channel
// links, network identity and pairing tokens.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/a2aproject/a2a-cli/internal/channel"
	"github.com/a2aproject/a2a-cli/internal/network"
	"github.com/a2aproject/a2a-cli/internal/store"
)

// Roles.
const (
	// RoleOwner mints codes and answers approvals by default.
	RoleOwner = "owner"
	// RoleMember can talk to the agent but can't approve unless listed as an approver.
	RoleMember = "member"
)

// OwnerID is the principal of the agent's owner, whichever way they arrive.
const OwnerID = "user:owner"

// Headers set by agent servers calling each other.
const (
	HeaderOnBehalfOf = "X-A2A-On-Behalf-Of"
	HeaderHops       = "X-A2A-Hops"
)

// MaxHops limits chains of agents calling agents.
const MaxHops = 3

// TailscaleCap is the Tailscale application capability which grants a role
// to tailnet users other than the owner: {"role": "member"}.
const TailscaleCap = "a2aproject.org/cap/a2a"

// Principal is an authenticated caller.
type Principal struct {
	// ID is user:<id>, pair:<name>, agent:<name> or node:<id>.
	ID    string   `json:"id"`
	Roles []string `json:"roles,omitempty"`
	// OnBehalfOf is the user who started a chain of agent calls.
	OnBehalfOf string `json:"onBehalfOf,omitempty"`
	// Hops is the number of agents the request passed through.
	Hops int `json:"hops,omitempty"`
}

// Has reports whether the principal has a role.
func (p Principal) Has(role string) bool { return slices.Contains(p.Roles, role) }

// User is the person the principal acts for.
func (p Principal) User() string {
	if p.OnBehalfOf != "" {
		return p.OnBehalfOf
	}
	return p.ID
}

// Owner is the principal of the owner.
func Owner() Principal { return Principal{ID: OwnerID, Roles: []string{RoleOwner}} }

type principalKey struct{}

// WithPrincipal returns a context carrying p.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFrom returns the principal carried by ctx.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

// Request is what an authenticator sees of a request.
type Request struct {
	// Listener is one of the network.Listener* kinds.
	Listener   string
	RemoteAddr string
	Header     http.Header
	// Channel is set for channel messages.
	Channel *channel.Inbound
}

// Authenticator turns a request into a principal, or says it doesn't apply.
type Authenticator interface {
	// Authenticate returns the principal of r, or false if this
	// authenticator can't identify the caller.
	Authenticate(ctx context.Context, r Request) (Principal, bool, error)
}

// Chain authenticates with channel links, then network identity on the
// private listener, then pairing tokens on any listener.
type Chain struct {
	Store *store.Store
	// Net identifies callers on the private listener. Optional.
	Net network.Provider
	// Identities maps network logins of users other than the owner to roles.
	Identities map[string]string
}

var _ Authenticator = (*Chain)(nil)

// Authenticate implements [Authenticator].
func (a *Chain) Authenticate(ctx context.Context, r Request) (Principal, bool, error) {
	if r.Channel != nil {
		return a.channelPrincipal(ctx, r.Channel)
	}
	if r.Listener == network.ListenerPrivate && a.Net != nil {
		p, ok, err := a.networkPrincipal(ctx, r)
		if err != nil || ok {
			return p, ok, err
		}
	}
	if tok, ok := BearerToken(r.Header.Get("Authorization")); ok && strings.HasPrefix(tok, TokenPrefix) {
		t, err := a.Store.LookupToken(ctx, tok)
		if errors.Is(err, store.ErrNotFound) {
			return Principal{}, false, nil
		}
		if err != nil {
			return Principal{}, false, err
		}
		return Principal{ID: t.Principal, Roles: t.Roles}, true, nil
	}
	return Principal{}, false, nil
}

func (a *Chain) channelPrincipal(ctx context.Context, in *channel.Inbound) (Principal, bool, error) {
	if !in.Authentic {
		return Principal{}, false, nil
	}
	l, err := a.Store.LinkOf(ctx, in.Channel, in.Identity)
	if errors.Is(err, store.ErrNotFound) {
		return Principal{}, false, nil
	}
	if err != nil {
		return Principal{}, false, err
	}
	u, err := a.Store.User(ctx, l.UserID)
	if err != nil {
		return Principal{}, false, err
	}
	return Principal{ID: u.ID, Roles: u.Roles}, true, nil
}

func (a *Chain) networkPrincipal(ctx context.Context, r Request) (Principal, bool, error) {
	peer, err := a.Net.WhoIs(ctx, r.RemoteAddr)
	if errors.Is(err, network.ErrNoIdentity) {
		return Principal{}, false, nil
	}
	if err != nil {
		return Principal{}, false, err
	}
	var p Principal
	switch {
	case peer.Self:
		p = Owner()
	case a.Identities[peer.ID] != "":
		p = Principal{ID: "user:" + peer.ID, Roles: []string{a.Identities[peer.ID]}}
	default:
		role := capRole(peer.Caps[TailscaleCap])
		if role == "" {
			return Principal{}, false, nil
		}
		p = Principal{ID: "user:" + peer.ID, Roles: []string{role}}
	}
	// Only a verified network caller can claim to act for someone, because
	// only then is the caller an agent server of a known user.
	p.OnBehalfOf = r.Header.Get(HeaderOnBehalfOf)
	if h, err := strconv.Atoi(r.Header.Get(HeaderHops)); err == nil {
		p.Hops = h
	}
	return p, true, nil
}

func capRole(values []json.RawMessage) string {
	for _, v := range values {
		var c struct {
			Role string `json:"role"`
		}
		if json.Unmarshal(v, &c) == nil && (c.Role == RoleOwner || c.Role == RoleMember) {
			return c.Role
		}
	}
	return ""
}

// BearerToken extracts the token of an Authorization header.
func BearerToken(h string) (string, bool) {
	scheme, tok, ok := strings.Cut(strings.TrimSpace(h), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	tok = strings.TrimSpace(tok)
	return tok, tok != ""
}

// Pairing.

// TokenPrefix marks pairing tokens.
const TokenPrefix = "a2p_"

// CodeTTL is how long pairing codes are valid.
const CodeTTL = 15 * time.Minute

// Code purposes.
const (
	PurposePair = "pair"
	PurposeNode = "node"
)

// codeAlphabet avoids characters that are easy to confuse when read out.
const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// NewCode returns an 8-character code like K7Q2-9XF4.
func NewCode() string {
	s := store.RandomString(8, codeAlphabet)
	return s[:4] + "-" + s[4:]
}

// NormalizeCode makes codes case- and dash-insensitive.
func NormalizeCode(code string) string {
	return strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(code)))
}

// NewToken returns a new pairing token.
func NewToken() string { return TokenPrefix + store.RandomString(40, store.Alphanumeric) }

// PrincipalForCode is the principal a pairing code creates.
func PrincipalForCode(name, role string) string {
	if role == RoleOwner {
		return OwnerID
	}
	return "pair:" + name
}

// MintCode stores a new single-use code and returns it.
func MintCode(ctx context.Context, s *store.Store, purpose, name, role string) (string, time.Time, error) {
	code := NewCode()
	expires := time.Now().Add(CodeTTL)
	if err := s.PutCode(ctx, NormalizeCode(code), store.Code{Purpose: purpose, Name: name, Role: role, Expires: expires}); err != nil {
		return "", time.Time{}, err
	}
	return code, expires, nil
}

// ErrInvalidCode is returned for unknown, used or expired codes.
var ErrInvalidCode = errors.New("invalid or expired code")

// IssueToken issues a bearer token at agentURL for a taken pair code.
func IssueToken(ctx context.Context, s *store.Store, c store.Code, agentURL string) (string, Principal, error) {
	p := Principal{ID: PrincipalForCode(c.Name, c.Role), Roles: []string{c.Role}}
	if _, err := s.EnsureUser(ctx, p.ID, p.Roles); err != nil {
		return "", Principal{}, err
	}
	tok := NewToken()
	if err := s.PutToken(ctx, tok, store.Token{Name: c.Name, Principal: p.ID, Roles: p.Roles, URL: agentURL}); err != nil {
		return "", Principal{}, err
	}
	return tok, p, nil
}

var linkCommand = regexp.MustCompile(`(?i)^\s*/?(?:link|start)(?:@\S+)?\s+([A-Za-z0-9-]{8,9})\s*$`)

// LinkCode returns the code of a "link <code>" or "/start <code>" message.
func LinkCode(text string) (string, bool) {
	m := linkCommand.FindStringSubmatch(text)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// Link consumes a pairing code sent from a channel and links the sender's
// identity to the code's principal.
func Link(ctx context.Context, s *store.Store, in *channel.Inbound, code string) (Principal, error) {
	if !in.Authentic {
		return Principal{}, fmt.Errorf("the sender of this message could not be verified")
	}
	c, err := s.TakeCode(ctx, NormalizeCode(code), PurposePair)
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrCodeExpired) {
		return Principal{}, ErrInvalidCode
	}
	if err != nil {
		return Principal{}, err
	}
	p := Principal{ID: PrincipalForCode(c.Name, c.Role), Roles: []string{c.Role}}
	if _, err := s.EnsureUser(ctx, p.ID, p.Roles); err != nil {
		return Principal{}, err
	}
	l := store.Link{Channel: in.Channel, Identity: in.Identity, UserID: p.ID}
	if in.Direct {
		l.Conv = string(in.Conv)
	}
	return p, s.PutLink(ctx, l)
}

// CanApprove reports whether p may answer tool permission requests.
func CanApprove(p Principal, approvers []string) bool {
	return p.Has(RoleOwner) || slices.Contains(approvers, p.ID)
}
