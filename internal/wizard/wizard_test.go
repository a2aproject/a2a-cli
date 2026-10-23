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

package wizard

import (
	"io"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/a2aproject/a2a-cli/internal/agentdir"
)

func TestRun(t *testing.T) {
	t.Parallel()
	env := map[string]string{"ANTHROPIC_API_KEY": "x", "NGROK_AUTHTOKEN": "y"}
	getenv := func(k string) string { return env[k] }
	tests := []struct {
		name  string
		input string
		want  Result
	}{
		{
			name:  "defaults",
			input: "\n\n\n\n\n\n\n",
			want: Result{Mode: ModeForeground, Settings: agentdir.Settings{
				Harness: "opencode", Name: "app", Secrets: []string{"ANTHROPIC_API_KEY"}, Private: "none", Public: "none",
			}},
		},
		{
			name:  "choices with retries",
			input: "claude\ncoder\nMISSING\nANTHROPIC_API_KEY\nwifi\ntailscale\nngrok\n\nenv:BOT\nservice\n",
			want: Result{Mode: ModeService, Settings: agentdir.Settings{
				Harness: "claude", Name: "coder", Secrets: []string{"ANTHROPIC_API_KEY"}, Private: "tailscale", Public: "ngrok",
				NgrokAuthToken: "env:NGROK_AUTHTOKEN", Telegram: "env:BOT",
			}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := Run(strings.NewReader(tc.input), io.Discard, "app", []string{"claude", "opencode"}, getenv)
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("Run() wrong result (-want +got) diff = %s", diff)
			}
		})
	}
}
