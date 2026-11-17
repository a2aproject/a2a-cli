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
	"github.com/spf13/cobra"
)

func newCatalogCmd(cfg *globalConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "catalog",
		Aliases: []string{"catalogs"},
		Short:   "Work with AI Catalogs of agents (experimental)",
		Long: "Register AI Catalogs and find the A2A agents they list.\n\n" +
			"Catalogs are kept in the store shared with ai-catalog-cli (~/.ai-catalog, or $AI_CATALOG_CACHE_DIR), " +
			"so a catalog registered with either tool is visible to both. Pass an agent's identifier to " +
			"--agent-card to talk to it.\n\n" +
			"These commands are experimental: the store layout is not part of the AI Catalog specification.",
	}
	cmd.AddCommand(
		newCatalogAddCmd(cfg),
		newCatalogListCmd(cfg),
		newCatalogUpdateCmd(cfg),
		newCatalogRemoveCmd(cfg),
		newCatalogSearchCmd(cfg),
	)
	return cmd
}
