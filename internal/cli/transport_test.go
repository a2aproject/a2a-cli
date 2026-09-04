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
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a2aproject/a2a-cli/internal/testutil"
	"github.com/a2aproject/a2a-go/v2/a2a"
)

func TestTransportListNoPlugins(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	out := mustRunCMD(t, "transport", "list")
	if !strings.Contains(out, "No transport plugins found") {
		t.Fatalf("transport list with empty PATH = %q, want a 'no plugins' message", out)
	}
}

func TestTransportPluginIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping plugin subprocess integration test in -short mode")
	}

	binDir := t.TempDir()
	buildEchoPlugin(t, binDir)
	t.Setenv("PATH", binDir)

	t.Run("transport list shows the plugin", func(t *testing.T) {
		out := mustRunCMD(t, "transport", "list", "-o", "json")
		var entries []transportEntry
		if err := json.Unmarshal([]byte(out), &entries); err != nil {
			t.Fatalf("json.Unmarshal(transport list) error = %v", err)
		}
		if len(entries) != 1 || entries[0].Name != "echo" {
			t.Fatalf("transport list = %+v, want a single 'echo' entry", entries)
		}
		if entries[0].Version != "1.0.0" {
			t.Fatalf("transport list echo.Version = %q, want %q", entries[0].Version, "1.0.0")
		}
	})

	t.Run("send proxies through the plugin", func(t *testing.T) {
		out := mustRunCMD(t, "send", "--transport", "echo", "--endpoint", "echo://demo", "-o", "json", "hello plugin")
		task := mustDecodeTask(t, out)
		if got := testutil.AllArtifactText(task); got != "hello plugin" {
			t.Fatalf("send via echo plugin artifact text = %q, want %q", got, "hello plugin")
		}
	})

	t.Run("streaming proxies through the plugin", func(t *testing.T) {
		out := mustRunCMD(t, "send", "--transport", "echo", "--endpoint", "echo://demo", "--stream", "-o", "json", "stream me")
		dec := json.NewDecoder(strings.NewReader(out))
		events := 0
		for dec.More() {
			var sr a2a.StreamResponse
			if err := dec.Decode(&sr); err != nil {
				t.Fatalf("json.Decode(event %d) error = %v", events, err)
			}
			events++
		}
		if events <= 1 {
			t.Fatalf("send --stream via echo plugin produced %d events, want > 1", events)
		}
	})

	t.Run("unknown transport reports available plugins", func(t *testing.T) {
		_, err := runCMD(t, "send", "--transport", "missing", "--endpoint", "x://y", "hi")
		if err == nil {
			t.Fatal("send --transport missing error = nil, want error")
		}
		if !strings.Contains(err.Error(), "a2a-transport-missing") || !strings.Contains(err.Error(), "echo") {
			t.Fatalf("send --transport missing error = %v, want it to mention the expected binary and available plugins", err)
		}
	})
}

// buildEchoPlugin compiles the example echo plugin into dir as
// "a2a-transport-echo" so it is discoverable on PATH.
func buildEchoPlugin(t *testing.T, dir string) {
	t.Helper()
	moduleRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolving module root error = %v", err)
	}
	out := filepath.Join(dir, "a2a-transport-echo")
	cmd := exec.Command("go", "build", "-o", out, "./examples/a2a-transport-echo")
	cmd.Dir = moduleRoot
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building echo plugin error = %v\n%s", err, combined)
	}
}
