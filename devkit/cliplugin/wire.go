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

package cliplugin

// SubcommandInfo is the optional subcommand a plugin binary may implement
// to report its metadata. When invoked as "a2a-<name> info", the binary
// should print an [Info] JSON document to stdout and exit 0.
const SubcommandInfo = "info"

// Info describes a command plugin. It is printed by the "info" subcommand.
type Info struct {
	// Name is the plugin name (the binary suffix after "a2a-").
	Name string `json:"name"`
	// Version is the plugin's own version string.
	Version string `json:"version,omitempty"`
	// Description is a short human-readable summary shown in --help and plugin list.
	Description string `json:"description,omitempty"`
}
