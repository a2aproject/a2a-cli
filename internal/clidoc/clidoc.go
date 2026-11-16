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

// Package clidoc renders the markdown command reference of a cobra command
// tree: one page for the root command, and one page per top-level command
// holding its subcommands.
package clidoc

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Options configures how flags are grouped.
type Options struct {
	// Section returns the help section of a flag, or "" for none.
	Section func(*pflag.Flag) string
	// Sections lists the sections in the order they are printed. Flags in
	// no section, or in a section not listed, come last under "Flags".
	Sections []string
	// Skip, if set, leaves a command and its subcommands out of the reference.
	Skip func(*cobra.Command) bool
}

// Pages renders the reference of root, keyed by file name.
func Pages(root *cobra.Command, opts Options) map[string]string {
	pages := map[string]string{root.Name() + ".md": rootPage(root, opts)}
	for _, c := range visible(root, opts) {
		var sb strings.Builder
		fmt.Fprintf(&sb, "# %s\n\n", c.CommandPath())
		writeCommand(&sb, c, opts, false)
		for _, sub := range descendants(c, opts) {
			fmt.Fprintf(&sb, "\n## %s\n\n", sub.CommandPath())
			writeCommand(&sb, sub, opts, true)
		}
		pages[fileName(c)] = sb.String()
	}
	return pages
}

// Write writes the pages of root to dir, removing markdown files there that
// are no longer generated.
func Write(dir string, root *cobra.Command, opts Options) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	pages := Pages(root, opts)
	stale, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return err
	}
	for _, path := range stale {
		if _, ok := pages[filepath.Base(path)]; ok {
			continue
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	for name, content := range pages {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func rootPage(root *cobra.Command, opts Options) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s\n\n", root.Name())
	sb.WriteString("<!-- Generated from the command definitions; do not edit. -->\n\n")
	writeText(&sb, longOrShort(root))
	groups := root.Groups()
	byGroup := map[string][]*cobra.Command{}
	for _, c := range visible(root, opts) {
		byGroup[c.GroupID] = append(byGroup[c.GroupID], c)
	}
	writeTable := func(title string, cmds []*cobra.Command) {
		if len(cmds) == 0 {
			return
		}
		fmt.Fprintf(&sb, "\n## %s\n\n| Command | Description |\n|---|---|\n", strings.TrimSuffix(title, ":"))
		for _, c := range cmds {
			fmt.Fprintf(&sb, "| [`%s`](%s) | %s |\n", c.Name(), fileName(c), cell(c.Short))
		}
	}
	for _, g := range groups {
		writeTable(g.Title, byGroup[g.ID])
	}
	ungrouped := "Other commands"
	if len(groups) == 0 {
		ungrouped = "Commands"
	}
	writeTable(ungrouped, byGroup[""])
	if fs := root.PersistentFlags(); fs.HasAvailableFlags() {
		sb.WriteString("\n## Global flags\n\nEvery command takes these.\n\n")
		writeFlagTable(&sb, flagList(fs))
	}
	return sb.String()
}

func writeCommand(sb *strings.Builder, c *cobra.Command, opts Options, sub bool) {
	if !sub {
		sb.WriteString("<!-- Generated from the command definitions; do not edit. -->\n\n")
	}
	writeText(sb, longOrShort(c))
	fmt.Fprintf(sb, "\n```\n%s\n```\n", usageLine(c))
	if len(c.Aliases) > 0 {
		fmt.Fprintf(sb, "\nAliases: `%s`\n", strings.Join(c.Aliases, "`, `"))
	}
	if c.HasExample() {
		fmt.Fprintf(sb, "\n**Examples**\n\n```bash\n%s\n```\n", dedent(c.Example))
	}
	bySection := map[string][]*pflag.Flag{}
	c.LocalNonPersistentFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden || f.Name == "help" {
			return
		}
		section := ""
		if opts.Section != nil {
			section = opts.Section(f)
		}
		bySection[section] = append(bySection[section], f)
	})
	for _, s := range opts.Sections {
		if flags := bySection[s]; len(flags) > 0 {
			fmt.Fprintf(sb, "\n**%s**\n\n", sectionTitle(s))
			writeFlagTable(sb, flags)
			delete(bySection, s)
		}
	}
	var rest []*pflag.Flag
	for _, flags := range bySection {
		rest = append(rest, flags...)
	}
	if len(rest) > 0 {
		slices.SortFunc(rest, func(a, b *pflag.Flag) int { return strings.Compare(a.Name, b.Name) })
		sb.WriteString("\n**Flags**\n\n")
		writeFlagTable(sb, rest)
	}
	if shared := flagList(c.PersistentFlags()); len(shared) > 0 && c.Parent() != nil {
		fmt.Fprintf(sb, "\n**Flags of every `%s` command**\n\n", c.CommandPath())
		writeFlagTable(sb, shared)
	}
	if root := c.Root(); !sub && root != c && root.PersistentFlags().HasAvailableFlags() {
		fmt.Fprintf(sb, "\nThe [global flags](%s.md#global-flags) apply too.\n", root.Name())
	}
}

// sectionTitle names the flags of a section: "Network flags", or
// "Run flags (not saved)" for a section with a parenthetical.
func sectionTitle(section string) string {
	name, note, ok := strings.Cut(section, " (")
	if !ok {
		return section + " flags"
	}
	return name + " flags (" + note
}

func usageLine(c *cobra.Command) string {
	line := c.UseLine()
	if c.HasAvailableSubCommands() && line == c.CommandPath() {
		return line + " <command>"
	}
	return line
}

func writeFlagTable(sb *strings.Builder, flags []*pflag.Flag) {
	sb.WriteString("| Flag | Default | Description |\n|---|---|---|\n")
	for _, f := range flags {
		name := "--" + f.Name
		if f.Shorthand != "" {
			name = "-" + f.Shorthand + ", " + name
		}
		varname, usage := pflag.UnquoteUsage(f)
		if varname != "" {
			name += " " + varname
		}
		fmt.Fprintf(sb, "| `%s` | %s | %s |\n", strings.ReplaceAll(name, "|", `\|`), defaultValue(f), cell(usage))
	}
}

func defaultValue(f *pflag.Flag) string {
	switch f.DefValue {
	case "", "false", "[]", "0", "0s":
		return ""
	}
	return "`" + strings.ReplaceAll(f.DefValue, "|", `\|`) + "`"
}

func flagList(fs *pflag.FlagSet) []*pflag.Flag {
	var flags []*pflag.Flag
	fs.VisitAll(func(f *pflag.Flag) {
		if !f.Hidden && f.Name != "help" {
			flags = append(flags, f)
		}
	})
	return flags
}

func visible(c *cobra.Command, opts Options) []*cobra.Command {
	var cmds []*cobra.Command
	for _, sub := range c.Commands() {
		if opts.Skip != nil && opts.Skip(sub) {
			continue
		}
		if sub.IsAvailableCommand() && sub.Name() != "help" {
			cmds = append(cmds, sub)
		}
	}
	return cmds
}

func descendants(c *cobra.Command, opts Options) []*cobra.Command {
	var cmds []*cobra.Command
	for _, sub := range visible(c, opts) {
		cmds = append(cmds, sub)
		cmds = append(cmds, descendants(sub, opts)...)
	}
	return cmds
}

func fileName(c *cobra.Command) string {
	return c.Name() + ".md"
}

func longOrShort(c *cobra.Command) string {
	if c.Long != "" {
		return c.Long
	}
	return c.Short
}

// writeText writes help text as markdown: indented lines become code
// blocks, and the rest is escaped prose.
func writeText(sb *strings.Builder, text string) {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	inCode := false
	for _, line := range lines {
		indented := strings.HasPrefix(line, "  ")
		switch {
		case indented && !inCode:
			if !strings.HasSuffix(sb.String(), "\n\n") {
				sb.WriteString("\n")
			}
			sb.WriteString("```\n")
			inCode = true
		case !indented && inCode && line != "":
			sb.WriteString("```\n\n")
			inCode = false
		}
		if inCode {
			if line == "" {
				continue
			}
			sb.WriteString(strings.TrimPrefix(line, "  ") + "\n")
			continue
		}
		sb.WriteString(escape(line) + "\n")
	}
	if inCode {
		sb.WriteString("```\n")
	}
}

func dedent(text string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimPrefix(line, "  ")
	}
	return strings.Join(lines, "\n")
}

var proseEscaper = strings.NewReplacer("<", "&lt;", ">", "&gt;", "*", `\*`)

// escape escapes markdown and HTML outside the backtick code spans of s.
func escape(s string) string {
	parts := strings.Split(s, "`")
	for i := 0; i < len(parts); i += 2 {
		parts[i] = proseEscaper.Replace(parts[i])
	}
	return strings.Join(parts, "`")
}

// cell escapes s for a table cell, where a pipe ends the cell even in a
// code span.
func cell(s string) string {
	return strings.ReplaceAll(escape(strings.ReplaceAll(s, "\n", " ")), "|", `\|`)
}
