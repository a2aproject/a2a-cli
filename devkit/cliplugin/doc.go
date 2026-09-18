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

// Package cliplugin provides helpers for authoring a2a command plugins.
//
// A command plugin is an executable binary named "a2a-<name>" placed anywhere
// on PATH. When the a2a CLI starts, it discovers these binaries and registers
// them as top-level commands. Running "a2a <name> [args...]" execs the binary
// with all remaining arguments forwarded verbatim.
//
// Plugins may optionally implement an "info" subcommand that prints a JSON
// [Info] document to stdout. The CLI uses this to show descriptions in
// "a2a plugin list" and "a2a --help". Use [ServeInfo] to implement it:
//
//	func main() {
//	    if len(os.Args) > 1 && os.Args[1] == cliplugin.SubcommandInfo {
//	        cliplugin.ServeInfo(cliplugin.Info{
//	            Name:        "myplugin",
//	            Version:     "0.1.0",
//	            Description: "Does something useful",
//	        })
//	        return
//	    }
//	    // ... plugin logic ...
//	}
//
// See the devkit/clitransport package for transport plugins, which use a
// different, heavier protocol involving a loopback proxy server.
package cliplugin
