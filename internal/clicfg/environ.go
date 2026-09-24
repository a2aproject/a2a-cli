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
	"sort"
	"strings"
)

// Environ returns base merged with the values loaded from configuration files,
// as a list of "KEY=value" entries suitable for exec.Cmd.Env.
func (s *Store) Environ(base []string) []string {
	inBase := make(map[string]bool, len(base))
	for _, kv := range base {
		if k, _, ok := strings.Cut(kv, "="); ok {
			inBase[k] = true
		}
	}

	overlay := map[string]string{}
	apply := func(f *loadedFile) {
		if f == nil {
			return
		}
		for k, v := range f.values {
			if k == "" || strings.ContainsRune(k, '=') || inBase[k] {
				continue
			}
			overlay[k] = formatValue(v)
		}
	}
	apply(s.global)
	apply(s.local)

	if len(overlay) == 0 {
		return base
	}

	keys := make([]string, 0, len(overlay))
	for k := range overlay {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]string, 0, len(base)+len(overlay))
	out = append(out, base...)
	for _, k := range keys {
		out = append(out, k+"="+overlay[k])
	}
	return out
}
