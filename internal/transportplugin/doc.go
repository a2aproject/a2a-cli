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

// Package transportplugin discovers and drives external A2A transport plugins.
//
// A transport plugin is a binary named "a2a-transport-<name>" on PATH. The host
// launches it as a subprocess proxy that speaks a standard A2A binding and
// forwards to a custom upstream protocol. See the devkit package
// github.com/a2aproject/a2a-cli/devkit/clitransport for authoring plugins.
package transportplugin
