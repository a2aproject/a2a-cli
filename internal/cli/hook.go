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
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/cardcache"
	"github.com/a2aproject/a2a-cli/internal/clicfg"
	"github.com/a2aproject/a2a-cli/internal/discover"
)

type hookFlags struct {
	discovery  bool
	allowHosts []string
	scanTools  []string
	spawn      func(args []string) error
}

func newHookCmd(cfg *globalConfig, spawn func(args []string) error) (*cobra.Command, error) {
	f := &hookFlags{spawn: spawn}
	cmd := &cobra.Command{
		Use:    "hook",
		Hidden: true,
		Short:  "Agent harness hook: add contacts and discovered agents to the conversation",
		Long: `Reads an agent harness hook event (JSON) on stdin and prints the context to add
to the conversation in the Claude Code hook output format, or nothing.

  SessionStart        the contact book (trusted agents); after a compaction, also
                      the agents discovered earlier in the session. Agents in
                      registered AI Catalogs are not listed.
  UserPromptSubmit    agents discovered at origins of URLs in the prompt
  PostToolBatch       agents discovered at origins of URLs in the calls of
  PostToolUse         --discovery-scan-tools

The hook never makes network requests. Origins without a cached card are probed
by a detached background process, and agents found are reported by a later
event. Each piece of context is added once per session.

Disable discovery with "discovery: false" in the user config or A2ACLI_DISCOVERY=false.
Discovery refuses link-local addresses, such as the IPv4 cloud metadata endpoint,
and redirects from a public address to an internal one, unless the host is allowed
with --discovery-allow-host.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// A failing hook must not disrupt the harness: report only in verbose mode.
			if err := runHook(cmd, cfg, f); err != nil {
				cfg.logf("hook: %v", err)
			}
			return nil
		},
	}

	pf := cmd.PersistentFlags()
	pf.BoolVar(&f.discovery, "discovery", true, "Discover agents at origins of URLs in the conversation")
	pf.StringArrayVar(&f.allowHosts, "discovery-allow-host", nil, "Host that discovery may probe whatever address it resolves to (repeatable)")
	cmd.Flags().StringSliceVar(&f.scanTools, "discovery-scan-tools", discover.DefaultScanTools, "Tools whose calls are scanned for URLs")
	if err := pf.SetAnnotation("discovery-allow-host", clicfg.AnnotationNotFromDiscoveredFile, []string{"true"}); err != nil {
		return nil, err
	}

	cmd.AddCommand(newHookProbeCmd(cfg, f))
	return cmd, nil
}

func runHook(cmd *cobra.Command, cfg *globalConfig, f *hookFlags) error {
	input, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return fmt.Errorf("reading hook event: %w", err)
	}
	dir, err := cfg.cacheRoot()
	if err != nil {
		return err
	}
	cards := cardcache.New(dir)
	var entries []discover.ContactEntry
	if book, err := cfg.loadContacts(); err != nil {
		cfg.logf("hook: %v", err)
	} else {
		for _, c := range book.List() {
			e := discover.ContactEntry{
				Name:        c.Name,
				Note:        c.Note,
				Origin:      c.Origin(),
				DisplayName: c.Entry.DisplayName,
				Description: c.Entry.Description,
			}
			e.Card, _, _ = cfg.cachedContactCard(cards, c)
			entries = append(entries, e)
		}
	}

	h := discover.Hook{
		Contacts:  entries,
		Cards:     cards,
		StateDir:  filepath.Join(dir, "sessions"),
		Discovery: f.discovery,
		ScanTools: f.scanTools,
		Probe: func(origins []string) error {
			return f.spawn(probeArgs(cfg, f, origins))
		},
		Logf: cfg.logf,
	}
	out, err := h.Run(input)
	if err != nil || out == "" {
		return err
	}
	_, err = fmt.Fprintln(cfg.Out, out)
	return err
}

// probeArgs builds the command line of the background probe, forwarding the
// flags that affect it so it behaves as the hook was configured.
func probeArgs(cfg *globalConfig, f *hookFlags, origins []string) []string {
	args := []string{"hook", "probe"}
	for _, h := range f.allowHosts {
		args = append(args, "--discovery-allow-host", h)
	}
	if cfg.caCert != "" {
		args = append(args, "--ca-cert", cfg.caCert)
	}
	if cfg.configPath != "" {
		args = append(args, "--config", cfg.configPath)
	}
	return append(append(args, "--"), origins...)
}
