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

	"github.com/spf13/cobra"
)

func newPairRmCmd(cfg *globalConfig) *cobra.Command {
	return &cobra.Command{
		Use:   "rm <agent-url>",
		Short: "Forget the token for an agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := cfg.credentialStore()
			if err != nil {
				return err
			}
			ok, err := store.Delete(args[0])
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("no saved token for %s", args[0])
			}
			_, err = fmt.Fprintf(cfg.Out, "✓ forgot the token for %s\n", args[0])
			return err
		},
	}
}
