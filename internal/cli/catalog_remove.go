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

	"github.com/spf13/cobra"
)

func newCatalogRemoveCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:     "remove <name|source>",
		Aliases: []string{"rm"},
		Short:   "Unregister an AI Catalog",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, err := cfg.openCatalogs()
			if err != nil {
				return err
			}
			removed, err := reg.Remove(cmd.Context(), args[0])
			if err != nil {
				return catalogCmdError("removing", args[0], err)
			}
			if cfg.IsJSON() {
				return cfg.PrintJSON(newCatalogView(removed))
			}
			_, err = fmt.Fprintf(cfg.Out, "Removed catalog %q\n", removed.Name)
			return err
		},
	}
}
