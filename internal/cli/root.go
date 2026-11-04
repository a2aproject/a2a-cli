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
	"context"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/clicfg"
	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-cli/internal/commandplugin"
	"github.com/a2aproject/a2a-cli/internal/credhelper"
	"github.com/a2aproject/a2a-cli/internal/flagparse"
	"github.com/a2aproject/a2a-cli/internal/oauth"
	"github.com/a2aproject/a2a-cli/internal/output"
	"github.com/a2aproject/a2a-cli/internal/polling"
)

const pluginsEnabledKey = "plugins-enabled"

type cfgLoaderFunc func(clicfg.LoadOpts) (*clicfg.Store, error)

type deps struct {
	poller          pollerFunc
	cfgLoader       cfgLoaderFunc
	isTerminal      func() bool
	notifyInterrupt func(context.Context) (context.Context, context.CancelFunc)

	// authInteractive reports whether an interactive sign-in may start.
	authInteractive func() bool
	// openURL, if set, is called with the URL the user has to visit to sign in.
	openURL func(string)
	// oauthDir is where grants are stored; defaults to ~/.config/a2a-cli/oauth.
	oauthDir string
	// authTimeout bounds an interactive sign-in.
	authTimeout time.Duration
	// authSleep, if set, replaces the wait between device flow polls.
	authSleep func(context.Context, time.Duration) error
	// keyring replaces the OS keychain used by --oauth-store keychain.
	keyring oauth.Keyring
}

func (d *deps) setDefaults() {
	if d.poller == nil {
		d.poller = polling.Stream
	}
	if d.cfgLoader == nil {
		d.cfgLoader = clicfg.Load
	}
	if d.isTerminal == nil {
		d.isTerminal = stdioIsTerminal
	}
	if d.notifyInterrupt == nil {
		d.notifyInterrupt = func(ctx context.Context) (context.Context, context.CancelFunc) {
			return signal.NotifyContext(ctx, os.Interrupt)
		}
	}
	if d.authInteractive == nil {
		d.authInteractive = stderrIsTerminal
	}
	if d.authTimeout == 0 {
		d.authTimeout = 5 * time.Minute
	}
}

type globalConfig struct {
	output       string
	agentCard    flagparse.URLOrPath
	url          string
	transports   []string
	svcParams    *flagparse.ServiceParams
	a2aVersion   string
	tenant       string
	timeout      time.Duration
	verbose      bool
	insecureGRPC bool
	caCert       string
	unixSocket   string
	configPath   string

	credentialHelpers credhelper.Flag
	credentials       *credhelper.Source

	oauthMode            string
	oauthClients         oauth.ClientsFlag
	oauthDefaultClientID string
	oauthStore           string
	oauth                *oauth.Manager

	// insecureWarning prints insecureCredentialWarning at most once.
	insecureWarning sync.Once

	pluginsEnabled bool

	bindings []clicfg.FlagBinding
	store    *clicfg.Store
	errOut   io.Writer

	httpTransport http.RoundTripper
	rootCAs       *x509.CertPool

	handlers      []handleEventFunc
	eventsHandled int
	*output.Printer
}

func (g *globalConfig) stderr() io.Writer {
	if g.errOut != nil {
		return g.errOut
	}
	return os.Stderr
}

func (g *globalConfig) logf(format string, args ...any) {
	if g.verbose {
		_, _ = fmt.Fprintf(g.stderr(), "# "+format+"\n", args...)
	}
}

// Environ implements [transportplugin.EnvironResolver].
func (g *globalConfig) Environ() []string {
	if g.store == nil {
		return nil
	}
	return g.store.Environ(os.Environ())
}

// Execute runs the CLI and returns the exit code.
func Execute() int {
	cfg := &globalConfig{
		Printer:   &output.Printer{Out: os.Stdout},
		svcParams: &flagparse.ServiceParams{},
	}
	root, err := newRootCmd(cfg, deps{})
	if err != nil {
		return cfg.renderError(err)
	}
	if err := root.Execute(); err != nil {
		return cfg.renderError(err)
	}
	return 0
}

func (g *globalConfig) renderError(err error) int {
	ce := clierr.Classify(err)
	// Flag, argument and config errors fail before PersistentPreRunE sets the mode.
	if mode := output.Mode(g.output); mode == output.ModeJson || mode == output.ModeJSONL {
		g.Mode = mode
	}
	if g.IsJSON() {
		if perr := g.PrintJSON(ce); perr != nil {
			_, _ = fmt.Fprintf(g.stderr(), "Error: %v\n", err)
		}
		return ce.Exit
	}
	_, _ = fmt.Fprintf(g.stderr(), "Error: %s\n", ce.Message)
	if ce.Hint != "" {
		_, _ = fmt.Fprintf(g.stderr(), "hint: %s\n", ce.Hint)
	}
	return ce.Exit
}

func newRootCmd(cfg *globalConfig, deps deps) (*cobra.Command, error) {
	deps.setDefaults()

	cmd := &cobra.Command{
		Use:           "a2a",
		Short:         "CLI for the Agent-to-Agent protocol",
		Version:       buildVersionInfo().Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return clierr.Usage(fmt.Sprintf("unknown command %q for %q", args[0], cmd.CommandPath()))
		},
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
			cfg.store = store
			for _, b := range bindings {
				if b.Ignored == "" {
					continue
				}
				if _, err := fmt.Fprintf(cfg.stderr(), "warning: ignoring %s from %s; pass --config %s to use it\n", b.Name, b.Ignored, b.Ignored); err != nil {
					return err
				}
			}
			cfg.initHTTPTransport()
			cfg.initCredentials()
			if err := cfg.initOAuth(deps); err != nil {
				return err
			}

			switch output.Mode(cfg.output) {
			case output.ModeText, output.ModeJson, output.ModeJSONL:
				cfg.Mode = output.Mode(cfg.output)
			default:
				return clierr.Usage(fmt.Sprintf("invalid --output %q (want text or json)", cfg.output))
			}

			return cfg.initEventHandlers(cmd)
		},
	}

	pf := cmd.PersistentFlags()
	pf.StringVarP(&cfg.output, "output", "o", "text", "Output format: text, json (indented), or jsonl (one compact JSON object per line)")
	pf.VarP(&cfg.agentCard, "agent-card", "a", "Agent Card reference: host/origin, full card URL, or local file path")
	pf.StringVarP(&cfg.url, "endpoint", "e", "", "Agent interface URL for a direct connection; skips card resolution and requires a single --transport flag")
	pf.StringArrayVar(&cfg.transports, "transport", nil, "Transport preference: rest, jsonrpc, grpc, or an installed plugin name (repeatable, highest preference first)")
	pf.StringVar(&cfg.a2aVersion, "a2a-version", "", "Controls which a2a-protocol version client will advertise to the server.")
	cfg.svcParams.Attach(pf)
	pf.StringVar(&cfg.tenant, "tenant", "", "Tenant identifier")
	pf.DurationVar(&cfg.timeout, "timeout", 30*time.Second, "Request timeout")
	pf.BoolVarP(&cfg.verbose, "verbose", "v", false, "Verbose output to stderr")
	pf.BoolVar(&cfg.insecureGRPC, "insecure", false, "Use insecure (plaintext) gRPC transport credentials")
	pf.StringVar(&cfg.caCert, "ca-cert", "", "Trust only the CA certificates in this PEM file for TLS connections")
	pf.StringVar(&cfg.unixSocket, "unix-socket", "", "Send every HTTP request and gRPC connection through the Unix socket at this path")
	pf.Var(&cfg.credentialHelpers, "credential-helper", "Credential helper returning the headers for an agent: [scope=]path (repeatable; see a2a auth get --help)")
	pf.StringVar(&cfg.oauthMode, "oauth", oauthAuto, "OAuth sign-in for agents that require it: auto (sign in when needed), cached (use stored grants only), or off (see a2a auth login --help)")
	pf.Var(&cfg.oauthClients, "oauth-clients", `OAuth client registration as JSON: {"authority":"<issuer>","client_id":"<id or CIMD URL>","scopes":[...]} (repeatable; a JSON array is also accepted)`)
	pf.StringVar(&cfg.oauthDefaultClientID, "oauth-default-client-id", oauth.DefaultClientID, "OAuth client ID used with authorization servers that support Client ID Metadata Documents when no oauth-clients entry matches; empty disables it")
	pf.StringVar(&cfg.oauthStore, "oauth-store", "file", "Where OAuth grants are stored: file (~/.config/a2a-cli/oauth/grants.json) or keychain (tokens in the OS keychain)")
	pf.StringVar(&cfg.configPath, "config", "", "Load configuration from an explicit file (.yaml, .json, or .env) in place of the local .env")
	for _, name := range []string{"unix-socket", "credential-helper", "ca-cert", "oauth-clients", "oauth-default-client-id", "oauth-store"} {
		if err := pf.SetAnnotation(name, clicfg.AnnotationNotFromDiscoveredFile, []string{"true"}); err != nil {
			return nil, err
		}
	}

	cmd.AddCommand(
		newCardCmd(cfg),
		newSendCmd(cfg, deps.poller),
		newChatCmd(cfg, deps),
		newTaskCmd(cfg),
		newConfigCmd(cfg),
		newAuthCmd(cfg, deps),
		newServeCmd(cfg),
		newTransportCmd(cfg),
		newPluginCmd(cfg),
		newSkillCmd(cfg),
		newVersionCmd(cfg),
	)

	preStore, err := deps.cfgLoader(clicfg.LoadOpts{})
	if err != nil {
		return nil, err
	}
	if preStore.LookupBool(pluginsEnabledKey) {
		cfg.pluginsEnabled = true
		addCommandPlugins(cmd)
	}

	cmd.SetUsageTemplate(rootUsageTemplate)
	markUsageErrors(cmd)
	wrapSignIn(cfg, deps, cmd)

	return cmd, nil
}

// addCommandPlugins discovers command plugin binaries on PATH and registers
// each as a dynamic top-level command. Built-in commands always win: a plugin
// whose name collides with an existing command is silently skipped.
//
// Descriptions are populated lazily and in parallel the first time root --help
// is invoked, so startup latency is not affected by the number of plugins.
func addCommandPlugins(root *cobra.Command) {
	// Initialise cobra's auto-generated help and completion commands so they
	// appear in the builtins map and cannot be shadowed by a plugin.
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()

	builtins := map[string]bool{}
	for _, sub := range root.Commands() {
		builtins[sub.Name()] = true
	}

	plugins := commandplugin.Discover()
	pluginPaths := make(map[string]string, len(plugins))
	for _, d := range plugins {
		if builtins[d.Name] {
			continue
		}
		pluginPaths[d.Name] = d.Path
		pluginCmd := &cobra.Command{
			Use:                d.Name,
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				return commandplugin.Exec(d.Path, args)
			},
		}
		root.AddCommand(pluginCmd)
	}

	if len(pluginPaths) > 0 {
		setLazyPluginDescriptions(root, pluginPaths)
	}
}

// setLazyPluginDescriptions installs a help-func wrapper that, the first time
// root-level help is rendered, queries every plugin's "info" subcommand in
// parallel and fills in the Short description on each registered command.
func setLazyPluginDescriptions(root *cobra.Command, pluginPaths map[string]string) {
	defaultHelp := root.HelpFunc()
	var once sync.Once
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if cmd == root {
			once.Do(func() { fillPluginDescriptions(root, pluginPaths) })
		}
		defaultHelp(cmd, args)
	})
}

func fillPluginDescriptions(root *cobra.Command, pluginPaths map[string]string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	subs := root.Commands()
	shorts := make([]string, len(subs))
	var wg sync.WaitGroup
	for i, sub := range subs {
		path, ok := pluginPaths[sub.Name()]
		if !ok || sub.Short != "" {
			continue
		}
		wg.Add(1)
		go func(i int, path string) {
			defer wg.Done()
			if info, err := commandplugin.QueryInfo(ctx, path); err == nil {
				shorts[i] = info.Description
			}
		}(i, path)
	}
	wg.Wait()

	for i, sub := range subs {
		if shorts[i] != "" {
			sub.Short = shorts[i]
		}
	}
}

// markUsageErrors makes cobra's flag- and argument-validation failures surface
// as clierr usage errors.
func markUsageErrors(cmd *cobra.Command) {
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return clierr.Usage(err.Error())
	})
	wrapArgsUsage(cmd)
}

func wrapArgsUsage(cmd *cobra.Command) {
	if validate := cmd.Args; validate != nil {
		cmd.Args = func(c *cobra.Command, args []string) error {
			if err := validate(c, args); err != nil {
				return clierr.Usage(err.Error())
			}
			return nil
		}
	}
	for _, sub := range cmd.Commands() {
		wrapArgsUsage(sub)
	}
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
