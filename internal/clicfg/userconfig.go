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

package clicfg

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DefaultUserConfigPath returns the path to the user-level YAML config file.
func DefaultUserConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".config", "a2a-cli", userConfigFileName), nil
}

// SetUserValue reads the user config at path, sets key to value, and writes it
// back, preserving other keys and creating parent directories as needed.
func SetUserValue(path, key string, value any) error {
	values := map[string]any{}
	if _, err := os.Stat(path); err == nil {
		existing, err := loadYAML(path)
		if err != nil {
			return fmt.Errorf("reading user config %q: %w", path, err)
		}
		if existing != nil {
			values = existing
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("reading user config %q: %w", path, err)
	}
	values[key] = value

	data, err := yaml.Marshal(values)
	if err != nil {
		return fmt.Errorf("encoding user config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing user config %q: %w", path, err)
	}
	return nil
}
