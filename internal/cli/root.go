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

// Package cli implements the a2a command-line interface.
package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/clicfg"
	"github.com/a2aproject/a2a-cli/internal/flagparse"
	"github.com/a2aproject/a2a-cli/internal/output"
	"github.com/a2aproject/a2a-cli/internal/polling"
)

type cfgLoaderFunc func(clicfg.LoadOpts) (*clicfg.Store, error)

type deps struct {
	poller    pollerFunc
	cfgLoader cfgLoaderFunc
}

func (d *deps) setDefaults() {
	if d.poller == nil {
		d.poller = polling.Stream
	}
	if d.cfgLoader == nil {
		d.cfgLoader = clicfg.Load
	}
}

type globalConfig struct {
	output       string
	agentCard    string
	url          string
	transports   []string
	svcParams    *flagparse.ServiceParams
	bearer       string
	a2aVersion   string
	tenant       string
	timeout      time.Duration
	verbose      bool
	insecureGRPC bool
	configPath   string

	bindings []clicfg.FlagBinding

	*output.Printer
}

func (g *globalConfig) logf(format string, args ...any) {
	if g.verbose {
		_, _ = fmt.Fprintf(os.Stderr, "# "+format+"\n", args...)
	}
}

// Execute runs the CLI and returns the exit code.
func Execute() int {
	cfg := &globalConfig{
		Printer:   &output.Printer{Out: os.Stdout},
		svcParams: &flagparse.ServiceParams{},
	}
	root := newRootCmd(cfg, deps{})
	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}

func newRootCmd(cfg *globalConfig, deps deps) *cobra.Command {
	deps.setDefaults()

	cmd := &cobra.Command{
		Use:           "a2a",
		Short:         "CLI for the Agent-to-Agent protocol",
		Version:       buildVersionInfo().Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			store, err := deps.cfgLoader(clicfg.LoadOpts{ConfigPath: cfg.configPath})
			if err != nil {
				return err
			}
			bindings, err := clicfg.Bind(cmd.Flags(), store)
			if err != nil {
				return err
			}
			cfg.bindings = bindings

			switch output.Mode(cfg.output) {
			case output.ModeText, output.ModeJson:
				cfg.Mode = output.Mode(cfg.output)
			default:
				return fmt.Errorf("invalid --output %q (want text or json)", cfg.output)
			}
			return nil
		},
	}

	pf := cmd.PersistentFlags()
	pf.StringVarP(&cfg.output, "output", "o", "text", "Output format: text, json")
	pf.StringVarP(&cfg.agentCard, "agent-card", "a", "", "Agent Card reference: host/origin, full card URL, or local file path")
	pf.StringVarP(&cfg.url, "endpoint", "e", "", "Agent interface URL for a direct connection; skips card resolution and requires a single --transport flag")
	pf.StringArrayVar(&cfg.transports, "transport", nil, "Transport preference: rest, jsonrpc, grpc (repeatable, highest preference first)")
	pf.StringVar(&cfg.a2aVersion, "a2a-version", "", "Controls which a2a-protocol version client will advertise to the server.")
	cfg.svcParams.Attach(pf)
	pf.StringVar(&cfg.tenant, "tenant", "", "Tenant identifier")
	pf.DurationVar(&cfg.timeout, "timeout", 30*time.Second, "Request timeout")
	pf.BoolVarP(&cfg.verbose, "verbose", "v", false, "Verbose output to stderr")
	pf.BoolVar(&cfg.insecureGRPC, "insecure", false, "Use insecure (plaintext) gRPC transport credentials")
	pf.StringVar(&cfg.configPath, "config", "", "Load configuration from an explicit .env file in place of the local .env")

	cmd.AddCommand(
		newCardCmd(cfg),
		newSendCmd(cfg, deps.poller),
		newTaskCmd(cfg),
		newConfigCmd(cfg),
		newServeCmd(cfg),
		newVersionCmd(cfg),
	)

	cmd.SetUsageTemplate(rootUsageTemplate)

	return cmd
}

// rootUsageTemplate is cobra's default usage template with one change: the root
// command (the one with no parent) lists each command's immediate subcommands
// indented beneath it, so the full command surface (e.g. `task get`, `card get`)
// is visible from `a2a --help`. Subcommands render the normal flat list.
const rootUsageTemplate = `Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

Available Commands:{{if .HasParent}}{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{if (and .IsAvailableCommand (ne .Name "completion"))}}{{range .Commands}}{{if .IsAvailableCommand}}
    {{rpad .Name .NamePadding}} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`
