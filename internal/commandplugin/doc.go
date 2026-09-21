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

// Package commandplugin discovers and executes external a2a command plugins.
//
// A command plugin is a binary named "a2a-<name>" on PATH. The CLI discovers
// these binaries at startup and registers them as dynamic top-level commands.
// Invoking "a2a <name> [args...]" execs the binary with all remaining
// arguments forwarded verbatim and the full environment inherited.
//
// Plugins in the "a2a-transport-" namespace are excluded; those are handled
// by the transportplugin package.
//
// See the devkit/cliplugin package for the public SDK for authoring plugins.
package commandplugin
