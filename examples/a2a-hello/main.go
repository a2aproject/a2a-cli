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

// Command a2a-hello is a reference A2A CLI command plugin.
//
// It demonstrates the command plugin contract using the cliplugin devkit:
// it implements the optional "info" subcommand and accepts a name argument to
// greet. Install it by putting the built binary on PATH:
//
//	go build -o a2a-hello .
//	mv a2a-hello ~/bin/
//
// Then try:
//
//	a2a plugin list
//	a2a hello
//	a2a hello World
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/a2aproject/a2a-cli/devkit/cliplugin"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == cliplugin.SubcommandInfo {
		cliplugin.ServeInfo(cliplugin.Info{
			Name:        "hello",
			Version:     "1.0.0",
			Description: "Greets the world — reference command plugin",
		})
		return // unreachable; ServeInfo exits
	}

	name := "World"
	if len(os.Args) > 1 {
		name = strings.Join(os.Args[1:], " ")
	}
	if _, err := fmt.Fprintf(os.Stdout, "Hello, %s! (from a2a-hello plugin)\n", name); err != nil {
		fmt.Fprintf(os.Stderr, "a2a-hello: %v\n", err)
		os.Exit(1)
	}
}
