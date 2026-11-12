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
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/aicatalog"
	"github.com/a2aproject/a2a-cli/internal/output"
)

// catalogAgentView is the JSON view of an A2A agent listed by a catalog.
type catalogAgentView struct {
	Identifier  string   `json:"identifier"`
	DisplayName string   `json:"displayName,omitempty"`
	Version     string   `json:"version,omitempty"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	URL         string   `json:"url,omitempty"`
	Catalog     string   `json:"catalog"`
}

func newCatalogSearchCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "search [query]",
		Short: "Find A2A agents in AI Catalogs",
		Long: "List the A2A agents (entries of type application/a2a-agent-card+json) whose identifier, " +
			"name, description or tags contain the query, ignoring case. Without a query every agent is listed.\n\n" +
			"Searches the registered catalogs, or only the one given with --catalog, which may also be " +
			"the URL of a catalog that is not registered.",
		Example: "  a2a catalog search finance\n" +
			"  a2a catalog search --catalog corp\n" +
			"  a2a send -a <identifier> \"hello\"",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), cfg.timeout)
			defer cancel()
			entries, _, err := catalogEntries(ctx, cfg, cfg.catalog, false)
			if err != nil {
				return err
			}
			query := ""
			if len(args) == 1 {
				query = args[0]
			}
			views := []catalogAgentView{}
			for _, e := range aicatalog.Search(entries, query) {
				views = append(views, catalogAgentView{
					Identifier:  e.Identifier,
					DisplayName: e.DisplayName,
					Version:     e.Version,
					Description: e.Description,
					Tags:        e.Tags,
					URL:         e.URL,
					Catalog:     e.Catalog,
				})
			}
			if cfg.IsJSON() {
				return cfg.PrintJSON(views)
			}
			return printCatalogAgents(cfg.Printer, views, query)
		},
	}
}

func printCatalogAgents(p *output.Printer, views []catalogAgentView, query string) error {
	if len(views) == 0 {
		msg := "No A2A agents found.\n"
		if query != "" {
			msg = fmt.Sprintf("No A2A agents match %q.\n", query)
		}
		_, err := io.WriteString(p.Out, msg)
		return err
	}
	rows := make([][]string, 0, len(views))
	for _, v := range views {
		rows = append(rows, []string{v.Identifier, dashIfEmpty(v.DisplayName), dashIfEmpty(v.Version), v.Catalog, dashIfEmpty(truncate(v.Description, 60))})
	}
	return p.PrintTable([]string{"IDENTIFIER", "NAME", "VERSION", "CATALOG", "DESCRIPTION"}, rows)
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
