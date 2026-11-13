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

// Command gendocs writes the markdown command reference of the a2a CLI.
//
// Usage:
//
//	go run ./internal/tools/gendocs [dir]
package main

import (
	"fmt"
	"os"

	"github.com/a2aproject/a2a-cli/internal/cli"
)

// DefaultDir is where the reference is written, relative to the repository root.
const DefaultDir = "docs/reference/commands"

func main() {
	dir := DefaultDir
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	if err := cli.WriteReference(dir); err != nil {
		fmt.Fprintf(os.Stderr, "gendocs: %v\n", err)
		os.Exit(1)
	}
}
