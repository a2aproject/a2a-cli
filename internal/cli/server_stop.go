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

	"github.com/a2aproject/a2a-cli/internal/agentsrv"
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
			name := agentsrv.ServiceName(d, conf)
			if uninstall {
				if !service.Installed(name) {
					return fmt.Errorf("%s is not installed as a service", conf.Name)
				}
				if err := service.Uninstall(name); err != nil {
					return err
				}
				_, err := fmt.Fprintf(cfg.Out, "✓ removed the %s service\n", conf.Name)
				return err
			}
			err = agentsrv.NewClient(d).Stop(cmd.Context())
			if errors.Is(err, agentsrv.ErrNotRunning) {
				_, err := fmt.Fprintf(cfg.Out, "%s is not running\n", conf.Name)
				return err
			}
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(cfg.Out, "✓ stopped %s\n", conf.Name); err != nil {
				return err
			}
			if service.Installed(name) {
				_, err = fmt.Fprintln(cfg.Out, "  it starts again at login; remove the service with `a2a server stop --uninstall`")
			}
			return err
		},
	}
	cmd.Flags().BoolVar(&uninstall, "uninstall", false, "Stop and remove the launchd or systemd service")
	return cmd
}
