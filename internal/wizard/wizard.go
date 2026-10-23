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

// Package wizard asks the questions of the interactive first run of
// `a2a server`, for people who start it without flags.
package wizard

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/a2aproject/a2a-cli/internal/agentdir"
)

// Run modes.
const (
	ModeForeground = "foreground"
	ModeDetach     = "detach"
	ModeService    = "service"
)

// Result is what the user chose.
type Result struct {
	Settings agentdir.Settings
	Mode     string
}

// apiKeys are the variables harnesses commonly need, in order of preference.
var apiKeys = map[string][]string{
	"claude":   {"ANTHROPIC_API_KEY", "CLAUDE_CODE_OAUTH_TOKEN"},
	"opencode": {"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY", "OPENROUTER_API_KEY"},
}

type prompter struct {
	r   *bufio.Reader
	out io.Writer
}

func (p *prompter) ask(question, def string) (string, error) {
	prompt := question + ": "
	if def != "" {
		prompt = fmt.Sprintf("%s [%s]: ", question, def)
	}
	if _, err := io.WriteString(p.out, prompt); err != nil {
		return "", err
	}
	line, err := p.r.ReadString('\n')
	if err != nil && !(errors.Is(err, io.EOF) && line != "") {
		if errors.Is(err, io.EOF) {
			return def, nil
		}
		return "", err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return def, nil
	}
	return line, nil
}

func (p *prompter) choose(question string, options []string, def string) (string, error) {
	for {
		v, err := p.ask(fmt.Sprintf("%s (%s)", question, strings.Join(options, ", ")), def)
		if err != nil {
			return "", err
		}
		if slices.Contains(options, v) {
			return v, nil
		}
		if _, err := fmt.Fprintf(p.out, "  choose one of: %s\n", strings.Join(options, ", ")); err != nil {
			return "", err
		}
	}
}

// Run asks for the settings of a new agent.
func Run(in io.Reader, out io.Writer, defaultName string, harnesses []string, getenv func(string) string) (Result, error) {
	if len(harnesses) == 0 {
		return Result{}, errors.New("no harnesses to choose from")
	}
	p := &prompter{r: bufio.NewReader(in), out: out}
	if _, err := fmt.Fprintln(out, "No agent here yet. A few questions to set one up (Enter keeps the default):"); err != nil {
		return Result{}, err
	}
	var res Result
	s := &res.Settings
	var err error
	def := harnesses[0]
	if slices.Contains(harnesses, "opencode") {
		def = "opencode"
	}
	if s.Harness, err = p.choose("Harness", harnesses, def); err != nil {
		return res, err
	}
	if s.Name, err = p.ask("Agent name", defaultName); err != nil {
		return res, err
	}

	var defKey string
	for _, k := range apiKeys[s.Harness] {
		if getenv(k) != "" {
			defKey = k
			break
		}
	}
	for {
		keys, err := p.ask("Environment variables with API keys to pass to the agent (comma-separated)", defKey)
		if err != nil {
			return res, err
		}
		s.Secrets = nil
		var missing []string
		for k := range strings.SplitSeq(keys, ",") {
			if k = strings.TrimSpace(k); k == "" {
				continue
			}
			if getenv(k) == "" {
				missing = append(missing, k)
			}
			s.Secrets = append(s.Secrets, k)
		}
		if len(missing) == 0 {
			break
		}
		if _, err := fmt.Fprintf(out, "  not set in this shell: %s\n", strings.Join(missing, ", ")); err != nil {
			return res, err
		}
	}

	if s.Private, err = p.choose("Private network for your machines and team", []string{"none", "tailscale", "lan"}, "none"); err != nil {
		return res, err
	}
	if s.Public, err = p.choose("Public URL on the internet", []string{"none", "tailscale", "ngrok"}, "none"); err != nil {
		return res, err
	}
	if s.Public == "ngrok" {
		def := ""
		if getenv("NGROK_AUTHTOKEN") != "" {
			def = "env:NGROK_AUTHTOKEN"
		}
		if s.NgrokAuthToken, err = p.ask("ngrok authtoken (env:VAR or the token)", def); err != nil {
			return res, err
		}
	}
	if s.Telegram, err = p.ask("Telegram bot token from @BotFather (env:VAR, the token, or empty to skip)", ""); err != nil {
		return res, err
	}
	if res.Mode, err = p.choose("Run", []string{ModeForeground, ModeDetach, ModeService}, ModeForeground); err != nil {
		return res, err
	}
	return res, nil
}
