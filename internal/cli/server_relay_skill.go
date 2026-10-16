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
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/relay"
)

func newServerRelaySkillCmd(cfg *globalConfig) *cobra.Command {
	var (
		rf      relayClientFlags
		install string
	)
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Print the a2a-relay agent skill, with links to the relay",
		Long: `Print the a2a-relay SKILL.md, which teaches an agent to pair, handle wakes,
reply, send and grant. The relay also serves it at /skill.md with its URL
filled in.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cc, err := relay.LoadClientConfig()
			if err != nil {
				return err
			}
			text := relay.Skill(rf.opts.RelayURL(cc))
			if install == "" {
				_, err := fmt.Fprint(cfg.Out, text)
				return err
			}
			dir := filepath.Join(install, "a2a-relay")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			path := filepath.Join(dir, "SKILL.md")
			if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cfg.Out, "Wrote %s\n", path)
			return err
		},
	}
	cmd.Flags().StringVar(&rf.opts.Relay, "relay", "", "Relay URL (default: $"+relay.EnvRelayURL+", the node token or the config file)")
	cmd.Flags().StringVar(&install, "install", "", "Write SKILLS_DIR/a2a-relay/SKILL.md instead of printing, e.g. ~/.claude/skills")
	return cmd
}
