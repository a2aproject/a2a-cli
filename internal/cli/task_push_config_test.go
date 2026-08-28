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

package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/a2aproject/a2a-cli/internal/localsrv"
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
	"github.com/a2aproject/a2a-go/v2/a2asrv/push"
)

func TestBuildPushConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      pushConfigCreateFlags
		want    *a2a.PushConfig
		wantErr bool
	}{
		{
			name: "all fields",
			in: pushConfigCreateFlags{
				taskID:          "task-1",
				tenant:          "acme",
				url:             "https://hook.example/cb",
				id:              "cfg-1",
				token:           "tok",
				authScheme:      "Bearer",
				authCredentials: "secret",
			},
			want: &a2a.PushConfig{
				Tenant: "acme", TaskID: "task-1", ID: "cfg-1", Token: "tok",
				URL:  "https://hook.example/cb",
				Auth: &a2a.PushAuthInfo{Scheme: "Bearer", Credentials: "secret"},
			},
		},
		{
			name: "url only",
			in:   pushConfigCreateFlags{taskID: "task-1", url: "https://hook.example/cb"},
			want: &a2a.PushConfig{TaskID: "task-1", URL: "https://hook.example/cb"},
		},
		{
			name:    "fail if url is missing",
			in:      pushConfigCreateFlags{taskID: "task-1"},
			wantErr: true,
		},
		{
			name:    "fail if no credentials scheme",
			in:      pushConfigCreateFlags{taskID: "task-1", url: "https://hook.example/cb", authCredentials: "secret"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := buildPushConfig(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("buildPushConfig(%+v) error = nil, want an error", tt.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("buildPushConfig(%+v) error = %v, want nil", tt.in, err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("buildPushConfig() wrong result (-want +got) diff = %s", diff)
			}
		})
	}
}

func TestPushConfigCreate(t *testing.T) {
	t.Parallel()
	url := startPushTestServer(t)
	taskID := sendTestMessage(t, url, "setup")

	tests := []struct {
		name    string
		args    []string
		want    *a2a.PushConfig
		wantErr bool
	}{
		{
			name: "creates config with all fields",
			args: []string{"-a", url, string(taskID), "-o", "json",
				"--url", "https://hook.example/cb", "--id", "cfg-1", "--token", "tok",
				"--auth-scheme", "Bearer", "--auth-credentials", "secret"},
			want: &a2a.PushConfig{
				TaskID: taskID, ID: "cfg-1", Token: "tok", URL: "https://hook.example/cb",
				Auth: &a2a.PushAuthInfo{Scheme: "Bearer", Credentials: "secret"},
			},
		},
		{
			name:    "missing --url fails",
			args:    []string{"-a", url, string(taskID)},
			wantErr: true,
		},
		{
			name:    "missing task id fails",
			args:    []string{"-a", url, "--url", "https://hook.example/cb"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			baseArgs := []string{"task", "push-config", "create"}
			out, err := runCMD(t, append(baseArgs, tt.args...)...)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("runCMD(%v) error = nil, want an error", tt.args)
				}
				return
			}
			if err != nil {
				t.Fatalf("runCMD(%v) error = %v, want nil", tt.args, err)
			}
			var got a2a.PushConfig
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("json.Unmarshal(push-config create output) error = %v", err)
			}
			if diff := cmp.Diff(*tt.want, got); diff != "" {
				t.Fatalf("a2a push-config create wrong result (-want +got) diff = %s", diff)
			}
		})
	}
}

func TestPushConfigGet(t *testing.T) {
	t.Parallel()
	url := startPushTestServer(t)
	taskID := sendTestMessage(t, url, "setup")
	created := createTestPushConfig(t, url, taskID)

	tests := []struct {
		name    string
		args    []string
		want    *a2a.PushConfig
		wantErr bool
	}{
		{
			name: "gets config by id",
			args: []string{"-a", url, string(taskID), created.ID, "-o", "json"},
			want: &created,
		},
		{
			name:    "unknown config id fails",
			args:    []string{"-a", url, string(taskID), "does-not-exist"},
			wantErr: true,
		},
		{
			name:    "missing config id fails",
			args:    []string{"-a", url, string(taskID)},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			baseArgs := []string{"task", "push-config", "get"}
			out, err := runCMD(t, append(baseArgs, tt.args...)...)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("runCMD(%v) error = nil, want an error", tt.args)
				}
				return
			}
			if err != nil {
				t.Fatalf("runCMD(%v) error = %v, want nil", tt.args, err)
			}
			var got a2a.PushConfig
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("json.Unmarshal(push-config get output) error = %v", err)
			}
			if diff := cmp.Diff(*tt.want, got); diff != "" {
				t.Fatalf("a2a push-config get wrong result (-want +got) diff = %s", diff)
			}
		})
	}
}

func TestPushConfigList(t *testing.T) {
	t.Parallel()
	url := startPushTestServer(t)
	taskID := sendTestMessage(t, url, "setup")
	first := createTestPushConfig(t, url, taskID)
	second := createTestPushConfig(t, url, taskID)

	tests := []struct {
		name    string
		args    []string
		want    []*a2a.PushConfig
		wantErr bool
	}{
		{
			name: "lists all configs for a task",
			args: []string{"-a", url, string(taskID), "-o", "json"},
			want: []*a2a.PushConfig{&first, &second},
		},
		{
			name:    "missing task id fails",
			args:    []string{"-a", url},
			wantErr: true,
		},
	}

	sortByID := cmpopts.SortSlices(func(a, b *a2a.PushConfig) bool { return a.ID < b.ID })
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			baseArgs := []string{"task", "push-config", "list"}
			out, err := runCMD(t, append(baseArgs, tt.args...)...)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("runCMD(%v) error = nil, want an error", tt.args)
				}
				return
			}
			if err != nil {
				t.Fatalf("runCMD(%v) error = %v, want nil", tt.args, err)
			}
			var got []*a2a.PushConfig
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("json.Unmarshal(push-config list output) error = %v", err)
			}
			if diff := cmp.Diff(tt.want, got, sortByID); diff != "" {
				t.Fatalf("a2a push-config list wrong result (-want +got) diff = %s", diff)
			}
		})
	}
}

func TestPushConfigDelete(t *testing.T) {
	t.Parallel()
	url := startPushTestServer(t)
	taskID := sendTestMessage(t, url, "setup")
	created := createTestPushConfig(t, url, taskID)

	type deleteResponse struct {
		Deleted bool   `json:"deleted"`
		TaskID  string `json:"taskId"`
		ID      string `json:"id"`
	}

	tests := []struct {
		name    string
		args    []string
		want    deleteResponse
		wantErr bool
	}{
		{
			name: "deletes config",
			args: []string{"-a", url, string(taskID), created.ID, "-o", "json"},
			want: deleteResponse{Deleted: true, TaskID: string(taskID), ID: created.ID},
		},
		{
			name:    "missing config id fails",
			args:    []string{"-a", url, string(taskID)},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			baseArgs := []string{"task", "push-config", "delete"}
			out, err := runCMD(t, append(baseArgs, tt.args...)...)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("runCMD(%v) error = nil, want an error", tt.args)
				}
				return
			}
			if err != nil {
				t.Fatalf("runCMD(%v) error = %v, want nil", tt.args, err)
			}
			var got deleteResponse
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("json.Unmarshal(push-config delete output) error = %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("a2a push-config delete wrong result (-want +got) diff = %s", diff)
			}
		})
	}
}

func createTestPushConfig(t *testing.T, url string, taskID a2a.TaskID) a2a.PushConfig {
	t.Helper()
	out := mustRunCMD(t, "task", "push-config", "create", "-a", url, string(taskID), "-o", "json",
		"--url", "https://hook.example/cb")
	var pc a2a.PushConfig
	if err := json.Unmarshal([]byte(out), &pc); err != nil {
		t.Fatalf("json.Unmarshal(push-config create output) error = %v", err)
	}
	return pc
}

func startPushTestServer(t *testing.T) string {
	t.Helper()

	capabilities := a2a.AgentCapabilities{Streaming: true, PushNotifications: true}
	handler := a2asrv.NewHandler(
		localsrv.NewEchoExecutor(),
		a2asrv.WithCapabilityChecks(&capabilities),
		a2asrv.WithPushNotifications(push.NewInMemoryStore(), push.NewHTTPPushSender(nil)),
	)

	mux := http.NewServeMux()
	mux.Handle("/", a2asrv.NewRESTHandler(handler))

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(&a2a.AgentCard{
		Name:                "Test Echo",
		Version:             "1.0.0",
		Capabilities:        capabilities,
		SupportedInterfaces: []*a2a.AgentInterface{a2a.NewAgentInterface(server.URL, a2a.TransportProtocolHTTPJSON)},
	}))

	return server.URL
}
