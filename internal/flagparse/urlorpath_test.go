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
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestURLOrPathURL(t *testing.T) {
	t.Parallel()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}

	tests := []struct {
		name string
		ref  string
		want string
	}{
		{name: "http url unchanged", ref: "http://a.example", want: "http://a.example"},
		{name: "https url unchanged", ref: "https://a.example/card.json", want: "https://a.example/card.json"},
		{name: "file url unchanged", ref: "file:///tmp/card.json", want: "file:///tmp/card.json"},
		{name: "absolute path becomes file url", ref: "/tmp/card.json", want: "file:///tmp/card.json"},
		{name: "bare host gets https", ref: "agent.example", want: "https://agent.example"},
		{name: "bare host:port gets https", ref: "agent.example:8443", want: "https://agent.example:8443"},
		{name: "loopback host:port gets http", ref: "127.0.0.1:8091", want: "http://127.0.0.1:8091"},
		{name: "loopback range gets http", ref: "127.5.5.5", want: "http://127.5.5.5"},
		{name: "ipv6 loopback gets http", ref: "[::1]:8080", want: "http://[::1]:8080"},
		{name: "localhost gets http", ref: "localhost:9000", want: "http://localhost:9000"},
		{name: "host starting with 127 is not loopback", ref: "127.example.com", want: "https://127.example.com"},
		{name: "relative path", ref: "./card.json", want: "file://" + filepath.Join(cwd, "card.json")},
		{name: "relative in parent", ref: "../card.json", want: "file://" + filepath.Join(cwd, "..", "card.json")},
		{name: "empty stays empty", ref: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var u URLOrPath
			if err := u.Set(tt.ref); err != nil {
				t.Fatalf("URLOrPath.Set(%q) error = %v", tt.ref, err)
			}
			if got := u.URL(); got != tt.want {
				t.Fatalf("URLOrPath.URL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestURLOrPathValidate(t *testing.T) {
	t.Parallel()

	existing := filepath.Join(t.TempDir(), "card.json")
	if err := os.WriteFile(existing, []byte("{}"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	tests := []struct {
		name      string
		ref       string
		wantErr   bool
		wantErrIs error
	}{
		{name: "empty is valid", ref: ""},
		{name: "bare host is valid", ref: "agent.example"},
		{name: "loopback host:port is valid", ref: "localhost:9000"},
		{name: "full https url is valid", ref: "https://agent.example/card.json"},
		{name: "existing absolute path is valid", ref: existing},
		{name: "existing file url is valid", ref: "file://" + existing},
		{name: "missing absolute path is a usage error", ref: "/no/such/card.json", wantErr: true, wantErrIs: os.ErrNotExist},
		{name: "missing relative path is a usage error", ref: "./no-such-card.json", wantErr: true, wantErrIs: os.ErrNotExist},
		{name: "missing file url is a usage error", ref: "file:///no/such/card.json", wantErr: true, wantErrIs: os.ErrNotExist},
		{name: "malformed url is a usage error", ref: "http://exa mple.com", wantErr: true},
		{name: "url without host is a usage error", ref: "https://", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var u URLOrPath
			if err := u.Set(tt.ref); err != nil {
				t.Fatalf("URLOrPath.Set(%q) error = %v", tt.ref, err)
			}
			err := u.Validate()
			if tt.wantErr != (err != nil) {
				t.Fatalf("URLOrPath.Validate() error = %v, want error = %v", err, tt.wantErr)
			}
			if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
				t.Errorf("URLOrPath.Validate() error = %v, want errors.Is %v", err, tt.wantErrIs)
			}
		})
	}
}

func TestURLOrPathIsSet(t *testing.T) {
	t.Parallel()

	var u URLOrPath
	if u.IsSet() {
		t.Fatalf("URLOrPath.IsSet() = true, want false for the zero value")
	}
	if err := u.Set("agent.example"); err != nil {
		t.Fatalf("URLOrPath.Set() error = %v", err)
	}
	if !u.IsSet() {
		t.Fatalf("URLOrPath.IsSet() = false, want true after Set")
	}
}
