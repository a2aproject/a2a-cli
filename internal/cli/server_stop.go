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
	"strings"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/controlapi"
	"github.com/a2aproject/a2a-cli/internal/launch"
	"github.com/a2aproject/a2a-cli/internal/service"
)

func newServerStopCmd(cfg *globalConfig, sf *serverFlags) *cobra.Command {
	var uninstall bool
	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the agent",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := sf.agentDir()
			if err != nil {
				return err
			}
			conf, err := d.LoadConfig()
			if err != nil {
				return err
			}
			client := controlapi.NewClient(d)
			_, err = client.Status(cmd.Context())
			running := err == nil
			if err != nil && !errors.Is(err, controlapi.ErrNotRunning) {
				return err
			}
			name := launch.ServiceName(d, conf)
			installed := service.Installed(name)
			if uninstall && installed {
				if err := service.Uninstall(name); err != nil {
					return err
				}
			}
			// A detached agent, or one the service manager didn't stop.
			if err := client.Stop(cmd.Context()); err != nil && !errors.Is(err, controlapi.ErrNotRunning) {
				return err
			}
			var sb strings.Builder
			if running {
				fmt.Fprintf(&sb, "✓ stopped %s\n", conf.Name)
			} else {
				fmt.Fprintf(&sb, "%s is not running\n", conf.Name)
			}
			switch {
			case uninstall && installed:
				fmt.Fprintf(&sb, "✓ removed the %s service\n", conf.Name)
			case uninstall:
				sb.WriteString("  no service was installed\n")
			case installed:
				sb.WriteString("  it starts again at login; remove the service with `a2a server stop --uninstall`\n")
			}
			_, err = fmt.Fprint(cfg.Out, sb.String())
			return err
		},
	}
	cmd.Flags().BoolVar(&uninstall, "uninstall", false, "Also remove the launchd or systemd service")
	return cmd
}
