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

	"github.com/a2aproject/a2a-cli/internal/commandplugin"
)

// pluginEntry is the JSON/text view of a discovered command plugin.
type pluginEntry struct {
	Name        string `json:"name"`
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
	Path        string `json:"path"`
	Error       string `json:"error,omitempty"`
}

func newPluginListCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed command plugins discovered on PATH",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			entries := collectPluginEntries(cmd)
			if cfg.IsJSON() {
				return cfg.PrintJSON(entries)
			}
			return printPluginTable(cfg.Out, entries)
		},
	}
}

func collectPluginEntries(cmd *cobra.Command) []pluginEntry {
	discovered := commandplugin.List(cmd.Context())
	entries := make([]pluginEntry, 0, len(discovered))
	for _, d := range discovered {
		entry := pluginEntry{Name: d.Name, Path: d.Path}
		switch {
		case d.InfoErr != nil:
			entry.Error = d.InfoErr.Error()
		case d.Info != nil:
			entry.Version = d.Info.Version
			entry.Description = d.Info.Description
		}
		entries = append(entries, entry)
	}
	return entries
}

func printPluginTable(out io.Writer, entries []pluginEntry) error {
	if len(entries) == 0 {
		_, err := io.WriteString(out, "No command plugins found on PATH.\nInstall one by placing an \"a2a-<name>\" binary on your PATH.\n")
		return err
	}

	tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	if _, err := io.WriteString(tw, "NAME\tVERSION\tDESCRIPTION\tPATH\n"); err != nil {
		return err
	}
	for _, e := range entries {
		desc := e.Description
		if e.Error != "" {
			desc = "(error: " + e.Error + ")"
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", e.Name, dashIfEmpty(e.Version), dashIfEmpty(desc), e.Path); err != nil {
			return err
		}
	}
	return tw.Flush()
}
