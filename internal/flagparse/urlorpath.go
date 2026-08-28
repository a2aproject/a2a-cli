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
	"net"
	"os"
	"path/filepath"
	"strings"
)

// URLOrPath is a pflag.Value that accepts a reference in any of three forms — a
// bare host/origin, a full URL, or a local file path — and normalizes it to a
// URL with an explicit scheme.
type URLOrPath struct {
	raw string
}

// Set records the raw flag value.
func (u *URLOrPath) Set(s string) error {
	u.raw = s
	return nil
}

// String returns the value exactly as provided on the command line.
func (u *URLOrPath) String() string { return u.raw }

// Type reports the pflag value type name.
func (u *URLOrPath) Type() string { return "host|url|path" }

// IsSet reports whether a non-empty value was provided.
func (u *URLOrPath) IsSet() bool { return u.raw != "" }

// URL returns the reference normalized to a URL with an explicit scheme.
func (u *URLOrPath) URL() string {
	ref := u.raw
	if ref == "" || strings.Contains(ref, "://") {
		return ref
	}
	if maybeFilePath(ref) {
		path := ref
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
		return "file://" + path
	}
	if isLoopbackHost(ref) {
		return "http://" + ref
	}
	return "https://" + ref
}

func maybeFilePath(ref string) bool {
	for _, prefix := range []string{"/", "./", "../"} {
		if strings.HasPrefix(ref, prefix) {
			return true
		}
	}
	if _, err := os.Stat(ref); err == nil {
		return true
	}
	return false
}

func isLoopbackHost(ref string) bool {
	host := ref
	if h, _, err := net.SplitHostPort(ref); err == nil {
		host = h
	}
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}
