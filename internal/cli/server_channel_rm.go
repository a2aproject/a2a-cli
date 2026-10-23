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
	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/a2aproject/a2a-cli/internal/controlapi"
)

func newServerChannelRmCmd(cfg *globalConfig, sf *serverFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "rm <telegram|discord|email>",
		Short: "Remove a messenger",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := sf.agentDir()
			if err != nil {
				return err
			}
			err = controlapi.NewClient(d).DeleteChannel(cmd.Context(), args[0])
			if errors.Is(err, controlapi.ErrNotRunning) {
				err = removeChannelOffline(d, args[0])
			}
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cfg.Out, "✓ removed %s\n", args[0])
			return err
		},
	}
}

func removeChannelOffline(d agentdir.Dir, kind string) error {
	conf, err := d.LoadConfig()
	if err != nil {
		return err
	}
	if err := conf.SetChannel(kind, "", nil); err != nil {
		return clierr.Usage(err.Error())
	}
	return d.SaveConfig(conf)
}
