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
	"errors"
	"strings"

	"github.com/spf13/pflag"
)

const envPrefix = "A2ACLI_"

// FlagBinding records how a single flag was resolved.
type FlagBinding struct {
	Name      string
	EnvVar    string
	Value     string
	Source    string
	Path      string
	Sensitive bool
}

// Bind fills every eligible flag that was not set on the command line from
// the configuration store. It records and returns how each flag was resolved.
func Bind(flags *pflag.FlagSet, store *Store) ([]FlagBinding, error) {
	var bindings []FlagBinding
	var errs []error

	flags.VisitAll(func(f *pflag.Flag) {
		switch f.Name { // flags that must come from an explicit command-line flag
		case "stream", "help", "version", "config":
			return
		}

		sensitive := false
		switch f.Name {
		case "auth", "bearer", "api-key":
			sensitive = true
		}

		binding := FlagBinding{
			Name:      f.Name,
			EnvVar:    flagToEnvVar(f.Name),
			Sensitive: sensitive,
		}

		if f.Changed {
			binding.Source = "flag"
			binding.Value = flagValueString(f)
			bindings = append(bindings, binding)
			return
		}

		value, source, ok := store.Lookup(binding.EnvVar)
		if !ok {
			binding.Source = "default"
			binding.Value = flagValueString(f)
			bindings = append(bindings, binding)
			return
		}

		if err := setFlagValue(f, value); err != nil {
			errs = append(errs, err)
			return
		}

		f.Changed = true
		binding.Source = source.String()
		binding.Path = source.Path
		binding.Value = value
		bindings = append(bindings, binding)
	})

	return bindings, errors.Join(errs...)
}

func flagValueString(f *pflag.Flag) string {
	if sv, ok := f.Value.(pflag.SliceValue); ok {
		return strings.Join(sv.GetSlice(), ",")
	}
	return f.Value.String()
}

func setFlagValue(f *pflag.Flag, value string) error {
	if sv, ok := f.Value.(pflag.SliceValue); ok {
		parts := strings.Split(value, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return sv.Replace(parts)
	}
	return f.Value.Set(value)
}

func flagToEnvVar(name string) string {
	return envPrefix + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}
