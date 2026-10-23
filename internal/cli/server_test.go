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
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/a2aproject/a2a-cli/internal/clicfg"
	"github.com/a2aproject/a2a-cli/internal/credstore"
	"github.com/a2aproject/a2a-cli/internal/flagparse"
	"github.com/a2aproject/a2a-cli/internal/output"
	"github.com/a2aproject/a2a-cli/internal/polling"
	"github.com/a2aproject/a2a-cli/internal/relayapi"
)

var (
	fakeACPOnce sync.Once
	fakeACPPath string
	fakeACPErr  error
)

// buildFakeACP builds the scripted ACP agent of the agentsrv tests.
func buildFakeACP(t *testing.T) string {
	t.Helper()
	fakeACPOnce.Do(func() {
		dir, err := os.MkdirTemp("", "fakeacp")
		if err != nil {
			fakeACPErr = err
			return
		}
		fakeACPPath = filepath.Join(dir, "fakeacp")
		out, err := exec.Command("go", "build", "-o", fakeACPPath, "../agentsrv/testdata/fakeacp").CombinedOutput()
		if err != nil {
			fakeACPErr = err
			t.Logf("go build: %s", out)
		}
	})
	if fakeACPErr != nil {
		t.Fatalf("building fakeacp: %v", fakeACPErr)
	}
	return fakeACPPath
}

func runCMDWithCreds(t *testing.T, creds *credstore.Store, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	cfg := &globalConfig{
		Printer:     output.NewPrinter(&buf, output.ModeText),
		svcParams:   &flagparse.ServiceParams{},
		credentials: creds,
	}
	root := mustNewRoot(t, cfg, deps{poller: polling.Stream, cfgLoader: clicfg.LoadEmpty})
	root.SetOut(&buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

// TestServerPairAndSend runs serially: building fakeacp loads the machine,
// and parallel tests with millisecond timeouts would time out.
func TestServerPairAndSend(t *testing.T) {
	fake := buildFakeACP(t)
	dir, err := os.MkdirTemp("", "a2a")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("removing %s: %v", dir, err)
		}
	})
	creds := &credstore.Store{Path: filepath.Join(t.TempDir(), "credentials.json")}

	done := make(chan error, 1)
	go func() {
		_, err := runCMDWithCreds(t, creds, "server", "acp", "--dir", dir, "--name", "coder", "--command", fake, "--sandbox", "local")
		done <- err
	}()
	var status struct {
		Local string `json:"local"`
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		out, err := runCMDWithCreds(t, creds, "-o", "json", "server", "status", "--dir", dir)
		if err == nil && json.Unmarshal([]byte(out), &status) == nil && status.Local != "" {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("a2a server acp error = %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("agent did not start: %s %v", out, err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	out, err := runCMDWithCreds(t, creds, "send", "-a", status.Local, "hello")
	if err != nil || !strings.Contains(out, "Hello from fakeacp.") {
		t.Fatalf("a2a send with the owner token of this machine = %q, %v, want the agent's answer", out, err)
	}
	if out, err := runCMDWithCreds(t, creds, "server", "pair", "--dir", dir); err != nil || !strings.Contains(out, "invite") {
		t.Fatalf("a2a server pair = %q, %v, want help listing invite", out, err)
	}

	out, err = runCMDWithCreds(t, creds, "server", "pair", "invite", "me", "--dir", dir, "--role", "owner")
	if err != nil {
		t.Fatalf("a2a server pair invite error = %v", err)
	}
	code := regexp.MustCompile(`code ([A-Z0-9]{4}-[A-Z0-9]{4})`).FindStringSubmatch(out)
	if code == nil {
		t.Fatalf("a2a server pair invite = %q, want a code", out)
	}
	if _, err := runCMDWithCreds(t, creds, "pair", "add", status.Local, code[1]); err != nil {
		t.Fatalf("a2a pair add error = %v", err)
	}
	out, err = runCMDWithCreds(t, creds, "send", "-a", status.Local, "hello")
	if err != nil {
		t.Fatalf("a2a send error = %v", err)
	}
	if !strings.Contains(out, "Hello from fakeacp.") {
		t.Fatalf("a2a send = %q, want the agent's answer", out)
	}
	if _, err := runCMDWithCreds(t, &credstore.Store{Path: filepath.Join(t.TempDir(), "none.json")}, "send", "-a", status.Local, "hello"); err == nil {
		t.Fatalf("a2a send without a token error = nil, want error")
	}

	out, err = runCMDWithCreds(t, creds, "server", "pair", "invite", "grok", "--dir", dir, "--webhook", "https://example.com/hook")
	if err != nil {
		t.Fatalf("a2a server pair invite --webhook error = %v", err)
	}
	code = regexp.MustCompile(`of the agent at (\S+)\.\n.*\n.*Your pairing code is ([A-Z0-9]{4}-[A-Z0-9]{4})\.`).FindStringSubmatch(out)
	if code == nil {
		t.Fatalf("a2a server pair invite --webhook = %q, want the node's instructions", out)
	}
	if out, err := runCMDWithCreds(t, creds, "pair", "add", code[1], code[2]); err != nil || !strings.Contains(out, "node grok") {
		t.Fatalf("a2a pair add with a node code = %q, %v, want the node token", out, err)
	}
	if out, err := runCMDWithCreds(t, creds, "relay", "inbox"); err != nil || !strings.Contains(out, "grok (node grok)") {
		t.Fatalf("a2a relay inbox = %q, %v, want the node", out, err)
	}
	if _, err := runCMDWithCreds(t, creds, "send", "-a", status.Local, "hello"); err != nil {
		t.Fatalf("a2a send after pairing a node error = %v", err)
	}

	if out, err := runCMDWithCreds(t, creds, "server", "pair", "list", "--dir", dir); err != nil || !strings.Contains(out, "user:owner") || !strings.Contains(out, "node:grok") {
		t.Fatalf("a2a server pair list = %q, %v, want me and grok", out, err)
	}
	if out, err := runCMDWithCreds(t, creds, "server", "pair", "rm", "grok", "--dir", dir); err != nil || !strings.Contains(out, "removed node grok") {
		t.Fatalf("a2a server pair rm grok = %q, %v, want the node removed", out, err)
	}
	if _, err := runCMDWithCreds(t, creds, "server", "pair", "rm", "me", "--dir", dir); err != nil {
		t.Fatalf("a2a server pair rm error = %v", err)
	}
	if _, err := runCMDWithCreds(t, creds, "send", "-a", status.Local, "hello"); err == nil {
		t.Fatalf("a2a send after a2a server pair rm error = nil, want error")
	}

	out, err = runCMDWithCreds(t, creds, "server", "stop", "--uninstall", "--dir", dir)
	if err != nil || !strings.Contains(out, "stopped coder") || !strings.Contains(out, "no service was installed") {
		t.Fatalf("a2a server stop --uninstall = %q, %v, want the agent stopped and no service removed", out, err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("a2a server acp error = %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatalf("agent did not stop")
	}
}

func TestNodeTokenAttached(t *testing.T) {
	t.Parallel()
	url, headers := startTestServerRecordingCardHeaders(t)
	nodeToken := relayapi.NewNodeToken(relayapi.APIBase(url), "grok", "secret")
	creds := &credstore.Store{Path: filepath.Join(t.TempDir(), "credentials.json")}
	if err := creds.Put(credstore.Credential{URL: url + "/n/grok", Kind: credstore.KindNode, Token: nodeToken}); err != nil {
		t.Fatalf("credstore.Put() error = %v", err)
	}
	if _, err := runCMDWithCreds(t, creds, "send", "-a", url, "hello"); err != nil {
		t.Fatalf("a2a send error = %v", err)
	}
	if got, want := headers.Get("Authorization"), "Bearer "+nodeToken; got != want {
		t.Fatalf("a2a send Authorization = %q, want %q", got, want)
	}
}

// TestNodeTokenFromEnvAttached can't run in parallel because of t.Setenv.
func TestNodeTokenFromEnvAttached(t *testing.T) {
	url, headers := startTestServerRecordingCardHeaders(t)
	nodeToken := relayapi.NewNodeToken(relayapi.APIBase(url), "grok", "secret")
	t.Setenv(relayapi.EnvNodeToken, nodeToken)
	creds := &credstore.Store{Path: filepath.Join(t.TempDir(), "credentials.json")}
	if _, err := runCMDWithCreds(t, creds, "send", "-a", url, "hello"); err != nil {
		t.Fatalf("a2a send error = %v", err)
	}
	if got, want := headers.Get("Authorization"), "Bearer "+nodeToken; got != want {
		t.Fatalf("a2a send Authorization = %q, want %q", got, want)
	}
}

func TestStoredTokenAttached(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args func(url string) []string
		want string
	}{
		{name: "card get positional", args: func(url string) []string { return []string{"card", "get", url} }, want: "Bearer stored"},
		{name: "card get flag", args: func(url string) []string { return []string{"card", "get", "-a", url} }, want: "Bearer stored"},
		{name: "send", args: func(url string) []string { return []string{"send", "-a", url, "hello"} }, want: "Bearer stored"},
		{name: "explicit auth wins", args: func(url string) []string { return []string{"send", "-a", url, "--auth", "Bearer flag", "hello"} }, want: "Bearer flag"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			url, headers := startTestServerRecordingCardHeaders(t)
			creds := &credstore.Store{Path: filepath.Join(t.TempDir(), "credentials.json")}
			if err := creds.Put(credstore.Credential{URL: url, Token: "stored"}); err != nil {
				t.Fatalf("credstore.Put() error = %v", err)
			}
			args := tc.args(url)
			if _, err := runCMDWithCreds(t, creds, args...); err != nil {
				t.Fatalf("a2a %s error = %v", strings.Join(args, " "), err)
			}
			if got := headers.Get("Authorization"); got != tc.want {
				t.Fatalf("a2a %s Authorization = %q, want %q", strings.Join(args, " "), got, tc.want)
			}
		})
	}
}
