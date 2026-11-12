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

	"github.com/spf13/cobra"
)

func newCatalogAddCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "add <name> <url|path>",
		Short: "Register an AI Catalog",
		Long: "Fetch the AI Catalog at a URL or local path, and the catalogs nested in it, " +
			"and register it under a name. A URL without a path, or a bare host, gets " +
			"the well-known path /.well-known/ai-catalog.json.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), cfg.timeout)
			defer cancel()
			if err := cfg.initTLS(); err != nil {
				return err
			}
			reg, err := cfg.openCatalogs()
			if err != nil {
				return err
			}
			res, err := reg.Add(ctx, cfg.catalogFetcher(), args[0], args[1])
			if err != nil {
				return err
			}
			if err := cfg.warnAll(res.Warnings); err != nil {
				return err
			}
			if cfg.IsJSON() {
				return cfg.PrintJSON(newCatalogView(res.Catalog))
			}
			_, err = fmt.Fprintf(cfg.Out, "Added catalog %q from %s (%d entries)\n", res.Catalog.Name, res.Catalog.Source, res.Catalog.EntryCount)
			return err
		},
	}
}
