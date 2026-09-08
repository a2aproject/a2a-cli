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

package transportplugin

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/a2aproject/a2a-cli/devkit/clitransport"
	"github.com/a2aproject/a2a-cli/internal/testutil"
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
)

func TestFactoryProxiesThroughPlugin(t *testing.T) {
	t.Parallel()

	for _, binding := range []a2a.TransportProtocol{a2a.TransportProtocolJSONRPC, a2a.TransportProtocolHTTPJSON, a2a.TransportProtocolGRPC} {
		t.Run(string(binding), func(t *testing.T) {
			t.Parallel()

			upstream := &echoUpstream{}
			hs, stopServer := startDevkitServer(t, binding, upstream)

			isClosed := false
			session := &session{handshake: hs, closeFunc: func() error {
				isClosed = true
				stopServer()
				return nil
			}}
			factory := newPluginTransportFactory("a2a-transport-fake", fakeLauncher{session: session})

			transport, err := factory.Create(t.Context(), nil, a2a.NewAgentInterface("fake://upstream", binding))
			if err != nil {
				t.Fatalf("factory.Create() error = %v", err)
			}

			params := a2aclient.ServiceParams{"authorization": {"Bearer secret"}}
			msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("ping"))
			result, err := transport.SendMessage(t.Context(), params, &a2a.SendMessageRequest{Message: msg})
			if err != nil {
				t.Fatalf("transport.SendMessage() error = %v", err)
			}
			task, ok := result.(*a2a.Task)
			if !ok {
				t.Fatalf("transport.SendMessage() result type = %T, want *a2a.Task", result)
			}
			if got := testutil.AllArtifactText(task); got != "ping" {
				t.Fatalf("transport.SendMessage() echoed = %q, want %q", got, "ping")
			}
			if got := upstream.lastAuth(); got != "Bearer secret" {
				t.Fatalf("upstream authorization = %q, want %q", got, "Bearer secret")
			}
			if got := upstream.tokenSeen(); got {
				t.Fatal("upstream saw the loopback plugin token; it must not be forwarded upstream")
			}

			if err := transport.Destroy(); err != nil {
				t.Fatalf("transport.Destroy() error = %v", err)
			}
			if !isClosed {
				t.Fatal("transport.Destroy() did not close the plugin session")
			}
		})
	}
}

func TestBaseTransportCredentialSelection(t *testing.T) {
	t.Parallel()

	t.Run("http uses default client without cert", func(t *testing.T) {
		t.Parallel()
		client, err := httpClient("")
		if err != nil {
			t.Fatalf("httpClient(empty) error = %v", err)
		}
		if client != nil {
			t.Fatalf("httpClient(empty) = %v, want nil (built-in default client)", client)
		}
	})

	t.Run("http rejects invalid cert", func(t *testing.T) {
		t.Parallel()
		if _, err := httpClient("not-a-pem-cert"); err == nil {
			t.Fatal("httpClient(invalid) error = nil, want error")
		}
	})

	t.Run("grpc uses insecure creds without cert", func(t *testing.T) {
		t.Parallel()
		creds, err := grpcCreds("")
		if err != nil {
			t.Fatalf("grpcCreds(empty) error = %v", err)
		}
		if got := creds.Info().SecurityProtocol; got != "insecure" {
			t.Fatalf("grpcCreds(empty) security = %q, want %q", got, "insecure")
		}
	})

	t.Run("grpc rejects invalid cert", func(t *testing.T) {
		t.Parallel()
		if _, err := grpcCreds("not-a-pem-cert"); err == nil {
			t.Fatal("grpcCreds(invalid) error = nil, want error")
		}
	})
}

// startDevkitServer runs a real devkit plugin proxy in-process for the given
// binding and returns its handshake payload plus a stop function.
func startDevkitServer(t *testing.T, binding a2a.TransportProtocol, upstream a2aclient.Transport) (*clitransport.Endpoint, func()) {
	t.Helper()

	stdoutR, stdoutW := io.Pipe()
	_, stdinW := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())

	cfg := clitransport.Config{
		Name:    "fake",
		Version: "1.0.0",
		NewTransport: func(context.Context, string) (a2aclient.Transport, error) {
			return upstream, nil
		},
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		ios := &clitransport.IO{Out: stdoutW}
		_ = clitransport.Run(ctx, cfg, []string{clitransport.SubcommandServe, "--endpoint", "fake://upstream", "--bind", string(binding)}, ios)
	}()

	line, err := bufio.NewReader(stdoutR).ReadString('\n')
	if err != nil {
		cancel()
		t.Fatalf("reading handshake error = %v", err)
	}
	var hs clitransport.Handshake
	if err := json.Unmarshal([]byte(line), &hs); err != nil {
		cancel()
		t.Fatalf("json.Unmarshal(handshake) error = %v", err)
	}
	if !hs.Success || hs.Endpoint == nil {
		cancel()
		t.Fatalf("plugin handshake unsuccessful: %+v", hs)
	}

	stop := func() {
		cancel()
		_ = stdinW.Close()
		_ = stdoutR.Close()
		<-done
	}
	return hs.Endpoint, stop
}

type fakeLauncher struct {
	session *session
}

func (l fakeLauncher) launch(context.Context, string, string) (*session, error) {
	return l.session, nil
}

// echoUpstream is a fake custom transport that echoes messages and records the
// service params it observed.
type echoUpstream struct {
	a2aclient.Transport
	mu    sync.Mutex
	auth  string
	token bool
}

func (u *echoUpstream) lastAuth() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.auth
}

func (u *echoUpstream) tokenSeen() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.token
}

func (u *echoUpstream) SendMessage(_ context.Context, params a2aclient.ServiceParams, req *a2a.SendMessageRequest) (a2a.SendMessageResult, error) {
	u.mu.Lock()
	if auth := params.Get("authorization"); len(auth) > 0 {
		u.auth = auth[0]
	}
	if tok := params.Get(clitransport.TokenSvcParam); len(tok) > 0 {
		u.token = true
	}
	u.mu.Unlock()

	var text strings.Builder
	if req.Message != nil {
		for _, p := range req.Message.Parts {
			text.WriteString(p.Text())
		}
	}
	return &a2a.Task{
		ID:        a2a.NewTaskID(),
		ContextID: a2a.NewContextID(),
		Status:    a2a.TaskStatus{State: a2a.TaskStateCompleted},
		Artifacts: []*a2a.Artifact{{ID: a2a.NewArtifactID(), Parts: a2a.ContentParts{a2a.NewTextPart(text.String())}}},
	}, nil
}

func (u *echoUpstream) Destroy() error { return nil }
