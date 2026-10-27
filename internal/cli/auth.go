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
)

func newAuthCmd(cfg *globalConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Work with the credentials sent to agents",
		Long: `Work with the credentials sent to agents.

A credential helper (--credential-helper, A2ACLI_CREDENTIAL_HELPER, or
credential-helper in the config file) is a program that returns the HTTP
headers to send to an agent, following the Bazel credential helper protocol:
https://github.com/EngFlow/credential-helper-spec. Its value is
"[scope=]command [args...]", where scope is a host such as example.com, or
*.example.com for a domain and its subdomains. The most specific helper for
the agent's host runs as "command args... get", and its headers are sent
unless --auth or --svc-param set the same header.`,
	}
	cmd.AddCommand(newAuthGetCmd(cfg))
	return cmd
}
