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
	"os"
	"path/filepath"
	"testing"
)

// tokenFromEnvScript is a plugin that echoes A2A_TEST_TOKEN back in its handshake
// token, letting the test observe the environment the subprocess was launched with.
const tokenFromEnvScript = "#!/bin/sh\n" +
	"if [ \"$1\" = \"serve\" ]; then\n" +
	"  printf '{\"success\":true,\"payload\":{\"address\":\"127.0.0.1:1\",\"binding\":\"JSONRPC\",\"protocol\":\"1.0\",\"token\":\"%s\"}}\\n' \"$A2A_TEST_TOKEN\"\n" +
	"  cat >/dev/null\n" +
	"fi\n"

func TestExecLauncherSetsEnv(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "a2a-transport-envcheck")
	writeExecutable(t, script, tokenFromEnvScript)

	env := append(os.Environ(), "A2A_TEST_TOKEN=secret-xyz")
	sess, err := execLauncher{}.launch(t.Context(), script, "jsonrpc://demo", env)
	if err != nil {
		t.Fatalf("execLauncher.launch() error = %v", err)
	}
	t.Cleanup(func() { _ = sess.close() })

	if got := sess.handshake.Token; got != "secret-xyz" {
		t.Fatalf("plugin observed A2A_TEST_TOKEN = %q, want %q", got, "secret-xyz")
	}
}
