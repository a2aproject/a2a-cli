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

// SourceKind identifies which source a resolved value came from.
type SourceKind string

const (
	// SourceEnv is a real environment variable.
	SourceEnv SourceKind = "env"
	// SourceLocalFile is the local .env (or the file named by --config).
	SourceLocalFile = "local-file"
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

// Store holds the configuration resolved from the environment and .env files.
type Store struct {
	lookupEnv func(string) (string, bool)
	local     *loadedFile
	global    *loadedFile
}

// Lookup returns the value for key and its source.
func (s *Store) Lookup(key string) (string, Source, bool) {
	if v, ok := s.lookupEnv(key); ok {
		return v, Source{Kind: SourceEnv}, true
	}
	if s.local != nil {
		if v, ok := s.local.values[key]; ok {
			return v, Source{Kind: SourceLocalFile, Path: s.local.path}, true
		}
	}
	if s.global != nil {
		if v, ok := s.global.values[key]; ok {
			return v, Source{Kind: SourceGlobalFile, Path: s.global.path}, true
		}
	}
	return "", Source{}, false
}

type loadedFile struct {
	path   string
	values map[string]string
}
