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

package clitransport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
)

func TestRunInfo(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cfg := Config{Name: "demo", Version: "2.1.0", Description: "demo plugin", NewTransport: newRecordingTransport}
	fds := &IO{Out: &buf}
	if err := Run(context.Background(), cfg, []string{SubcommandInfo}, fds); err != nil {
		t.Fatalf("Run(info) error = %v", err)
	}

	var got Info
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(info) error = %v", err)
	}
	want := Info{Name: "demo", Version: "2.1.0", Description: "demo plugin", Protocol: a2a.Version, Binding: a2a.TransportProtocolGRPC}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("Run(info) wrong result (-want +got) diff = %s", diff)
	}
}

func TestRunValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  Config
		args []string
	}{
		{"missing name", Config{NewTransport: newRecordingTransport}, []string{SubcommandInfo}},
		{"missing transport factory", Config{Name: "demo"}, []string{SubcommandInfo}},
		{"no subcommand", Config{Name: "demo", NewTransport: newRecordingTransport}, nil},
		{"unknown subcommand", Config{Name: "demo", NewTransport: newRecordingTransport}, []string{"bogus"}},
		{"serve without endpoint", Config{Name: "demo", NewTransport: newRecordingTransport}, []string{SubcommandServe}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := Run(context.Background(), tt.cfg, tt.args, nil); err == nil {
				t.Fatalf("Run(%v) error = nil, want error", tt.args)
			}
		})
	}
}

func TestServeRoundTrip(t *testing.T) {
	t.Parallel()

	for _, binding := range []string{string(a2a.TransportProtocolJSONRPC), string(a2a.TransportProtocolHTTPJSON)} {
		t.Run(binding, func(t *testing.T) {
			t.Parallel()
			rec := &recordingTransport{}
			hs, stop := startPlugin(t, Config{
				Name:    "demo",
				Version: "1.0.0",
				NewTransport: func(context.Context, string) (a2aclient.Transport, error) {
					return rec, nil
				},
			}, binding)
			defer stop()

			client := newTokenClient(t, hs)
			defer func() { _ = client.Destroy() }()

			params := a2aclient.ServiceParams{"authorization": {"Bearer secret"}}
			msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("ping"))
			result, err := client.SendMessage(t.Context(), params, &a2a.SendMessageRequest{Message: msg})
			if err != nil {
				t.Fatalf("client.SendMessage() error = %v", err)
			}
			task, ok := result.(*a2a.Task)
			if !ok {
				t.Fatalf("client.SendMessage() result type = %T, want *a2a.Task", result)
			}
			if got := artifactText(task); got != "ping" {
				t.Fatalf("client.SendMessage() echoed = %q, want %q", got, "ping")
			}
			if got := rec.lastAuth(); got != "Bearer secret" {
				t.Fatalf("upstream transport saw authorization = %q, want %q", got, "Bearer secret")
			}
		})
	}
}

func TestServeReportsStartupFailure(t *testing.T) {
	t.Parallel()

	cfg := Config{
		Name: "demo",
		NewTransport: func(context.Context, string) (a2aclient.Transport, error) {
			return nil, fmt.Errorf("boom")
		},
	}

	var out bytes.Buffer
	ios := &IO{Out: &out}
	err := Run(context.Background(), cfg, []string{SubcommandServe, "--endpoint", "test://upstream"}, ios)
	if err == nil {
		t.Fatal("Run(serve) error = nil, want a startup failure")
	}

	var hs Handshake
	if uerr := json.Unmarshal(out.Bytes(), &hs); uerr != nil {
		t.Fatalf("json.Unmarshal(handshake) error = %v (raw=%q)", uerr, out.String())
	}
	if hs.Success {
		t.Fatalf("handshake Success = true, want false (raw=%q)", out.String())
	}
	if hs.Payload != nil {
		t.Fatalf("handshake Payload = %+v, want nil on failure", hs.Payload)
	}
	if !strings.Contains(hs.Error, "boom") {
		t.Fatalf("handshake Error = %q, want it to mention the upstream failure", hs.Error)
	}
}

func TestServeRejectsMissingToken(t *testing.T) {
	t.Parallel()

	hs, stop := startPlugin(t, Config{
		Name:         "demo",
		NewTransport: newRecordingTransport,
	}, string(a2a.TransportProtocolJSONRPC))
	defer stop()

	// Trust the certificate but omit the per-launch token: the proxy must reject
	// the request for the missing token rather than fail the TLS handshake.
	httpClient := &http.Client{Timeout: 5 * time.Second, Transport: trustingTransport(t, hs.CertPEM)}
	client := a2aclient.NewJSONRPCTransport(hs.Address, httpClient)
	defer func() { _ = client.Destroy() }()

	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("ping"))
	if _, err := client.SendMessage(t.Context(), nil, &a2a.SendMessageRequest{Message: msg}); err == nil {
		t.Fatal("SendMessage() without token error = nil, want a rejection")
	}
}

func TestServeAdvertisesCertificate(t *testing.T) {
	t.Parallel()

	bindings := []string{
		string(a2a.TransportProtocolJSONRPC),
		string(a2a.TransportProtocolHTTPJSON),
		string(a2a.TransportProtocolGRPC),
	}
	for _, binding := range bindings {
		t.Run(binding, func(t *testing.T) {
			t.Parallel()
			hs, stop := startPlugin(t, Config{Name: "demo", NewTransport: newRecordingTransport}, binding)
			defer stop()

			if hs.CertPEM == "" {
				t.Fatal("handshake CertPEM = empty, want a per-launch certificate")
			}
			if _, err := ClientTLSConfig([]byte(hs.CertPEM)); err != nil {
				t.Fatalf("ClientTLSConfig(handshake cert) error = %v", err)
			}
		})
	}
}

// startPlugin runs a devkit plugin in serve mode in-process and returns its
// handshake payload plus a stop function that requests shutdown.
func startPlugin(t *testing.T, cfg Config, binding string) (HandshakeBody, func()) {
	t.Helper()

	stdoutR, stdoutW := io.Pipe()
	_, stdinW := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		ios := &IO{Out: stdoutW}
		_ = Run(ctx, cfg, []string{SubcommandServe, "--endpoint", "test://upstream", "--bind", binding}, ios)
	}()

	line, err := bufio.NewReader(stdoutR).ReadString('\n')
	if err != nil {
		cancel()
		t.Fatalf("reading handshake error = %v", err)
	}
	var hs Handshake
	if err := json.Unmarshal([]byte(line), &hs); err != nil {
		cancel()
		t.Fatalf("json.Unmarshal(handshake) error = %v", err)
	}
	if !hs.Success || hs.Payload == nil {
		cancel()
		t.Fatalf("plugin handshake unsuccessful: %+v", hs)
	}

	stop := func() {
		cancel()
		_ = stdinW.Close()
		_ = stdoutR.Close()
		<-done
	}
	return *hs.Payload, stop
}

// newTokenClient builds a client transport for the HTTP bindings that trusts the
// per-launch certificate and stamps the per-launch token on every request,
// mirroring what the host CLI does.
func newTokenClient(t *testing.T, hs HandshakeBody) a2aclient.Transport {
	t.Helper()
	httpClient := &http.Client{
		Timeout:   5 * time.Second,
		Transport: &testTokenRT{base: trustingTransport(t, hs.CertPEM), token: hs.Token},
	}
	switch hs.Binding {
	case a2a.TransportProtocolJSONRPC:
		return a2aclient.NewJSONRPCTransport(hs.Address, httpClient)
	case a2a.TransportProtocolHTTPJSON:
		u, err := url.Parse(hs.Address)
		if err != nil {
			t.Fatalf("url.Parse(%q) error = %v", hs.Address, err)
		}
		return a2aclient.NewRESTTransport(u, httpClient)
	default:
		t.Fatalf("newTokenClient: unsupported binding %q", hs.Binding)
		return nil
	}
}

// trustingTransport returns an HTTP transport that pins the plugin's per-launch
// certificate, matching how the host dials the loopback proxy over TLS.
func trustingTransport(t *testing.T, certPEM string) http.RoundTripper {
	t.Helper()
	tlsConfig, err := ClientTLSConfig([]byte(certPEM))
	if err != nil {
		t.Fatalf("ClientTLSConfig() error = %v", err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	return transport
}

type testTokenRT struct {
	base  http.RoundTripper
	token string
}

func (rt *testTokenRT) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header.Set(TokenSvcParam, rt.token)
	return rt.base.RoundTrip(clone)
}

func artifactText(task *a2a.Task) string {
	var sb bytes.Buffer
	for _, art := range task.Artifacts {
		for _, part := range art.Parts {
			sb.WriteString(part.Text())
		}
	}
	return sb.String()
}

// recordingTransport is a fake upstream transport that echoes the message and
// records the service params it observed.
type recordingTransport struct {
	mu       sync.Mutex
	authSeen string
}

func newRecordingTransport(context.Context, string) (a2aclient.Transport, error) {
	return &recordingTransport{}, nil
}

func (r *recordingTransport) lastAuth() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.authSeen
}

func (r *recordingTransport) SendMessage(_ context.Context, params a2aclient.ServiceParams, req *a2a.SendMessageRequest) (a2a.SendMessageResult, error) {
	r.mu.Lock()
	if auth := params.Get("authorization"); len(auth) > 0 {
		r.authSeen = auth[0]
	}
	r.mu.Unlock()
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

func (r *recordingTransport) SendStreamingMessage(context.Context, a2aclient.ServiceParams, *a2a.SendMessageRequest) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {}
}

func (r *recordingTransport) GetTask(context.Context, a2aclient.ServiceParams, *a2a.GetTaskRequest) (*a2a.Task, error) {
	return nil, a2a.ErrTaskNotFound
}

func (r *recordingTransport) ListTasks(context.Context, a2aclient.ServiceParams, *a2a.ListTasksRequest) (*a2a.ListTasksResponse, error) {
	return &a2a.ListTasksResponse{}, nil
}

func (r *recordingTransport) CancelTask(context.Context, a2aclient.ServiceParams, *a2a.CancelTaskRequest) (*a2a.Task, error) {
	return nil, a2a.ErrTaskNotFound
}

func (r *recordingTransport) SubscribeToTask(context.Context, a2aclient.ServiceParams, *a2a.SubscribeToTaskRequest) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {}
}

func (r *recordingTransport) GetTaskPushConfig(context.Context, a2aclient.ServiceParams, *a2a.GetTaskPushConfigRequest) (*a2a.PushConfig, error) {
	return nil, a2a.ErrPushNotificationNotSupported
}

func (r *recordingTransport) ListTaskPushConfigs(context.Context, a2aclient.ServiceParams, *a2a.ListTaskPushConfigRequest) ([]*a2a.PushConfig, error) {
	return nil, a2a.ErrPushNotificationNotSupported
}

func (r *recordingTransport) CreateTaskPushConfig(context.Context, a2aclient.ServiceParams, *a2a.PushConfig) (*a2a.PushConfig, error) {
	return nil, a2a.ErrPushNotificationNotSupported
}

func (r *recordingTransport) DeleteTaskPushConfig(context.Context, a2aclient.ServiceParams, *a2a.DeleteTaskPushConfigRequest) error {
	return a2a.ErrPushNotificationNotSupported
}

func (r *recordingTransport) GetExtendedAgentCard(context.Context, a2aclient.ServiceParams, *a2a.GetExtendedAgentCardRequest) (*a2a.AgentCard, error) {
	return &a2a.AgentCard{Name: "recording"}, nil
}

func (r *recordingTransport) Destroy() error { return nil }
