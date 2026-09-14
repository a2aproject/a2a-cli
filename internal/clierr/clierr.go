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

// Package clierr classifies errors into the a2a-cli error contract,
package clierr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"syscall"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

// Code is a symbolic error identifier.
type Code string

const (
	// CodeUsage marks invalid arguments, flags, or flag combinations; exit 2.
	CodeUsage = "A2ACLI_ERR_USAGE"
	// CodeProtocol marks a failure the agent reported as an a2a.Error; the
	// specific A2A reason travels in the A2ACode field. Exit 1.
	CodeProtocol = "A2ACLI_ERR_PROTOCOL"
	// CodeIO marks a transport failure reaching or reading from the agent —
	// DNS, connection, TLS, reset, broken pipe. Exit 3.
	CodeIO = "A2ACLI_ERR_IO"
	// CodeCardInvalid marks a card that was reached but is not usable — a
	// non-OK status or a malformed body. Exit 4.
	CodeCardInvalid = "A2ACLI_ERR_CARD_INVALID"
	// CodeTimeout marks a --timeout that expired before a terminal state; exit 5.
	CodeTimeout = "A2ACLI_ERR_TIMEOUT"
	// CodeInternal marks an unexpected CLI-side failure, or any condition with
	// no better code. Exit 1.
	CodeInternal = "A2ACLI_ERR_INTERNAL"
)

// Error is a classified CLI failure.
type Error struct {
	// Code is the symbolic identifier reported to callers.
	Code string
	// Message is the human-readable, single-line failure description.
	Message string
	// Hint is optional remediation advice shown to the user.
	Hint string
	// A2ACode is the A2A protocol error reason, set only for protocol failures.
	A2ACode string
	// Exit is the process exit status for this failure.
	Exit int

	// err is the wrapped cause, exposed via Unwrap.
	err error
}

// Error returns the failure message, satisfying the error interface.
func (e *Error) Error() string { return e.Message }

// Unwrap returns the wrapped cause so errors.Is and errors.As can inspect it.
func (e *Error) Unwrap() error { return e.err }

// Usage builds a usage error for invalid arguments, flags, or flag combinations.
func Usage(msg string) *Error {
	return &Error{Code: CodeUsage, Message: msg, Exit: 2}
}

// CardResolution builds an error for a failure to resolve an --agent-card.
//
// It is called only from the card-resolution path, where the reference itself
// is already known to be well-formed (see flagparse.URLOrPath.Validate), so the
// outcome is binary: either the agent could not be reached (an IO failure), or
// it was reached but did not yield a usable card.
func CardResolution(err error) *Error {
	msg := fmt.Sprintf("failed to resolve agent card: %v", err)
	if isIOError(err) {
		return &Error{
			Code:    CodeIO,
			Message: msg,
			Exit:    3,
			err:     err,
			Hint:    "check the agent is running and the --agent-card reference is correct",
		}
	}
	return &Error{
		Code:    CodeCardInvalid,
		Message: msg,
		Exit:    4,
		Hint:    "check the --agent-card reference (host, full card URL, or local file path)",
		err:     err,
	}
}

// MarshalJSON implements [json.Marshaler].
func (e *Error) MarshalJSON() ([]byte, error) {
	type body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Hint    string `json:"hint,omitempty"`
		A2ACode string `json:"a2aCode,omitempty"`
	}
	type wrapper struct {
		Error body `json:"error"`
	}
	return json.Marshal(wrapper{Error: body{
		Code:    e.Code,
		Message: e.Message,
		A2ACode: e.A2ACode,
		Hint:    e.Hint,
	}})
}

// Classify maps an arbitrary error to a classified CLI Error with a stable code
// and exit status. It returns nil for a nil error and passes an already
// classified *Error (e.g. from Usage or CardResolution) through unchanged.
func Classify(err error) *Error {
	if err == nil {
		return nil
	}

	var classified *Error
	if errors.As(err, &classified) {
		return classified
	}

	msg := err.Error()

	if errors.Is(err, context.DeadlineExceeded) {
		return &Error{
			Code:    CodeTimeout,
			Message: msg,
			Exit:    5,
			err:     err,
			Hint:    "increase --timeout, or start the task with --async and follow it later",
		}
	}

	var a2aErr *a2a.Error
	if errors.As(err, &a2aErr) {
		return &Error{
			Code:    CodeProtocol,
			Message: msg,
			A2ACode: a2a.ErrorReason(a2aErr.Err),
			Exit:    1,
			err:     err,
		}
	}

	if isIOError(err) {
		return &Error{
			Code:    CodeIO,
			Message: msg,
			Exit:    3,
			Hint:    "check connectivity or whether the agent is running",
			err:     err,
		}
	}

	return &Error{Code: CodeInternal, Message: msg, Exit: 1, err: err}
}

// isIOError reports whether err is a transport-level failure — a network error,
// a broken stream, or a reset/closed connection — identified by type and
// sentinel rather than by matching message text.
func isIOError(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	return errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, io.ErrClosedPipe) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EPIPE)
}
