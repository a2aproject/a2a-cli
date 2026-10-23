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
	"os"
	"strings"

	"github.com/google/shlex"
	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/agentdir"
	"github.com/a2aproject/a2a-cli/internal/clierr"
)

// newServerHarnessCmd builds `a2a server <harness>`, which saves the flags of
// the first run to .a2a/config.yaml and starts the agent. "acp" starts any
// ACP agent given with --command.
func newServerHarnessCmd(cfg *globalConfig, sf *serverFlags, name string) *cobra.Command {
	var (
		s       agentdir.Settings
		email   agentdir.EmailSettings
		command string
		detach  bool
		service bool
	)
	short := fmt.Sprintf("Start a %s agent in this directory", name)
	if name == "acp" {
		short = "Start any ACP agent (--command) in this directory"
	}
	cmd := &cobra.Command{
		Use:   name,
		Short: short,
		Example: fmt.Sprintf(`  a2a server %[1]s --name coder --secret OPENROUTER_API_KEY
  a2a server %[1]s --private tailscale --telegram env:CODER_BOT_TOKEN --detach
  a2a server %[1]s --mount ~/src/app --mount ~/docs:/docs:ro`, name),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if command != "" {
				argv, err := shlex.Split(command)
				if err != nil || len(argv) == 0 {
					return clierr.Usage(fmt.Sprintf("invalid --command %q", command))
				}
				s.Command = argv
			}
			if name == "acp" && len(s.Command) == 0 {
				return clierr.Usage("a2a server acp needs --command, such as --command \"gemini --experimental-acp\"")
			}
			s.Harness = name
			if email != (agentdir.EmailSettings{}) {
				s.Email = &email
			}
			d, err := harnessDir(sf.dir)
			if err != nil {
				return err
			}
			if err := saveSettings(d, s); err != nil {
				return err
			}
			return runAgent(cmd.Context(), cfg, d, detach, service)
		},
	}
	f := cmd.Flags()
	f.StringVar(&s.Name, "name", "", "Agent name, also its tailnet machine name (default: the directory name)")
	f.StringVar(&s.Description, "description", "", "Agent description for its card")
	f.StringArrayVar(&s.Secrets, "secret", nil, "Environment variable passed to the harness, such as an API key; its value is stored in .a2a/state/secrets (repeatable)")
	f.StringArrayVar(&s.Mounts, "mount", nil, "Directory to mount: src[:dst][:ro]; the first is the workspace (repeatable, default: the current directory)")
	f.StringArrayVar(&s.Env, "env", nil, "Extra NAME=value for the harness (repeatable)")
	f.StringVar(&s.Private, "private", "", "Private network: tailscale, lan or none")
	f.StringVar(&s.Public, "public", "", "Public URL: tailscale (Funnel), ngrok or none")
	f.StringVar(&s.Telegram, "telegram", "", "Telegram bot token: env:VAR or the token")
	f.StringVar(&s.Discord, "discord", "", "Discord bot token: env:VAR or the token")
	f.StringVar(&email.Address, "email-address", "", "Email channel: the agent's address")
	f.StringVar(&email.IMAP, "email-imap", "", "Email channel: IMAP host:port")
	f.StringVar(&email.SMTP, "email-smtp", "", "Email channel: SMTP host:port")
	f.StringVar(&email.Username, "email-username", "", "Email channel: login (default: the address)")
	f.StringVar(&email.Password, "email-password", "", "Email channel: password, env:VAR or the value")
	f.StringVar(&email.Mailbox, "email-mailbox", "", "Email channel: mailbox to watch (default INBOX)")
	f.StringVar(&email.TrustedAuthServ, "email-trusted-authserv", "", "Email channel: authserv-id of your mail server's Authentication-Results")
	f.StringVar(&s.Sandbox, "sandbox", "", "Where the harness runs: docker (default) or local")
	f.StringVar(&s.Image, "image", "", "Container image of the harness")
	f.StringVar(&command, "command", "", "Command line of the ACP agent, replacing the profile's")
	f.IntVar(&s.Port, "port", 0, "Localhost port (default: a free port, kept across runs)")
	f.StringSliceVar(&s.AllowTools, "allow-tools", nil, "ACP tool kinds approved without asking, such as read,search")
	f.StringSliceVar(&s.DenyTools, "deny-tools", nil, "ACP tool kinds denied without asking")
	f.StringSliceVar(&s.Approvers, "approver", nil, "Principals besides the owner who may approve tool calls, such as pair:anna")
	f.StringVar(&s.TailscaleControlURL, "tailscale-control-url", "", "Headscale URL, instead of Tailscale's control server")
	f.StringVar(&s.TailscaleAuthKey, "tailscale-authkey", "", "Tailscale auth key, env:VAR or the key, to join without the login link")
	f.StringVar(&s.NgrokURL, "ngrok-url", "", "Reserved ngrok domain, such as https://coder.ngrok.app")
	f.StringVar(&s.NgrokAuthToken, "ngrok-authtoken", "", "ngrok authtoken, env:VAR or the token")
	f.BoolVar(&detach, "detach", false, "Run the agent in the background")
	f.BoolVar(&service, "service", false, "Install and start the agent as a launchd or systemd user service")
	return cmd
}

// harnessDir is --dir, or the current directory: a first run creates the
// agent where it is started, not in a parent agent's directory.
func harnessDir(dir string) (agentdir.Dir, error) {
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return agentdir.Dir{}, err
		}
		dir = wd
	}
	if strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return agentdir.Dir{}, err
		}
		dir = home + dir[1:]
	}
	return agentdir.Open(dir)
}
