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
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/a2aproject/a2a-cli/internal/clicfg"
	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-cli/internal/flagparse"
	"github.com/a2aproject/a2a-cli/internal/output"
	"github.com/a2aproject/a2a-cli/internal/polling"
)

// contactEnv runs commands against an isolated contact book and card cache.
type contactEnv struct {
	t            *testing.T
	contactsFile string
	cacheDir     string
	spawned      [][]string
}

func newContactEnv(t *testing.T) *contactEnv {
	t.Helper()
	dir := t.TempDir()
	return &contactEnv{t: t, contactsFile: filepath.Join(dir, "contacts.yaml"), cacheDir: filepath.Join(dir, "cache")}
}

func (e *contactEnv) run(stdin string, args ...string) (string, error) {
	e.t.Helper()
	var buf bytes.Buffer
	cfg := &globalConfig{
		Printer:      output.NewPrinter(&buf, output.ModeText),
		svcParams:    &flagparse.ServiceParams{},
		contactsFile: e.contactsFile,
		cacheDir:     e.cacheDir,
	}
	spawn := func(args []string) error {
		e.spawned = append(e.spawned, args)
		return nil
	}
	root := mustNewRoot(e.t, cfg, deps{poller: polling.Stream, cfgLoader: clicfg.LoadEmpty, spawnDetached: spawn})
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

func (e *contactEnv) mustRun(args ...string) string {
	e.t.Helper()
	out, err := e.run("", args...)
	if err != nil {
		e.t.Fatalf("run(%v) error = %v", args, err)
	}
	return out
}

func TestContactAddListGetRm(t *testing.T) {
	t.Parallel()
	agentURL := startTestServer(t)
	env := newContactEnv(t)

	if out := env.mustRun("contact", "list"); !strings.Contains(out, "No contacts") {
		t.Fatalf("contact list = %q, want empty book message", out)
	}
	if out := env.mustRun("contact", "add", "echo", agentURL, "--note", "test agent"); !strings.Contains(out, "Added contact echo: Test Echo") {
		t.Fatalf("contact add = %q, want confirmation", out)
	}
	if out := env.mustRun("contact", "list"); !strings.Contains(out, "echo") || !strings.Contains(out, "Test Echo") || !strings.Contains(out, "test agent") {
		t.Fatalf("contact list = %q, want the added contact with its agent name and note", out)
	}

	var views []contactView
	if err := json.Unmarshal([]byte(env.mustRun("contact", "list", "-o", "json")), &views); err != nil {
		t.Fatalf("contact list -o json: json.Unmarshal() error = %v", err)
	}
	if len(views) != 1 || views[0].Name != "echo" || views[0].Card != agentURL || views[0].Agent != "Test Echo" {
		t.Fatalf("contact list -o json = %+v, want one contact echo for %s", views, agentURL)
	}

	if out := env.mustRun("contact", "get", "echo"); !strings.Contains(out, "Contact:  echo") || !strings.Contains(out, "Test Echo") {
		t.Fatalf("contact get = %q, want contact details and card", out)
	}
	if out := env.mustRun("contact", "rm", "echo"); !strings.Contains(out, "Removed contact echo") {
		t.Fatalf("contact rm = %q, want confirmation", out)
	}
	if out := env.mustRun("contact", "list"); !strings.Contains(out, "No contacts") {
		t.Fatalf("contact list after rm = %q, want empty book message", out)
	}
}

func TestContactUsageErrors(t *testing.T) {
	t.Parallel()
	agentURL := startTestServer(t)

	tests := []struct {
		name    string
		seed    [][]string
		args    []string
		wantErr string
	}{
		{name: "invalid name", args: []string{"contact", "add", "Bad.Name", agentURL}, wantErr: "invalid contact name"},
		{name: "duplicate", seed: [][]string{{"contact", "add", "echo", agentURL}}, args: []string{"contact", "add", "echo", agentURL}, wantErr: "--replace"},
		{name: "get unknown", args: []string{"contact", "get", "nope"}, wantErr: `no contact named "nope"`},
		{name: "rm unknown", args: []string{"contact", "rm", "nope"}, wantErr: `no contact named "nope"`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := newContactEnv(t)
			for _, args := range tc.seed {
				env.mustRun(args...)
			}
			_, err := env.run("", tc.args...)
			var ce *clierr.Error
			if !errors.As(err, &ce) || ce.Code != clierr.CodeUsage || !strings.Contains(ce.Message, tc.wantErr) {
				t.Fatalf("run(%v) error = %v, want usage error containing %q", tc.args, err, tc.wantErr)
			}
		})
	}
}

func TestContactAddUnreachableCardNotSaved(t *testing.T) {
	t.Parallel()
	env := newContactEnv(t)

	if _, err := env.run("", "contact", "add", "ghost", "http://127.0.0.1:1", "--timeout", "2s"); err == nil {
		t.Fatalf("contact add(unreachable) error = nil, want error")
	}
	if out := env.mustRun("contact", "list"); !strings.Contains(out, "No contacts") {
		t.Fatalf("contact list = %q, want the unreachable contact not saved", out)
	}
}

func TestAgentCardContactName(t *testing.T) {
	t.Parallel()
	agentURL := startTestServer(t)
	env := newContactEnv(t)
	env.mustRun("contact", "add", "echo", agentURL)

	if out := env.mustRun("send", "-a", "echo", "hello contact"); !strings.Contains(out, "hello contact") {
		t.Fatalf("send -a echo = %q, want the echoed message", out)
	}
	if out := env.mustRun("card", "get", "echo"); !strings.Contains(out, "Test Echo") {
		t.Fatalf("card get echo = %q, want the contact's card", out)
	}
}

func TestHookCmd(t *testing.T) {
	t.Parallel()
	agentURL := startTestServer(t)
	env := newContactEnv(t)
	env.mustRun("contact", "add", "echo", agentURL)

	out, err := env.run(`{"hook_event_name":"SessionStart","session_id":"s1","source":"startup"}`, "hook")
	if err != nil {
		t.Fatalf("hook(SessionStart) error = %v", err)
	}
	if !strings.Contains(out, `"hookEventName":"SessionStart"`) || !strings.Contains(out, "- echo — Test Echo") {
		t.Fatalf("hook(SessionStart) = %q, want the contact in hook output", out)
	}

	out, err = env.run(`{"hook_event_name":"UserPromptSubmit","session_id":"s1","prompt":"look at https://new.example/x"}`, "hook", "--discovery-allow-host", "new.example")
	if err != nil {
		t.Fatalf("hook(UserPromptSubmit) error = %v", err)
	}
	if out != "" {
		t.Fatalf("hook(UserPromptSubmit) = %q, want no output before the probe completes", out)
	}
	want := [][]string{{"hook", "probe", "--discovery-allow-host", "new.example", "--", "https://new.example"}}
	if diff := cmp.Diff(want, env.spawned); diff != "" {
		t.Fatalf("hook(UserPromptSubmit) spawned wrong probe (-want +got) diff = %s", diff)
	}
}

func TestHookCmdDiscoveryDisabled(t *testing.T) {
	t.Parallel()
	env := newContactEnv(t)

	out, err := env.run(`{"hook_event_name":"UserPromptSubmit","session_id":"s1","prompt":"https://new.example"}`, "hook", "--discovery=false")
	if err != nil {
		t.Fatalf("hook error = %v", err)
	}
	if out != "" || len(env.spawned) != 0 {
		t.Fatalf("hook --discovery=false = %q, spawned %v; want no output and no probe", out, env.spawned)
	}
}

func TestHookCmdInvalidInputIsSilent(t *testing.T) {
	t.Parallel()
	env := newContactEnv(t)

	out, err := env.run("not json", "hook")
	if err != nil || out != "" {
		t.Fatalf("hook(not json) = %q, %v; want no output and no error", out, err)
	}
}
