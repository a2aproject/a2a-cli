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

package transportplugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/a2aproject/a2a-cli/devkit/clitransport"
)

const binaryPrefix = "a2a-transport-"

// Discovered describes a transport plugin binary found on PATH.
type Discovered struct {
	// Name is the transport name (the binary suffix after the prefix).
	Name string
	// Path is the absolute path to the plugin binary.
	Path string
	// Info holds the plugin's self-reported metadata. It is nil when the plugin
	// could not be queried; InfoErr then explains why.
	Info *clitransport.Info
	// InfoErr records a failure to query the plugin's "info" subcommand.
	InfoErr error
}

// List discovers every transport plugin on PATH and queries each one's info document.
func List(ctx context.Context) []Discovered {
	out := discover()
	for i := range out {
		info, err := QueryInfo(ctx, out[i].Path)
		if err != nil {
			out[i].InfoErr = err
			continue
		}
		out[i].Info = info
	}
	return out
}

// QueryInfo runs the plugin's "info" subcommand and decodes the result.
func QueryInfo(ctx context.Context, binary string) (*clitransport.Info, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary, clitransport.SubcommandInfo)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("running %q %s: %w", binary, clitransport.SubcommandInfo, err)
	}
	var info clitransport.Info
	if err := json.Unmarshal(out, &info); err != nil {
		return nil, fmt.Errorf("parsing info from %q: %w", binary, err)
	}
	return &info, nil
}

func discover() []Discovered {
	seen := map[string]string{}
	var names []string

	pathDirs := filepath.SplitList(os.Getenv("PATH"))
	for _, dir := range pathDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			base := entry.Name()
			if !strings.HasPrefix(base, binaryPrefix) {
				continue
			}
			name := strings.TrimPrefix(base, binaryPrefix)
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			fullPath := filepath.Join(dir, base)
			if !isExecutable(fullPath) {
				continue
			}
			seen[name] = fullPath
			names = append(names, name)
		}
	}

	sort.Strings(names)
	out := make([]Discovered, 0, len(names))
	for _, name := range names {
		out = append(out, Discovered{Name: name, Path: seen[name]})
	}
	return out
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode()&0b001001001 != 0
}

func binaryName(name string) string {
	return binaryPrefix + strings.ToLower(name)
}
