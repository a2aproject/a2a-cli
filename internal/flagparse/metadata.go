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

package flagparse

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/pflag"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

// Metadata collects one or more --metadata JSON object flags.
// Repeated flags merge into a single object with later keys.
type Metadata struct {
	data map[string]any
}

// Attach registers a repeatable JSON-object flag backed by this parser.
func (m *Metadata) Attach(f *pflag.FlagSet, name, usage string) {
	f.Var(m, name, usage)
}

// Set parses a JSON object string and merges it into the accumulated metadata.
func (m *Metadata) Set(s string) error {
	obj := map[string]any{}
	if err := json.Unmarshal([]byte(s), &obj); err != nil {
		return fmt.Errorf("expected a JSON object: %w", err)
	}
	if m.data == nil {
		m.data = map[string]any{}
	}
	for k, v := range obj {
		m.data[k] = v
	}
	return nil
}

func (m *Metadata) Type() string   { return "json" }
func (m *Metadata) String() string { return "" }

// Map returns the accumulated metadata. The result may be nil when none was
// provided; callers must not assume a non-nil map.
func (m *Metadata) Map() map[string]any {
	return m.data
}

// ApplyTo copies the accumulated metadata onto any request that carries
// request-level metadata (SendMessageRequest, CancelTaskRequest, ...).
func (m *Metadata) ApplyTo(c a2a.MetadataCarrier) {
	for k, v := range m.data {
		c.SetMeta(k, v)
	}
}
