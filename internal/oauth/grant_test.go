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
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestGrantExpiry(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name             string
		grant            Grant
		wantExpired      bool
		wantNeedsRefresh bool
	}{
		{name: "no expiry", grant: Grant{AccessToken: "at"}},
		{name: "valid", grant: Grant{AccessToken: "at", ExpiresAt: now.Add(time.Hour)}},
		{name: "within refresh skew", grant: Grant{AccessToken: "at", ExpiresAt: now.Add(refreshSkew / 2)}, wantNeedsRefresh: true},
		{name: "expired", grant: Grant{AccessToken: "at", ExpiresAt: now}, wantExpired: true, wantNeedsRefresh: true},
		{name: "no access token", grant: Grant{RefreshToken: "rt"}, wantNeedsRefresh: true},
	}
	for _, tc := range tests {
		if got := tc.grant.expired(now); got != tc.wantExpired {
			t.Errorf("%s: expired() = %v, want %v", tc.name, got, tc.wantExpired)
		}
		if got := tc.grant.needsRefresh(now); got != tc.wantNeedsRefresh {
			t.Errorf("%s: needsRefresh() = %v, want %v", tc.name, got, tc.wantNeedsRefresh)
		}
	}
}

func TestGrantApply(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	g := Grant{
		Key:          NewGrantKey("https://as.example.com", "cli", "https://agent.example.com", []string{"read"}),
		AccessToken:  "old",
		RefreshToken: "rt",
		ExpiresAt:    now,
		Endpoints:    Endpoints{Issuer: "https://as.example.com", Token: "https://as.example.com/token"},
	}

	// A response without a refresh token keeps the current one.
	g.apply(&tokenResponse{AccessToken: "new", TokenType: "Bearer", ExpiresIn: 60}, now)
	if g.AccessToken != "new" || g.RefreshToken != "rt" || !g.ExpiresAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("apply() = %+v, want the new access token, the old refresh token and expiry in a minute", g)
	}
	if got := g.header(); got != "Bearer new" {
		t.Fatalf("header() = %q, want %q", got, "Bearer new")
	}

	// A rotated refresh token replaces it; no expires_in clears the expiry.
	g.apply(&tokenResponse{AccessToken: "newer", RefreshToken: "rt2"}, now)
	if g.RefreshToken != "rt2" || !g.ExpiresAt.IsZero() {
		t.Fatalf("apply() = %+v, want the rotated refresh token and no expiry", g)
	}

	want := &Requirement{Endpoints: g.Endpoints, Resource: "https://agent.example.com", Scopes: []string{"read"}}
	if diff := cmp.Diff(want, g.requirement()); diff != "" {
		t.Fatalf("requirement() wrong result (-want +got) diff = %s", diff)
	}
}

func TestFailure(t *testing.T) {
	t.Parallel()
	err := failure("refreshing the grant for %s: %w", "https://agent.example.com", io.ErrUnexpectedEOF)
	var fe *FailureError
	if !errors.As(err, &fe) || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("failure() = %#v, want a *FailureError wrapping the cause", err)
	}
	if want := "refreshing the grant for https://agent.example.com: unexpected EOF"; err.Error() != want {
		t.Fatalf("failure().Error() = %q, want %q", err.Error(), want)
	}
}
