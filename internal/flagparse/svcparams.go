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
	entries  []serviceParam
	auth     string
	defaults []serviceParam
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
	if len(s.entries) == 0 && s.auth == "" && len(s.defaults) == 0 {
		return nil
	}
	params := a2aclient.ServiceParams{}
	for _, e := range s.entries {
		params.Append(e.key, e.value)
	}
	if s.auth != "" {
		params.Append("Authorization", s.auth)
	}
	for _, d := range s.defaults {
		if !s.isSet(d.key) {
			params.Append(d.key, d.value)
		}
	}
	return params
}

// SetDefault adds values for key that are sent unless --auth or --svc-param
// set the same key, such as the headers from a credential helper.
func (s *ServiceParams) SetDefault(key string, values ...string) {
	for _, v := range values {
		s.defaults = append(s.defaults, serviceParam{key: key, value: v})
	}
}

// isSet reports whether --svc-param, or --auth for Authorization, sets key.
func (s *ServiceParams) isSet(key string) bool {
	if s.auth != "" && strings.EqualFold(key, "Authorization") {
		return true
	}
	for _, e := range s.entries {
		if strings.EqualFold(e.key, key) {
			return true
		}
	}
	return false
}

// SendsCredential reports whether Params includes an Authorization entry,
// set explicitly or by default.
func (s *ServiceParams) SendsCredential() bool {
	if s.HasCredential() {
		return true
	}
	for _, d := range s.defaults {
		if strings.EqualFold(d.key, "Authorization") {
			return true
		}
	}
	return false
}

// HasCredential reports whether --auth or an Authorization --svc-param is set.
func (s *ServiceParams) HasCredential() bool {
	return s.isSet("Authorization")
}

type svcParamValue struct{ s *ServiceParams }

func (v *svcParamValue) Set(kv string) error {
	k, val, ok := cutServiceParam(kv)
	if !ok {
		return fmt.Errorf("expected key=value or key:value, got %q", kv)
	}
	if k == "" {
		return fmt.Errorf("empty key in %q", kv)
	}
	v.s.entries = append(v.s.entries, serviceParam{key: k, value: val})
	return nil
}

// cutServiceParam splits a --svc-param argument on whichever of ':' or '=' comes first.
func cutServiceParam(kv string) (key, value string, ok bool) {
	sep := -1
	for i := 0; i < len(kv); i++ {
		if kv[i] == ':' || kv[i] == '=' {
			sep = i
			break
		}
	}
	if sep < 0 {
		return "", "", false
	}
	return kv[:sep], kv[sep+1:], true
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
