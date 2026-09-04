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
	"fmt"
	"strings"

	"github.com/spf13/pflag"

	"github.com/a2aproject/a2a-go/v2/a2aclient"
)

// ServiceParams collects the --svc-param and --auth flags and merges them into a
// single set of horizontally-applicable request parameters. Like Parts, Attach
// registers several flags backed by one accumulator; merging happens as flags
// are parsed, so Params can be read repeatedly without accumulating duplicates.
type ServiceParams struct {
	entries []serviceParam
	auth    string
}

type serviceParam struct {
	key   string
	value string
}

// Attach registers the --svc-param and --auth flags backed by this parser.
func (s *ServiceParams) Attach(f *pflag.FlagSet) {
	f.Var(&svcParamValue{s}, "svc-param", "Service parameter key=value (repeatable)")
	f.Var(&authValue{s}, "auth", "Shorthand for --svc-param Authorization=<creds>")
}

// Params returns the accumulated parameters, including the Authorization entry
// derived from --auth. A fresh map is returned on each call, so callers may add
// to the result and repeated calls never accumulate duplicates. Returns nil when
// nothing was provided.
func (s *ServiceParams) Params() a2aclient.ServiceParams {
	if len(s.entries) == 0 && s.auth == "" {
		return nil
	}
	params := a2aclient.ServiceParams{}
	for _, e := range s.entries {
		params.Append(e.key, e.value)
	}
	if s.auth != "" {
		params.Append("Authorization", s.auth)
	}
	return params
}

// Auth returns the raw --auth value, needed by flows that attach credentials
// outside the request path, such as agent card resolution.
func (s *ServiceParams) Auth() string {
	return s.auth
}

// HasCredential reports whether --auth or an Authorization --svc-param is set.
func (s *ServiceParams) HasCredential() bool {
	if s.auth != "" {
		return true
	}
	for _, e := range s.entries {
		if strings.EqualFold(e.key, "Authorization") {
			return true
		}
	}
	return false
}

type svcParamValue struct{ s *ServiceParams }

func (v *svcParamValue) Set(kv string) error {
	k, val, ok := strings.Cut(kv, "=")
	if !ok {
		return fmt.Errorf("expected key=value, got %q", kv)
	}
	if k == "" {
		return fmt.Errorf("empty key in %q", kv)
	}
	v.s.entries = append(v.s.entries, serviceParam{key: k, value: val})
	return nil
}

func (v *svcParamValue) String() string { return "" }
func (v *svcParamValue) Type() string   { return "key=value" }

type authValue struct{ s *ServiceParams }

func (v *authValue) Set(creds string) error {
	v.s.auth = creds
	return nil
}

func (v *authValue) String() string { return "" }
func (v *authValue) Type() string   { return "string" }
