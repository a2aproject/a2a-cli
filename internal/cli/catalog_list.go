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
	"io"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/aicatalog"
	"github.com/a2aproject/a2a-cli/internal/output"
)

// catalogView is the JSON view of a registered catalog.
type catalogView struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Entries     int    `json:"entries"`
	LastUpdated string `json:"lastUpdated,omitempty"`
}

func newCatalogView(c aicatalog.Catalog) catalogView {
	return catalogView{Name: c.Name, Source: c.Source, Entries: c.EntryCount, LastUpdated: c.LastUpdated}
}

func newCatalogListCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List registered AI Catalogs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, err := cfg.openCatalogs()
			if err != nil {
				return err
			}
			views := []catalogView{}
			for _, c := range reg.Catalogs() {
				views = append(views, newCatalogView(c))
			}
			if cfg.IsJSON() {
				return cfg.PrintJSON(views)
			}
			return printCatalogList(cfg.Printer, views)
		},
	}
}

func printCatalogList(p *output.Printer, views []catalogView) error {
	if len(views) == 0 {
		_, err := io.WriteString(p.Out, "No catalogs registered. Add one with: a2a catalog add <name> <url>\n")
		return err
	}
	rows := make([][]string, 0, len(views))
	for _, v := range views {
		rows = append(rows, []string{v.Name, v.Source, strconv.Itoa(v.Entries), formatTimestamp(v.LastUpdated)})
	}
	return p.PrintTable([]string{"NAME", "SOURCE", "ENTRIES", "LAST UPDATED"}, rows)
}

func formatTimestamp(rfc3339 string) string {
	t, err := time.Parse(time.RFC3339Nano, rfc3339)
	if err != nil {
		return dashIfEmpty(rfc3339)
	}
	return t.Local().Format(time.DateTime)
}
