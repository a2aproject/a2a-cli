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
	"strings"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/skills"
)

func newSkillCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "skill",
		Short: "Print the a2a-cli agent skill",
		Long:  "Print the a2a-cli SKILL.md bundled into this binary: instructions for teaching an agent to drive A2A agents with this CLI.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			content := skills.A2ACLI
			if !strings.HasSuffix(content, "\n") {
				content += "\n"
			}
			_, err := fmt.Fprint(cfg.Out, content)
			return err
		},
	}
}
