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
	"fmt"
	"strconv"
	"strings"

	"github.com/a2aproject/a2a-cli/internal/clierr"
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

		value, source, ok := store.LookupFlag(binding.Name, binding.EnvVar)
		if !ok {
			binding.Source = "default"
			binding.Value = flagValueString(f)
			bindings = append(bindings, binding)
			return
		}

		if err := setFlagValue(f, value); err != nil {
			var msg string
			if source.Path != "" {
				msg = fmt.Sprintf("invalid value for --%s from config file %s: %v", f.Name, source.Path, err)
			} else if source.Kind == SourceEnv {
				msg = fmt.Sprintf("invalid value for --%s from environment variable %s: %v", f.Name, binding.EnvVar, err)
			} else {
				msg = fmt.Sprintf("invalid value for --%s from %s: %v", f.Name, source.String(), err)
			}
			errs = append(errs, clierr.Usage(msg))
			return
		}

		f.Changed = true
		binding.Source = source.String()
		binding.Path = source.Path
		binding.Value = formatValue(value)
		bindings = append(bindings, binding)
	})

	if len(errs) == 1 {
		return bindings, errs[0]
	}
	if len(errs) > 1 {
		return bindings, clierr.Usage(errors.Join(errs...).Error())
	}
	return bindings, nil
}

func flagValueString(f *pflag.Flag) string {
	if sv, ok := f.Value.(pflag.SliceValue); ok {
		return strings.Join(sv.GetSlice(), ",")
	}
	return f.Value.String()
}

func setFlagValue(f *pflag.Flag, raw any) error {
	if slice, ok := toStringSlice(raw); ok {
		if sv, ok := f.Value.(pflag.SliceValue); ok {
			return sv.Replace(slice)
		}
		for _, item := range slice {
			if err := f.Value.Set(item); err != nil {
				return err
			}
		}
		return nil
	}

	switch v := raw.(type) {
	case string:
		if sv, ok := f.Value.(pflag.SliceValue); ok {
			parts := strings.Split(v, ",")
			for i := range parts {
				parts[i] = strings.TrimSpace(parts[i])
			}
			return sv.Replace(parts)
		}
		return f.Value.Set(v)
	case bool:
		return f.Value.Set(strconv.FormatBool(v))
	default:
		return f.Value.Set(fmt.Sprint(v))
	}
}

func flagToEnvVar(name string) string {
	return envPrefix + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

func envVarToFlag(envVar string) (string, bool) {
	if !strings.HasPrefix(envVar, envPrefix) {
		return "", false
	}
	trimmed := strings.TrimPrefix(envVar, envPrefix)
	return strings.ToLower(strings.ReplaceAll(trimmed, "_", "-")), true
}
