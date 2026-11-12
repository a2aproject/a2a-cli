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
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/aicatalog"
)

func newCatalogUpdateCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "update [name]",
		Short: "Fetch registered AI Catalogs again",
		Long:  "Fetch a registered AI Catalog from its source again, or every registered catalog when no name is given.",
		Args:  cobra.MaximumNArgs(1),
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
			var names []string
			if len(args) == 1 {
				names = args
			} else {
				for _, c := range reg.Catalogs() {
					names = append(names, c.Name)
				}
			}
			if len(names) == 0 && !cfg.IsJSON() {
				_, err := io.WriteString(cfg.Out, "No catalogs registered. Add one with: a2a catalog add <name> <url>\n")
				return err
			}

			views := []catalogView{}
			for _, name := range names {
				res, err := reg.Update(ctx, cfg.catalogFetcher(), name)
				if err != nil {
					return catalogCmdError("updating", name, err)
				}
				if err := cfg.warnAll(res.Warnings); err != nil {
					return err
				}
				views = append(views, newCatalogView(res.Catalog))
				if cfg.IsJSON() {
					continue
				}
				state := "up to date"
				if res.Changed {
					state = fmt.Sprintf("updated (%d entries)", res.Catalog.EntryCount)
				}
				if _, err := fmt.Fprintf(cfg.Out, "Catalog %q %s\n", res.Catalog.Name, state); err != nil {
					return err
				}
			}
			if cfg.IsJSON() {
				return cfg.PrintJSON(views)
			}
			return nil
		},
	}
}

// catalogCmdError classifies an error from a catalog command.
func catalogCmdError(action, name string, err error) error {
	if _, ok := errors.AsType[*aicatalog.UnknownCatalogError](err); ok {
		return catalogError(err)
	}
	return fmt.Errorf("%s catalog %q: %w", action, name, err)
}
