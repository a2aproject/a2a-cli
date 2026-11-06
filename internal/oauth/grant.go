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

package oauth

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"time"
)

// refreshSkew is how long before expiry a token is refreshed.
const refreshSkew = time.Minute

// GrantKey identifies a grant in the store.
type GrantKey struct {
	Issuer   string   `json:"issuer"`
	ClientID string   `json:"client_id"`
	Resource string   `json:"resource"`
	Scopes   []string `json:"scopes,omitempty"`
}

// NewGrantKey builds a key with normalized resource and sorted, deduplicated scopes.
func NewGrantKey(issuer, clientID, resource string, scopes []string) GrantKey {
	s := slices.Clone(scopes)
	slices.Sort(s)
	s = slices.Compact(s)
	if len(s) == 0 {
		s = nil
	}
	return GrantKey{
		Issuer:   strings.TrimSuffix(issuer, "/"),
		ClientID: clientID,
		Resource: NormalizeResource(resource),
		Scopes:   s,
	}
}

// ID is a stable identifier of the key.
func (k GrantKey) ID() string {
	sum := sha256.Sum256([]byte(k.Issuer + "\n" + k.ClientID + "\n" + k.Resource + "\n" + strings.Join(k.Scopes, " ")))
	return hex.EncodeToString(sum[:16])
}

// Equal reports whether two keys identify the same grant.
func (k GrantKey) Equal(o GrantKey) bool {
	return k.Issuer == o.Issuer && k.ClientID == o.ClientID && k.Resource == o.Resource && slices.Equal(k.Scopes, o.Scopes)
}

// Grant is a stored authorization: the tokens and the endpoints to refresh and
// revoke them with.
type Grant struct {
	Key          GrantKey  `json:"key"`
	AccessToken  string    `json:"access_token,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitzero"`
	Endpoints    Endpoints `json:"endpoints"`
	CreatedAt    time.Time `json:"created_at,omitzero"`
	// ClientSource records whether the client came from configuration or the CIMD default.
	ClientSource string `json:"client_source,omitempty"`
}

// expired reports whether the access token is past its expiry.
func (g *Grant) expired(now time.Time) bool {
	return !g.ExpiresAt.IsZero() && !now.Before(g.ExpiresAt)
}

// needsRefresh reports whether the access token expires within [refreshSkew].
func (g *Grant) needsRefresh(now time.Time) bool {
	return g.AccessToken == "" || (!g.ExpiresAt.IsZero() && !now.Add(refreshSkew).Before(g.ExpiresAt))
}

// header is the Authorization header value for the grant.
func (g *Grant) header() string {
	return "Bearer " + g.AccessToken
}

// requirement rebuilds the requirement the grant was obtained for.
func (g *Grant) requirement() *Requirement {
	return &Requirement{Endpoints: g.Endpoints, Resource: g.Key.Resource, Scopes: g.Key.Scopes}
}

func (g *Grant) apply(t *tokenResponse, now time.Time) {
	g.AccessToken = t.AccessToken
	if t.RefreshToken != "" {
		g.RefreshToken = t.RefreshToken
	}
	g.TokenType = t.TokenType
	g.ExpiresAt = time.Time{}
	if t.ExpiresIn > 0 {
		g.ExpiresAt = now.Add(time.Duration(t.ExpiresIn) * time.Second).UTC()
	}
}
