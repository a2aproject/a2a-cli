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

	"github.com/a2aproject/a2a-cli/internal/fileparts"
	"github.com/a2aproject/a2a-cli/internal/flagparse"
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/spf13/cobra"
)

type handleEventFunc func(a2a.Event) error

func (g *globalConfig) initEventHandlers(cmd *cobra.Command) error {
	filePartsDir, err := flagparse.LookupFilePartsOutDir(cmd.Flags())
	if err != nil {
		return err
	}
	if filePartsDir != "" {
		saver := fileparts.NewSaver(filePartsDir)
		g.handlers = append(g.handlers,
			func(e a2a.Event) error {
				saved, err := saver.Save(e)
				for _, f := range saved {
					if f.Written {
						g.logf("saved file part: %s", f.Path)
					}
				}
				if err != nil {
					return fmt.Errorf("failed to save file parts: %w", err)
				}
				return nil
			})
	}

	g.handlers = append(g.handlers,
		func(e a2a.Event) error {
			if err := g.Printer.PrintEvent(e); err != nil {
				return fmt.Errorf("failed to print event: %w", err)
			}
			return nil
		},
	)

	return nil
}

func (g *globalConfig) handleEvent(e a2a.Event) error {
	for _, handler := range g.handlers {
		if err := handler(e); err != nil {
			return err
		}
	}
	return nil
}
