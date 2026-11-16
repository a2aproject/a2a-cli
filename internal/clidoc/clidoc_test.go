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

package clidoc

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func testTree() *cobra.Command {
	root := &cobra.Command{Use: "tool", Short: "A tool"}
	root.PersistentFlags().BoolP("verbose", "v", false, "Verbose output")
	root.AddGroup(&cobra.Group{ID: "run", Title: "Run it:"})
	start := &cobra.Command{
		Use:     "start <name>",
		Short:   "Start it",
		Long:    "Start <name>, or `tool start <other>`.\n\n  tool start a\n  tool start b\n\nThen *wait*.",
		Example: "  tool start a --mode fast",
		GroupID: "run",
		Run:     func(*cobra.Command, []string) {},
	}
	start.Flags().String("mode", "slow", "Mode: `fast|slow`")
	start.Flags().String("net", "", "Network")
	start.Flags().String("hidden", "", "Hidden")
	start.Flags().String("other", "", "Other")
	if err := start.Flags().MarkHidden("hidden"); err != nil {
		panic(err)
	}
	if err := start.Flags().SetAnnotation("net", "section", []string{"Network"}); err != nil {
		panic(err)
	}
	pair := &cobra.Command{Use: "pair", Short: "Pair | join"}
	pair.PersistentFlags().String("dir", "", "Agent directory")
	pair.AddCommand(&cobra.Command{Use: "rm <name>", Short: "Remove a pairing", Run: func(*cobra.Command, []string) {}})
	secret := &cobra.Command{Use: "secret", Short: "Secret", Hidden: true, Run: func(*cobra.Command, []string) {}}
	root.AddCommand(start, pair, secret)
	return root
}

var testOptions = Options{
	Section: func(f *pflag.Flag) string {
		if s := f.Annotations["section"]; len(s) > 0 {
			return s[0]
		}
		return ""
	},
	Sections: []string{"Network"},
}

func TestPages(t *testing.T) {
	t.Parallel()
	pages := Pages(testTree(), testOptions)
	var names []string
	for name := range pages {
		names = append(names, name)
	}
	slices.Sort(names)
	if diff := cmp.Diff([]string{"pair.md", "start.md", "tool.md"}, names); diff != "" {
		t.Fatalf("Pages() wrong files (-want +got) diff = %s", diff)
	}

	tests := []struct {
		page    string
		want    []string
		notWant []string
	}{
		{
			page: "tool.md",
			want: []string{
				"## Run it\n\n| Command | Description |\n|---|---|\n| [`start`](start.md) | Start it |",
				"## Other commands\n\n| Command | Description |\n|---|---|\n| [`pair`](pair.md) | Pair \\| join |",
				"| `-v, --verbose` |  | Verbose output |",
			},
			notWant: []string{"secret"},
		},
		{
			page: "start.md",
			want: []string{
				"# tool start\n",
				"Start &lt;name&gt;, or `tool start <other>`.\n\n```\ntool start a\ntool start b\n```\n\nThen \\*wait\\*.",
				"```\ntool start <name> [flags]\n```",
				"**Examples**\n\n```bash\ntool start a --mode fast\n```",
				"**Network flags**\n\n| Flag | Default | Description |\n|---|---|---|\n| `--net string` |  | Network |",
				"**Flags**\n\n| Flag | Default | Description |\n|---|---|---|\n| `--mode fast\\|slow` | `slow` | Mode: fast\\|slow |\n| `--other string` |  | Other |",
				"The [global flags](tool.md#global-flags) apply too.",
			},
			notWant: []string{"--hidden", "--verbose"},
		},
		{
			page: "pair.md",
			want: []string{
				"```\ntool pair <command>\n```",
				"**Flags of every `tool pair` command**\n\n| Flag | Default | Description |\n|---|---|---|\n| `--dir string` |  | Agent directory |",
				"## tool pair rm\n\nRemove a pairing\n\n```\ntool pair rm <name>\n```"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.page, func(t *testing.T) {
			t.Parallel()
			got := pages[tc.page]
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Fatalf("Pages()[%q] = %s, want it to contain %q", tc.page, got, want)
				}
			}
			for _, notWant := range tc.notWant {
				if strings.Contains(got, notWant) {
					t.Fatalf("Pages()[%q] = %s, want no %q", tc.page, got, notWant)
				}
			}
		})
	}
}

func TestPagesSkip(t *testing.T) {
	t.Parallel()
	opts := testOptions
	opts.Skip = func(c *cobra.Command) bool { return c.Name() == "pair" }
	pages := Pages(testTree(), opts)
	if _, ok := pages["pair.md"]; ok {
		t.Fatalf("Pages() wrote pair.md, want it skipped")
	}
	if got := pages["tool.md"]; strings.Contains(got, "pair") {
		t.Fatalf("Pages()[%q] = %s, want no pair", "tool.md", got)
	}
}

func TestPagesUngroupedRoot(t *testing.T) {
	t.Parallel()
	root := &cobra.Command{Use: "tool", Short: "A tool"}
	root.AddCommand(&cobra.Command{Use: "send", Short: "Send it", Run: func(*cobra.Command, []string) {}})
	got := Pages(root, Options{})["tool.md"]
	want := "## Commands\n\n| Command | Description |\n|---|---|\n| [`send`](send.md) | Send it |"
	if !strings.Contains(got, want) {
		t.Fatalf("Pages()[%q] = %s, want it to contain %q", "tool.md", got, want)
	}
}

func TestWriteRemovesStalePages(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, name := range []string{"gone.md", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := Write(dir, testTree(), testOptions); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if diff := cmp.Diff([]string{"notes.txt", "pair.md", "start.md", "tool.md"}, got); diff != "" {
		t.Fatalf("Write() wrong files (-want +got) diff = %s", diff)
	}
}
