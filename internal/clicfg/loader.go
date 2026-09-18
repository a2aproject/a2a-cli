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
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

// LoadOpts controls how Load discovers configuration.
type LoadOpts struct {
	// ConfigPath is loaded as the local file instead of the closest .env.
	ConfigPath string
	// WorkingDir is where the walk-up search for a local .env begins. Defaults to cwd.
	WorkingDir string
	// GlobalPath is the global .env location. Defaults to ~/.config/a2a-cli/.env.
	GlobalPath string
	// LookupEnv reads an environment variable. Defaults to os.LookupEnv.
	LookupEnv func(string) (string, bool)
}

// LoadEmpty returns an empty store disregarding the provided options.
func LoadEmpty(LoadOpts) (*Store, error) {
	return &Store{lookupEnv: func(string) (string, bool) { return "", false }}, nil
}

// Load resolves the configuration sources described by opts.
func Load(opts LoadOpts) (*Store, error) {
	lookupEnv := opts.LookupEnv
	if lookupEnv == nil {
		lookupEnv = os.LookupEnv
	}
	store := &Store{lookupEnv: lookupEnv}

	local, err := loadLocal(opts)
	if err != nil {
		return nil, err
	}
	store.local = local

	global, err := loadGlobal(opts)
	if err != nil {
		return nil, err
	}
	store.global = global

	return store, nil
}

const configFileName = ".env"

func loadLocal(opts LoadOpts) (*loadedFile, error) {
	if opts.ConfigPath != "" {
		values, err := loadConfigFile(opts.ConfigPath)
		if err != nil {
			return nil, fmt.Errorf("reading --config %q: %w", opts.ConfigPath, err)
		}
		return &loadedFile{path: opts.ConfigPath, values: values}, nil
	}

	workingDir := opts.WorkingDir
	if workingDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to get the current working dir: %w", err)
		}
		workingDir = wd
	}
	path, ok := findUpwards(workingDir, configFileName)
	if !ok {
		return nil, nil
	}
	values, err := loadDotenvIfExists(path)
	if err != nil {
		return nil, err
	}
	return &loadedFile{path: path, values: toStringAnyMap(values)}, nil
}

func loadGlobal(opts LoadOpts) (*loadedFile, error) {
	path := opts.GlobalPath
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, nil
		}
		path = filepath.Join(home, ".config", "a2a-cli", configFileName)
	}

	values, err := loadDotenvIfExists(path)
	if err != nil {
		return nil, err
	}
	return &loadedFile{path: path, values: toStringAnyMap(values)}, nil
}

func loadConfigFile(path string) (map[string]any, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		return loadJSON(path)
	case ".yaml", ".yml":
		return loadYAML(path)
	case ".env":
		m, err := loadDotenv(path)
		if err != nil {
			return nil, err
		}
		return toStringAnyMap(m), nil
	default:
		return loadUnknown(path)
	}
}

func loadJSON(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, nil
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing JSON config: %w", err)
	}
	return raw, nil
}

func loadYAML(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, nil
	}
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing YAML config: %w", err)
	}
	if raw == nil {
		return map[string]any{}, nil
	}
	return raw, nil
}

func loadUnknown(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, nil
	}
	var jsonMap map[string]any
	if err := json.Unmarshal(data, &jsonMap); err == nil && jsonMap != nil {
		return jsonMap, nil
	}
	var yamlMap map[string]any
	if err := yaml.Unmarshal(data, &yamlMap); err == nil && yamlMap != nil {
		return yamlMap, nil
	}
	dotenvMap, err := godotenv.Parse(bytes.NewReader(data))
	if err == nil {
		return toStringAnyMap(dotenvMap), nil
	}
	return nil, fmt.Errorf("unrecognized config format in %q", path)
}

func toStringAnyMap(m map[string]string) map[string]any {
	if m == nil {
		return nil
	}
	res := make(map[string]any, len(m))
	for k, v := range m {
		res[k] = v
	}
	return res
}

func loadDotenvIfExists(path string) (map[string]string, error) {
	values, err := loadDotenv(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return values, err
}

func loadDotenv(path string) (values map[string]string, err error) {
	f, openErr := os.Open(path)
	if openErr != nil {
		return nil, openErr
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()
	return godotenv.Parse(f)
}

func findUpwards(dir, name string) (string, bool) {
	for {
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
