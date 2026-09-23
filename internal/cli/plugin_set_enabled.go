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
	"strconv"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/clicfg"
	"github.com/a2aproject/a2a-cli/internal/clierr"
)

func newPluginSetEnabledCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "set-enabled <true|false>",
		Short: "Enable or disable command plugins in the user config",
		Long: "Enable or disable discovery and loading of command plugins.\n\n" +
			"The setting is written to the user config file (~/.config/a2a-cli/config.yaml) " +
			"as plugins-enabled and applies to every invocation until changed.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			enabled, err := strconv.ParseBool(args[0])
			if err != nil {
				return clierr.Usage(fmt.Sprintf("invalid boolean %q: want true or false", args[0]))
			}

			path, err := clicfg.DefaultUserConfigPath()
			if err != nil {
				return err
			}
			if err := clicfg.SetUserValue(path, pluginsEnabledKey, enabled); err != nil {
				return err
			}

			if cfg.IsJSON() {
				return cfg.PrintJSON(map[string]any{
					"plugins-enabled": enabled,
					"path":            path,
				})
			}
			state := "disabled"
			if enabled {
				state = "enabled"
			}
			_, err = fmt.Fprintf(cfg.Out, "Command plugins %s (%s in %s)\n", state, pluginsEnabledKey, path)
			return err
		},
	}
}
