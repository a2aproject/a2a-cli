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
	"fmt"
	"strings"
)

// InteractionRequiredError reports that a request needs a grant that has to be
// obtained with user interaction first.
type InteractionRequiredError struct {
	// Requirement describes where and for what to obtain the grant.
	Requirement *Requirement
	// Reason explains why a stored grant could not be used; empty when there was none.
	Reason string
}

// Error implements the error interface.
func (e *InteractionRequiredError) Error() string {
	msg := fmt.Sprintf("%s requires authorization at %s", e.Requirement.Resource, e.Requirement.Endpoints.Issuer)
	if e.Reason != "" {
		msg += ": " + e.Reason
	}
	return msg
}

// FailureError reports a failed step of the authorization process: discovery,
// metadata validation or token refresh.
type FailureError struct {
	Err error
}

// Error implements the error interface.
func (e *FailureError) Error() string { return e.Err.Error() }

// Unwrap returns the cause.
func (e *FailureError) Unwrap() error { return e.Err }

func failure(format string, args ...any) error {
	return &FailureError{Err: fmt.Errorf(format, args...)}
}

// NoClientError reports that no client registration can be used with an
// authorization server.
type NoClientError struct {
	// Issuers are the authorization servers that were tried.
	Issuers []string
}

// Error implements the error interface.
func (e *NoClientError) Error() string {
	example := `{"authority":"<issuer>","client_id":"<client id or CIMD URL>"}`
	if len(e.Issuers) > 0 {
		example = fmt.Sprintf(`{"authority":%q,"client_id":"<client id or CIMD URL>"}`, e.Issuers[0])
	}
	return fmt.Sprintf("no client in oauth-clients matches the authorization server %s; add %s to oauth-clients",
		strings.Join(e.Issuers, ", "), example)
}

// Error is an OAuth error response from an authorization server (RFC 6749 §5.2).
type Error struct {
	Code        string `json:"error"`
	Description string `json:"error_description,omitempty"`
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Description != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Description)
	}
	return e.Code
}
