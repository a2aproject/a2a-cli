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
	"net/url"
	"os"
	"strings"
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
			name:     "local link error is not io, exit 1",
			err:      &os.LinkError{Op: "rename", Old: "a", New: "b", Err: syscall.EXDEV},
			wantCode: CodeInternal,
			wantExit: 1,
		},
		{
			name:     "local syscall error is not io, exit 1",
			err:      os.NewSyscallError("fsync", syscall.ENOSPC),
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

func TestClassifyPlaintextServer(t *testing.T) {
	t.Parallel()

	const handshake = `connection error: desc = "transport: authentication handshake failed: tls: first record does not look like a TLS handshake"`
	tests := []struct {
		name string
		err  error
	}{
		{name: "gRPC status converted to an a2a error", err: fmt.Errorf("failed to send message: %w", a2a.NewError(a2a.ErrInternalError, handshake))},
		{name: "transport error", err: fmt.Errorf("failed to send message: %w", errors.New(handshake))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Classify(tt.err)
			if got.Code != CodeIO {
				t.Fatalf("Classify(%v).Code = %q, want %q", tt.err, got.Code, CodeIO)
			}
			if got.Exit != 3 {
				t.Fatalf("Classify(%v).Exit = %d, want 3", tt.err, got.Exit)
			}
			if got.Hint != plaintextServerHint {
				t.Fatalf("Classify(%v).Hint = %q, want %q", tt.err, got.Hint, plaintextServerHint)
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

func TestAuthErrors(t *testing.T) {
	t.Parallel()
	cause := errors.New("cause")
	tests := []struct {
		name     string
		err      *Error
		wantCode string
	}{
		{name: "credentials missing", err: CredentialsMissing("needs sign-in", "run a2a auth login", cause), wantCode: CodeCredentialsMissing},
		{name: "auth failed", err: AuthFailed("token rejected", cause), wantCode: CodeAuthFailed},
	}
	for _, tc := range tests {
		if tc.err.Code != tc.wantCode || tc.err.Exit != 4 {
			t.Errorf("%s: code %q exit %d, want %q exit 4", tc.name, tc.err.Code, tc.err.Exit, tc.wantCode)
		}
		if !errors.Is(tc.err, cause) {
			t.Errorf("%s: error does not wrap its cause", tc.name)
		}
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
			wantHint: "pass or check credentials (--auth, --svc-param or --credential-helper); if the agent uses OAuth, run `a2a auth login` and retry with --oauth auto",
		},
		{
			name:     "forbidden card request asks for credentials",
			err:      &agentcard.ErrStatusNotOK{StatusCode: 403, Status: "403 Forbidden"},
			wantCode: CodeCardInvalid,
			wantExit: 4,
			wantHint: "pass or check credentials (--auth, --svc-param or --credential-helper); if the agent uses OAuth, run `a2a auth login` and retry with --oauth auto",
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

func TestErrorMessageRedactsURL(t *testing.T) {
	t.Parallel()

	refused := &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
	// net/http masks the password of the URL it reports, but not the query.
	getErr := &url.Error{Op: "Get", URL: "http://bob:***@127.0.0.1:1/.well-known/agent-card.json?token=S3CRETQ", Err: refused}
	// url.Parse reports the URL as given.
	parseErr := &url.Error{Op: "parse", URL: "http://bob:S3CRETPW@127.0.0.1:x/?token=S3CRETQ", Err: errors.New(`invalid port ":x" after host`)}

	tests := []struct {
		name string
		err  *Error
		want string
	}{
		{
			name: "card request failure",
			err:  CardResolution(fmt.Errorf("card request failed: %w", getErr)),
			want: `failed to resolve agent card: card request failed: Get "http://bob:***@127.0.0.1:1/.well-known/agent-card.json": dial tcp: connection refused`,
		},
		{
			name: "card request timeout",
			err:  CardResolution(fmt.Errorf("card request failed: %w", &url.Error{Op: "Get", URL: getErr.URL, Err: context.DeadlineExceeded})),
			want: `failed to resolve agent card: card request failed: Get "http://bob:***@127.0.0.1:1/.well-known/agent-card.json": context deadline exceeded`,
		},
		{
			name: "request failure",
			err:  Classify(fmt.Errorf("failed to send message: %w", getErr)),
			want: `failed to send message: Get "http://bob:***@127.0.0.1:1/.well-known/agent-card.json": dial tcp: connection refused`,
		},
		{
			name: "unparsable url",
			err:  Classify(fmt.Errorf("failed to create HTTP request: %w", parseErr)),
			want: `failed to create HTTP request: parse "http://bob:***@127.0.0.1:x/": invalid port ":x" after host`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.err.Message != tt.want {
				t.Errorf("Message = %q, want %q", tt.err.Message, tt.want)
			}
			if strings.Contains(tt.err.Message, "S3CRET") {
				t.Errorf("Message %q contains a secret", tt.err.Message)
			}
		})
	}
}

func TestRedactURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw  string
		want string
	}{
		{raw: "https://agent.example/card.json", want: "https://agent.example/card.json"},
		{raw: "https://agent.example?token=abc", want: "https://agent.example"},
		{raw: "https://agent.example/?a=1&b=2", want: "https://agent.example/"},
		{raw: "https://bob:pw@agent.example/x?token=abc", want: "https://bob:***@agent.example/x"},
		{raw: "https://bob@agent.example/", want: "https://bob@agent.example/"},
		{raw: "https://agent.example/a@b:c", want: "https://agent.example/a@b:c"},
		{raw: "agent.example?token=abc", want: "agent.example"},
	}

	for _, tt := range tests {
		if got := RedactURL(tt.raw); got != tt.want {
			t.Errorf("RedactURL(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}
