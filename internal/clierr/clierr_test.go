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

package clierr

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"syscall"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
)

func TestClassify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		wantCode string
		wantA2A  string
		wantExit int
	}{
		{
			name:     "already-classified usage error passes through",
			err:      Usage("bad flag combo"),
			wantCode: CodeUsage,
			wantExit: 2,
		},
		{
			name:     "already-classified card error passes through",
			err:      CardResolution(errors.New("not found")),
			wantCode: CodeCardInvalid,
			wantExit: 4,
		},
		{
			name:     "card resolution deadline exceeded is a timeout",
			err:      CardResolution(context.DeadlineExceeded),
			wantCode: CodeTimeout,
			wantExit: 5,
		},
		{
			name:     "deadline exceeded is a timeout",
			err:      fmt.Errorf("waiting: %w", context.DeadlineExceeded),
			wantCode: CodeTimeout,
			wantExit: 5,
		},
		{
			name:     "protocol error carries the A2A reason in a2aCode, exit 1",
			err:      fmt.Errorf("failed to get task x: %w", a2a.NewError(a2a.ErrTaskNotFound, "task not found")),
			wantCode: CodeProtocol,
			wantA2A:  "TASK_NOT_FOUND",
			wantExit: 1,
		},
		{
			name:     "auth is a protocol error, not a special case",
			err:      fmt.Errorf("send: %w", a2a.NewError(a2a.ErrUnauthenticated, "unauthenticated")),
			wantCode: CodeProtocol,
			wantA2A:  "UNAUTHENTICATED",
			wantExit: 1,
		},
		{
			name:     "net error is io, exit 3",
			err:      fmt.Errorf("resolving agent card: %w", &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}),
			wantCode: CodeIO,
			wantExit: 3,
		},
		{
			name:     "connection reset sentinel is io, exit 3",
			err:      fmt.Errorf("reading stream: %w", syscall.ECONNRESET),
			wantCode: CodeIO,
			wantExit: 3,
		},
		{
			name:     "local file error is not io, exit 1",
			err:      fmt.Errorf("failed to save file parts: %w", &fs.PathError{Op: "open", Path: "out/x.txt", Err: syscall.EACCES}),
			wantCode: CodeInternal,
			wantExit: 1,
		},
		{
			name:     "broken pipe on a file is still io",
			err:      fmt.Errorf("writing to plugin: %w", &fs.PathError{Op: "write", Path: "|1", Err: syscall.EPIPE}),
			wantCode: CodeIO,
			wantExit: 3,
		},
		{
			name:     "unclassified error is internal, exit 1",
			err:      errors.New("something odd happened"),
			wantCode: CodeInternal,
			wantExit: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Classify(tt.err)
			if got == nil {
				t.Fatalf("Classify(%v) = nil, want a classified error", tt.err)
			}
			if got.Code != tt.wantCode {
				t.Errorf("Classify(%v).Code = %q, want %q", tt.err, got.Code, tt.wantCode)
			}
			if got.A2ACode != tt.wantA2A {
				t.Errorf("Classify(%v).A2ACode = %q, want %q", tt.err, got.A2ACode, tt.wantA2A)
			}
			if got.Exit != tt.wantExit {
				t.Errorf("Classify(%v).Exit = %d, want %d", tt.err, got.Exit, tt.wantExit)
			}
		})
	}
}

func TestClassifyNil(t *testing.T) {
	t.Parallel()
	if got := Classify(nil); got != nil {
		t.Fatalf("Classify(nil) = %v, want nil", got)
	}
}

func TestUsage(t *testing.T) {
	t.Parallel()
	got := Usage("bad flags")
	if got.Code != CodeUsage {
		t.Errorf("Usage().Code = %q, want %q", got.Code, CodeUsage)
	}
	if got.Exit != 2 {
		t.Errorf("Usage().Exit = %d, want 2", got.Exit)
	}
	if got.Hint != UsageHint {
		t.Errorf("Usage().Hint = %q, want %q", got.Hint, UsageHint)
	}
}

func TestCardResolution(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		wantCode string
		wantExit int
		wantHint string
	}{
		{
			name:     "transport failure is io",
			err:      fmt.Errorf("card request failed: %w", &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}),
			wantCode: CodeIO,
			wantExit: 3,
			wantHint: "check the agent is running and the --agent-card reference is correct",
		},
		{
			name:     "non-transport failure is card-invalid",
			err:      errors.New("card request failed, status: 404 Not Found"),
			wantCode: CodeCardInvalid,
			wantExit: 4,
			wantHint: "check the --agent-card reference (host, full card URL, or local file path)",
		},
		{
			name:     "unauthorized card request asks for credentials",
			err:      &agentcard.ErrStatusNotOK{StatusCode: 401, Status: "401 Unauthorized"},
			wantCode: CodeCardInvalid,
			wantExit: 4,
			wantHint: "pass credentials with --auth, --svc-param or --credential-helper",
		},
		{
			name:     "forbidden card request asks for credentials",
			err:      &agentcard.ErrStatusNotOK{StatusCode: 403, Status: "403 Forbidden"},
			wantCode: CodeCardInvalid,
			wantExit: 4,
			wantHint: "pass credentials with --auth, --svc-param or --credential-helper",
		},
		{
			name:     "server error on card request points at the agent",
			err:      &agentcard.ErrStatusNotOK{StatusCode: 500, Status: "500 Internal Server Error"},
			wantCode: CodeCardInvalid,
			wantExit: 4,
			wantHint: "the agent failed to serve its card, check its logs or retry",
		},
		{
			name:     "deadline exceeded is a timeout",
			err:      context.DeadlineExceeded,
			wantCode: CodeTimeout,
			wantExit: 5,
			wantHint: "increase --timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := CardResolution(tt.err)
			if got.Code != tt.wantCode {
				t.Fatalf("CardResolution(%v).Code = %q, want %q", tt.err, got.Code, tt.wantCode)
			}
			if got.Exit != tt.wantExit {
				t.Fatalf("CardResolution(%v).Exit = %d, want %d", tt.err, got.Exit, tt.wantExit)
			}
			if got.Hint != tt.wantHint {
				t.Fatalf("CardResolution(%v).Hint = %q, want %q", tt.err, got.Hint, tt.wantHint)
			}
			if !errors.Is(got, tt.err) {
				t.Fatalf("CardResolution(%v) does not wrap the cause", tt.err)
			}
		})
	}
}
