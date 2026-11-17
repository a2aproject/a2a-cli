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
	"io"

	"github.com/spf13/cobra"

	"github.com/a2aproject/a2a-cli/internal/clicfg"
	"github.com/a2aproject/a2a-cli/internal/clidoc"
	"github.com/a2aproject/a2a-cli/internal/flagparse"
	"github.com/a2aproject/a2a-cli/internal/output"
)

// WriteReference writes the markdown command reference to dir.
func WriteReference(dir string) error {
	root, err := referenceRoot()
	if err != nil {
		return err
	}
	return clidoc.Write(dir, root, referenceOptions)
}

// ReferencePages renders the markdown command reference, keyed by file name.
func ReferencePages() (map[string]string, error) {
	root, err := referenceRoot()
	if err != nil {
		return nil, err
	}
	return clidoc.Pages(root, referenceOptions), nil
}

// referenceRoot builds the command tree without reading configuration, so
// that local settings such as plugins-enabled don't change the reference.
func referenceRoot() (*cobra.Command, error) {
	cfg := &globalConfig{
		Printer:   output.NewPrinter(io.Discard, output.ModeText),
		svcParams: &flagparse.ServiceParams{},
	}
	root, err := newRootCmd(cfg, deps{cfgLoader: clicfg.LoadEmpty})
	if err != nil {
		return nil, err
	}
	root.InitDefaultCompletionCmd()
	return root, nil
}

// referenceOptions leaves out server, which is due to be removed.
var referenceOptions = clidoc.Options{
	Skip: func(c *cobra.Command) bool { return c.Name() == "server" },
}
