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
	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/credstore"
)

func newPairCmd(cfg *globalConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pair",
		Short: "Pair with agents served by `a2a server` and manage the saved tokens",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newPairAddCmd(cfg), newPairListCmd(cfg), newPairRmCmd(cfg))
	return cmd
}

func (g *globalConfig) credentialStore() (*credstore.Store, error) {
	if g.credentials != nil {
		return g.credentials, nil
	}
	return credstore.Default()
}
