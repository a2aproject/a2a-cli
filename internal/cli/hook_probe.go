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
	"context"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/cardcache"
	"github.com/a2aproject/a2a-cli/internal/contacts"
	"github.com/a2aproject/a2a-cli/internal/discover"
)

const probeRunTimeout = 15 * time.Second

func newHookProbeCmd(cfg *globalConfig, f *hookFlags) *cobra.Command {
	return &cobra.Command{
		Use:    "probe <origin>...",
		Hidden: true,
		Short:  "Fetch the agent cards of origins into the card cache (started by a2a hook)",
		Args:   cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cfg.initTLS(); err != nil {
				return err
			}
			dir, err := cfg.cacheRoot()
			if err != nil {
				return err
			}

			var origins []string
			for _, arg := range args {
				if o := contacts.Origin(arg); o != "" {
					origins = append(origins, o)
					continue
				}
				cfg.logf("skipping %q: not an http(s) origin", arg)
			}

			p := discover.Prober{
				Cards:   cardcache.New(dir),
				LockDir: filepath.Join(dir, "probes"),
				Guard:   discover.Guard{AllowHosts: f.allowHosts},
				RootCAs: cfg.rootCAs,
				Logf:    cfg.logf,
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), probeRunTimeout)
			defer cancel()
			p.Probe(ctx, origins)
			return nil
		},
	}
}
