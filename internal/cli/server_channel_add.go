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
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/agentdir"
	"github.com/a2aproject/a2a-cli/internal/agentsrv"
	"github.com/a2aproject/a2a-cli/internal/clierr"
)

func newServerChannelAddCmd(cfg *globalConfig, sf *serverFlags) *cobra.Command {
	var (
		token string
		email agentdir.EmailSettings
	)
	cmd := &cobra.Command{
		Use:   "add <telegram|discord|email>",
		Short: "Add a messenger, or replace its settings",
		Example: `  a2a server channel add telegram --token env:CODER_BOT_TOKEN
  a2a server channel add email --address coder@example.com --imap imap.example.com:993 \
    --smtp smtp.example.com:465 --password env:MAIL_PASSWORD --trusted-authserv mx.example.com`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := sf.agentDir()
			if err != nil {
				return err
			}
			var s agentdir.Settings
			switch args[0] {
			case agentsrv.ChannelTelegram:
				s.Telegram = token
			case agentsrv.ChannelDiscord:
				s.Discord = token
			case agentsrv.ChannelEmail:
				s.Email = &email
			default:
				return clierr.Usage("unknown channel " + args[0] + ": want telegram, discord or email")
			}
			if args[0] != agentsrv.ChannelEmail && token == "" {
				return clierr.Usage("--token is required")
			}
			if err := saveSettings(d, s); err != nil {
				return err
			}
			conf, err := d.LoadConfig()
			if err != nil {
				return err
			}
			st, err := agentsrv.NewClient(d).AddChannel(cmd.Context(), agentsrv.ChannelRequestFromConfig(conf, args[0]))
			if errors.Is(err, agentsrv.ErrNotRunning) {
				_, err := fmt.Fprintf(cfg.Out, "✓ saved %s; it starts with the agent (a2a server)\n", args[0])
				return err
			}
			if err != nil {
				return err
			}
			if cfg.IsJSON() {
				return cfg.PrintJSON(st)
			}
			_, err = fmt.Fprintf(cfg.Out, "✓ %s %s\n", st.Name, st.Account)
			return err
		},
	}
	f := cmd.Flags()
	f.StringVar(&token, "token", "", "Bot token: env:VAR or the token")
	f.StringVar(&email.Address, "address", "", "Email: the agent's address")
	f.StringVar(&email.IMAP, "imap", "", "Email: IMAP host:port")
	f.StringVar(&email.SMTP, "smtp", "", "Email: SMTP host:port")
	f.StringVar(&email.Username, "username", "", "Email: login (default: the address)")
	f.StringVar(&email.Password, "password", "", "Email: password, env:VAR or the value")
	f.StringVar(&email.Mailbox, "mailbox", "", "Email: mailbox to watch (default INBOX)")
	f.StringVar(&email.TrustedAuthServ, "trusted-authserv", "", "Email: authserv-id of your mail server's Authentication-Results")
	return cmd
}
