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

package credhelper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Flag is a repeatable flag of credential helpers. On the command line and
// in environment variables each value is "[scope=]path"; config files list
// [Entry] objects.
type Flag struct {
	helpers []Helper
}

// Helpers returns the configured helpers in order.
func (f *Flag) Helpers() []Helper {
	return f.helpers
}

// String implements [pflag.Value].
func (f *Flag) String() string {
	return strings.Join(f.GetSlice(), ",")
}

// Set implements [pflag.Value].
func (f *Flag) Set(spec string) error {
	return f.Append(spec)
}

// Type implements [pflag.Value].
func (f *Flag) Type() string {
	return "helper"
}

// Append implements [pflag.SliceValue].
func (f *Flag) Append(spec string) error {
	h, err := Parse(spec)
	if err != nil {
		return err
	}
	f.helpers = append(f.helpers, h)
	return nil
}

// Replace implements [pflag.SliceValue].
func (f *Flag) Replace(specs []string) error {
	helpers := make([]Helper, 0, len(specs))
	for _, spec := range specs {
		h, err := Parse(spec)
		if err != nil {
			return err
		}
		helpers = append(helpers, h)
	}
	f.helpers = helpers
	return nil
}

// GetSlice implements [pflag.SliceValue].
func (f *Flag) GetSlice() []string {
	specs := make([]string, len(f.helpers))
	for i, h := range f.helpers {
		specs[i] = h.Spec
	}
	return specs
}

// SetConfig sets the helpers from a configuration value: one "[scope=]path"
// string, or a list of [Entry] objects.
func (f *Flag) SetConfig(raw any) error {
	if s, ok := raw.(string); ok {
		return f.Replace([]string{s})
	}
	items, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("want a list of credential helpers, got %T", raw)
	}
	helpers := make([]Helper, 0, len(items))
	for _, item := range items {
		h, err := parseConfigItem(item)
		if err != nil {
			return err
		}
		helpers = append(helpers, h)
	}
	f.helpers = helpers
	return nil
}

func parseConfigItem(item any) (Helper, error) {
	if _, ok := item.(map[string]any); !ok {
		return Helper{}, fmt.Errorf("credential helper %v: want {scope, command}", item)
	}
	data, err := json.Marshal(item)
	if err != nil {
		return Helper{}, fmt.Errorf("credential helper %v: %w", item, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var e Entry
	if err := dec.Decode(&e); err != nil {
		return Helper{}, fmt.Errorf("credential helper %s: want {scope, command}: %w", data, err)
	}
	return e.Helper()
}
