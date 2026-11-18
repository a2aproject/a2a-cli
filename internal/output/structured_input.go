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

package output

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

const structuredInputURI = "https://github.com/a2aproject/experimental-ext-structured-input/tree/main/v1"

// structuredResumeData returns a --data-part reply template that keeps the
// constants of the message's Structured Input schema, or "" when the schema
// has none or can't be read.
func structuredResumeData(message *a2a.Message) string {
	if message == nil {
		return ""
	}
	var schemaPart *a2a.Part
	for _, part := range message.Parts {
		if part == nil {
			continue
		}
		metadata, ok := part.Metadata[structuredInputURI].(map[string]any)
		if !ok || metadata["type"] != "input_schema" {
			continue
		}
		if schemaPart != nil {
			return ""
		}
		schemaPart = part
	}
	if schemaPart == nil {
		return ""
	}
	encoded, err := json.Marshal(schemaPart.Data())
	if err != nil {
		return ""
	}
	var schema struct {
		Type       any                        `json:"type"`
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(encoded, &schema); err != nil {
		return ""
	}
	if schema.Type != nil {
		if kind, ok := schema.Type.(string); !ok || kind != "object" {
			return ""
		}
	}
	values := make(map[string]any)
	for name, property := range schema.Properties {
		var keywords map[string]json.RawMessage
		if err := json.Unmarshal(property, &keywords); err != nil {
			continue
		}
		if value, ok := keywords["const"]; ok {
			values[name] = value
		}
	}
	if len(values) == 0 {
		return ""
	}
	for _, name := range schema.Required {
		if _, constant := values[name]; constant {
			continue
		}
		values[name] = "<" + name + ">"
	}
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(values); err != nil {
		return ""
	}
	return strings.TrimSuffix(out.String(), "\n")
}
