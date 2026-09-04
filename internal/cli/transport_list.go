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

	"github.com/a2aproject/a2a-cli/internal/transportplugin"
)

// transportEntry is the JSON/text view of a discovered transport plugin.
type transportEntry struct {
	Name        string `json:"name"`
	Version     string `json:"version,omitempty"`
	Protocol    string `json:"protocol,omitempty"`
	Description string `json:"description,omitempty"`
	Path        string `json:"path"`
	Error       string `json:"error,omitempty"`
}

func newTransportListCmd(cfg *globalConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List installed transport plugins discovered on PATH",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			entries := collectTransportEntries(cmd)
			if cfg.IsJSON() {
				return cfg.PrintJSON(entries)
			}
			return printTransportTable(cfg.Out, entries)
		},
	}
	return cmd
}

func collectTransportEntries(cmd *cobra.Command) []transportEntry {
	discovered := transportplugin.List(cmd.Context())
	entries := make([]transportEntry, 0, len(discovered))
	for _, d := range discovered {
		entry := transportEntry{Name: d.Name, Path: d.Path}
		switch {
		case d.InfoErr != nil:
			entry.Error = d.InfoErr.Error()
		case d.Info != nil:
			entry.Version = d.Info.Version
			entry.Protocol = string(d.Info.Protocol)
			entry.Description = d.Info.Description
		}
		entries = append(entries, entry)
	}
	return entries
}

func printTransportTable(out io.Writer, entries []transportEntry) error {
	if len(entries) == 0 {
		_, err := io.WriteString(out, "No transport plugins found on PATH.\nInstall one by placing an \"a2a-transport-<name>\" binary on your PATH.\n")
		return err
	}

	tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	if _, err := io.WriteString(tw, "NAME\tVERSION\tPROTOCOL\tDESCRIPTION\tPATH\n"); err != nil {
		return err
	}
	for _, e := range entries {
		desc := e.Description
		if e.Error != "" {
			desc = "(error: " + e.Error + ")"
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", e.Name, dashIfEmpty(e.Version), dashIfEmpty(e.Protocol), dashIfEmpty(desc), e.Path); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
