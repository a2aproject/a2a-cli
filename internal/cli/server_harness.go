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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/pflag"

	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-cli/internal/harness"
)

// harnessFlags are the flags shared by the agent harness modes.
type harnessFlags struct {
	serverFlags
	workspace   string
	stateDir    string
	bin         string
	approve     string
	allowTools  []string
	denyTools   []string
	harnessArgs []string
}

func (h *harnessFlags) register(f *pflag.FlagSet, defaultBin string) {
	h.serverFlags.register(f)
	f.StringVar(&h.workspace, "workspace", ".", "Working directory of the harness")
	f.StringVar(&h.stateDir, "state-dir", "", "Where the context-to-session map is kept (default ~/.local/state/a2a-cli/<workspace hash>)")
	f.StringVar(&h.bin, "bin", defaultBin, "Harness executable")
	f.StringVar(&h.approve, "approve", "ask", "Decision for permission requests not matched by --allow-tools/--deny-tools: ask (the A2A caller), allow, deny")
	f.StringSliceVar(&h.allowTools, "allow-tools", nil, "Tool globs approved without asking")
	f.StringSliceVar(&h.denyTools, "deny-tools", nil, "Tool globs denied without asking")
	f.StringArrayVar(&h.harnessArgs, "harness-arg", nil, "Extra argument passed to the harness (repeatable)")
}

// harnessSetup is what every harness executor needs.
type harnessSetup struct {
	sessions  harness.SessionStore
	questions harness.Questions
	log       io.Writer
}

// setup resolves the workspace and state directory and builds the session
// store and the approval rules.
func (h *harnessFlags) setup(cfg *globalConfig) (harnessSetup, error) {
	rules := harness.Rules{Allow: h.allowTools, Deny: h.denyTools}
	switch h.approve {
	case "ask":
	case "allow":
		rules.Allow = append(rules.Allow, "*")
	case "deny":
		rules.Deny = append(rules.Deny, "*")
	default:
		return harnessSetup{}, clierr.Usage(fmt.Sprintf("--approve must be ask, allow or deny, got %q", h.approve))
	}

	ws, err := filepath.Abs(h.workspace)
	if err != nil {
		return harnessSetup{}, err
	}
	h.workspace = ws
	if err := os.MkdirAll(ws, 0o755); err != nil {
		return harnessSetup{}, err
	}
	if h.stateDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return harnessSetup{}, err
		}
		sum := sha256.Sum256([]byte(ws))
		h.stateDir = filepath.Join(home, ".local", "state", "a2a-cli", hex.EncodeToString(sum[:6]))
	}
	sessions, err := harness.NewFileSessions(filepath.Join(h.stateDir, "sessions.json"))
	if err != nil {
		return harnessSetup{}, err
	}

	harnessLog := io.Discard
	if cfg.verbose {
		harnessLog = cfg.stderr()
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}
	return harnessSetup{sessions: sessions, questions: rules, log: harnessLog}, nil
}

// cardDefaults fills the card name and description unless set by flags.
func (h *harnessFlags) cardDefaults(name, desc string) {
	if h.name == "" {
		h.name = name
	}
	if h.desc == "" {
		h.desc = desc
	}
}
