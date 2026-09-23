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
	"strconv"
	"strings"
)

// SourceKind identifies which source a resolved value came from.
type SourceKind string

const (
	// SourceEnv is a real environment variable.
	SourceEnv SourceKind = "env"
	// SourceLocalFile is the local .env (or the file named by --config).
	SourceLocalFile = "local-file"
	// SourceUserFile is the user-level config.yaml under ~/.config/a2a-cli.
	SourceUserFile = "user-file"
	// SourceGlobalFile is the global .env under ~/.config/a2a-cli.
	SourceGlobalFile = "global-file"
)

// Source describes where a value resolved from.
type Source struct {
	// Kind describes the source.
	Kind SourceKind
	// Path is set for file-backed sources.
	Path string
}

// String implements Stringer.
func (s Source) String() string {
	return string(s.Kind)
}

// Store holds the configuration resolved from the environment and config files.
type Store struct {
	lookupEnv func(string) (string, bool)
	local     *loadedFile
	user      *loadedFile
	global    *loadedFile
}

// Lookup returns the string value for key and its source.
func (s *Store) Lookup(key string) (string, Source, bool) {
	val, src, ok := s.lookupValue(key)
	if !ok {
		return "", Source{}, false
	}
	return formatValue(val), src, true
}

// LookupBool returns true is config has a value matching [strconv.ParseBool] accepted
// truth literals and false otherwise.
func (s *Store) LookupBool(key string) bool {
	val, _, ok := s.lookupValue(key)
	if !ok {
		return false
	}
	enabled, err := strconv.ParseBool(strings.TrimSpace(formatValue(val)))
	return err == nil && enabled
}

func (s *Store) lookupValue(key string) (any, Source, bool) {
	if flagName, ok := envVarToFlag(key); ok {
		return s.LookupFlag(flagName, key)
	}
	return s.LookupFlag(key, flagToEnvVar(key))
}

// LookupFlag returns the configuration value for a flag, checking environment variables
// by envVar and config files by flag name or envVar.
func (s *Store) LookupFlag(name, envVar string) (any, Source, bool) {
	if envVar != "" {
		if v, ok := s.lookupEnv(envVar); ok {
			return v, Source{Kind: SourceEnv}, true
		}
	}
	if s.local != nil {
		if v, ok := s.local.lookup(name, envVar); ok {
			return v, Source{Kind: SourceLocalFile, Path: s.local.path}, true
		}
	}
	if s.user != nil {
		if v, ok := s.user.lookup(name, envVar); ok {
			return v, Source{Kind: SourceUserFile, Path: s.user.path}, true
		}
	}
	if s.global != nil {
		if v, ok := s.global.lookup(name, envVar); ok {
			return v, Source{Kind: SourceGlobalFile, Path: s.global.path}, true
		}
	}
	return nil, Source{}, false
}

type loadedFile struct {
	path   string
	values map[string]any
}

func (f *loadedFile) lookup(name, envVar string) (any, bool) {
	if f == nil || f.values == nil {
		return nil, false
	}
	if v, ok := f.values[name]; ok {
		return v, true
	}
	if envVar != "" {
		if v, ok := f.values[envVar]; ok {
			return v, true
		}
	}
	return nil, false
}

func toStringSlice(raw any) ([]string, bool) {
	switch v := raw.(type) {
	case []string:
		return v, true
	case []any:
		parts := make([]string, len(v))
		for i, item := range v {
			parts[i] = fmt.Sprint(item)
		}
		return parts, true
	default:
		return nil, false
	}
}

func formatValue(val any) string {
	if val == nil {
		return ""
	}
	if s, ok := val.(string); ok {
		return s
	}
	if slice, ok := toStringSlice(val); ok {
		return strings.Join(slice, ",")
	}
	return fmt.Sprint(val)
}
