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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		spec    string
		want    Helper
		wantErr bool
	}{
		{spec: "a2a-serve credential", want: Helper{Command: []string{"a2a-serve", "credential"}}},
		{spec: "/usr/bin/helper", want: Helper{Command: []string{"/usr/bin/helper"}}},
		{spec: "example.com=/usr/bin/helper", want: Helper{Scope: "example.com", Command: []string{"/usr/bin/helper"}}},
		{spec: "*.Example.com=helper --flag", want: Helper{Scope: "*.example.com", Command: []string{"helper", "--flag"}}},
		{spec: `helper --opt=a\ b`, want: Helper{Command: []string{"helper", "--opt=a b"}}},
		{spec: "/opt/x=y/helper", want: Helper{Command: []string{"/opt/x=y/helper"}}},
		{spec: "example.com=", wantErr: true},
		{spec: "", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.spec, func(t *testing.T) {
			t.Parallel()
			got, err := Parse(tc.spec)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Parse(%q) error = %v, want error %v", tc.spec, err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			tc.want.Spec = tc.spec
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("Parse(%q) wrong result (-want +got) diff = %s", tc.spec, diff)
			}
		})
	}
}

func TestSelect(t *testing.T) {
	t.Parallel()
	helpers, err := ParseAll([]string{
		"default",
		"*.example.com=wildcard",
		"*.dev.example.com=dev-wildcard",
		"api.example.com=exact",
		"api.example.com=exact-override",
	})
	if err != nil {
		t.Fatalf("ParseAll() error = %v", err)
	}
	tests := []struct {
		url  string
		want string
	}{
		{url: "https://api.example.com/agent", want: "exact-override"},
		{url: "https://API.example.com:8443", want: "exact-override"},
		{url: "https://foo.dev.example.com", want: "dev-wildcard"},
		{url: "https://foo.example.com", want: "wildcard"},
		{url: "https://example.com", want: "wildcard"},
		{url: "https://other.org", want: "default"},
	}
	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			t.Parallel()
			got, ok := Select(helpers, tc.url)
			if !ok || got.Command[0] != tc.want {
				t.Fatalf("Select(%q) = %v, %v, want %s", tc.url, got.Command, ok, tc.want)
			}
		})
	}

	scoped, err := ParseAll([]string{"example.com=helper"})
	if err != nil {
		t.Fatalf("ParseAll() error = %v", err)
	}
	if got, ok := Select(scoped, "https://other.org"); ok {
		t.Fatalf("Select() = %v, want no helper for a host outside every scope", got.Command)
	}
}

func writeHelper(t *testing.T, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("helper scripts need a POSIX shell")
	}
	path := filepath.Join(t.TempDir(), "helper")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestGet(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		script  string
		want    *Response
		wantErr string
	}{
		{
			name:   "headers for the uri",
			script: `[ "$1" = get ] || exit 2; uri=$(sed 's/.*"uri":"\([^"]*\)".*/\1/'); printf '{"headers":{"Authorization":["Bearer %s"]},"expires":"2030-01-02T03:04:05Z"}' "$uri"`,
			want:   &Response{Headers: map[string][]string{"Authorization": {"Bearer https://agent.example.com"}}, Expires: "2030-01-02T03:04:05Z"},
		},
		{name: "no headers", script: `cat >/dev/null; echo '{}'`, want: &Response{}},
		{name: "failure", script: `echo "no token" >&2; exit 1`, wantErr: "no token"},
		{name: "invalid json", script: `echo nope`, wantErr: "invalid response"},
		{name: "invalid expires", script: `echo '{"expires":"tomorrow"}'`, wantErr: "invalid expires"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h, err := Parse(writeHelper(t, tc.script))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			got, err := h.Get(t.Context(), "https://agent.example.com")
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("Get() error = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Get() error = %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("Get() wrong result (-want +got) diff = %s", diff)
			}
		})
	}
}
