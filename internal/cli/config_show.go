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
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

type flagBindingView struct {
	Name   string `json:"name"`
	EnvVar string `json:"envVar"`
	Value  string `json:"value"`
	Source string `json:"source"`
	Path   string `json:"path,omitempty"`
}

func newConfigShowCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show the effective configuration and where each value resolved from",
		Long: "Show the effective configuration and where each value resolved from.\n\n" +
			"Change settings by exporting the matching A2ACLI_* environment variables or adding them to a .env file.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			views := make([]flagBindingView, 0, len(cfg.bindings))
			for _, r := range cfg.bindings {
				view := flagBindingView{
					Name:   r.Name,
					EnvVar: r.EnvVar,
					Source: r.Source,
					Path:   r.Path,
				}
				if r.Sensitive {
					view.Value = "<redacted>"
				} else if r.Value != "" {
					view.Value = r.Value
				} else {
					view.Value = "(unset)"
				}
				views = append(views, view)
			}

			if cfg.IsJSON() {
				return cfg.PrintJSON(views)
			}

			tw := tabwriter.NewWriter(cfg.Out, 0, 4, 2, ' ', 0)
			if _, err := io.WriteString(tw, "SETTING\tVALUE\tSOURCE\n"); err != nil {
				return err
			}
			for _, v := range views {
				if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\n", v.Name, v.Value, v.Source); err != nil {
					return err
				}
			}
			return tw.Flush()
		},
	}
}
