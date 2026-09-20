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
	"time"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/discover"
)

// newDiscoverCmd builds the `a2a discover` command: an agent-harness hook helper
// that reads a lifecycle event on stdin, probes any origins it mentions for a
// well-known Agent Card, and prints a context block naming the A2A agents found.
// It is hidden because it is invoked by hooks, not by people.
func newDiscoverCmd(cfg *globalConfig) *cobra.Command {
	var (
		allowPrivate bool
		allow        []string
		probeTimeout time.Duration
		noCache      bool
		urls         []string
		session      string
	)

	cmd := &cobra.Command{
		Use:    "discover",
		Short:  "Discover A2A agents from URLs in an agent harness event (hook helper)",
		Hidden: true,
		Long: `Reads an agent-harness hook event (JSON) on stdin, extracts the origins it
mentions, probes each for a /.well-known/agent-card.json Agent Card, and prints a
context block describing the A2A agents found so a harness can inject it.

Designed to back a Claude Code UserPromptSubmit or PostToolUse hook. It stays
silent (no output, exit 0) when nothing is found, so it never disrupts a turn.

By default it refuses to probe hosts that resolve to private, loopback, or
link-local addresses (an SSRF guard); pass --allow-private or --allow <host> to
probe them (needed for local demos).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			input, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return err
			}

			var cache *discover.Cache
			if !noCache {
				c := discover.NewCache()
				cache = &c
			}

			out, err := discover.Run(cmd.Context(), discover.RunConfig{
				Input:        input,
				ExtraOrigins: urls,
				Guard:        discover.Guard{AllowPrivate: allowPrivate, Allowlist: allow},
				Timeout:      probeTimeout,
				Cache:        cache,
				Logf:         cfg.logf,
				SessionID:    session,
			})
			if err != nil {
				// A hook must not break the turn: log and stay silent.
				cfg.logf("discover: %v", err)
				return nil
			}
			if out != "" {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), out); err != nil {
					return err
				}
			}
			return nil
		},
	}

	f := cmd.Flags()
	f.BoolVar(&allowPrivate, "allow-private", false, "Probe hosts that resolve to private/loopback/link-local addresses")
	f.StringArrayVar(&allow, "allow", nil, "Hostname always allowed to be probed (repeatable)")
	f.DurationVar(&probeTimeout, "probe-timeout", 4*time.Second, "Per-origin card probe timeout")
	f.BoolVar(&noCache, "no-cache", false, "Disable probe caching and per-session dedupe")
	f.StringArrayVar(&urls, "url", nil, "Probe this origin in addition to any found on stdin (repeatable)")
	f.StringVar(&session, "session", "", "Override the session id used for per-session dedupe")

	return cmd
}
