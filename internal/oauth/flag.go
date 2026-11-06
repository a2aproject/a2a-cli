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

package oauth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// ClientsFlag is the repeatable --oauth-clients flag. On the command line and in
// environment variables each value is a JSON object or array; config files list
// {authority, client_id, scopes} objects.
type ClientsFlag struct {
	regs []Registration
}

// Registrations returns the configured registrations in order.
func (f *ClientsFlag) Registrations() []Registration {
	return f.regs
}

// String implements [pflag.Value].
func (f *ClientsFlag) String() string {
	entries := make([]string, len(f.regs))
	for i, r := range f.regs {
		entries[i] = r.Authority + "=" + r.ClientID
	}
	return strings.Join(entries, ",")
}

// Set implements [pflag.Value].
func (f *ClientsFlag) Set(value string) error {
	return f.Append(value)
}

// Type implements [pflag.Value].
func (f *ClientsFlag) Type() string {
	return "json"
}

// Append implements [pflag.SliceValue].
func (f *ClientsFlag) Append(value string) error {
	regs, err := ParseRegistrations([]string{value})
	if err != nil {
		return err
	}
	f.regs = append(f.regs, regs...)
	return nil
}

// Replace implements [pflag.SliceValue].
func (f *ClientsFlag) Replace(values []string) error {
	regs, err := ParseRegistrations(values)
	if err != nil {
		return err
	}
	f.regs = regs
	return nil
}

// GetSlice implements [pflag.SliceValue].
func (f *ClientsFlag) GetSlice() []string {
	out := make([]string, len(f.regs))
	for i, r := range f.regs {
		b, _ := json.Marshal(r)
		out[i] = string(b)
	}
	return out
}

// SetConfig implements clicfg.ConfigValue: a JSON string from an environment
// variable or .env, or a list of objects from a YAML or JSON config file.
func (f *ClientsFlag) SetConfig(raw any) error {
	if s, ok := raw.(string); ok {
		return f.Replace([]string{s})
	}
	items, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("want a list of {authority, client_id} objects, got %T", raw)
	}
	regs := make([]Registration, 0, len(items))
	for _, item := range items {
		r, err := parseConfigRegistration(item)
		if err != nil {
			return err
		}
		regs = append(regs, r)
	}
	f.regs = regs
	return nil
}

func parseConfigRegistration(item any) (Registration, error) {
	if _, ok := item.(map[string]any); !ok {
		return Registration{}, fmt.Errorf("oauth-clients entry %v: want {authority, client_id, scopes}", item)
	}
	data, err := json.Marshal(item)
	if err != nil {
		return Registration{}, fmt.Errorf("oauth-clients entry %v: %w", item, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var r Registration
	if err := dec.Decode(&r); err != nil {
		return Registration{}, fmt.Errorf("oauth-clients entry %s: want {authority, client_id, scopes}: %w", data, err)
	}
	return r, r.validate()
}
