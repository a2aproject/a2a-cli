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

// Command a2a-transport-echo is a reference A2A CLI transport plugin.
//
// It demonstrates the plugin contract using the devkit: the custom transport
// does not talk to any real upstream, it simply echoes the caller's message
// back as a completed task. Install it by putting the built binary on PATH and
// run, for example:
//
//	a2a send --transport echo --endpoint echo://demo "hello there"
//	a2a transport list
package main

import (
	"context"

	"github.com/a2aproject/a2a-cli/devkit/clitransport"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
)

func main() {
	clitransport.Main(clitransport.Config{
		Name:        "echo",
		Version:     "1.0.0",
		Description: "Echoes the caller's message back as a completed task",
		NewTransport: func(_ context.Context, endpoint string) (a2aclient.Transport, error) {
			return newEchoTransport(endpoint), nil
		},
	})
}
