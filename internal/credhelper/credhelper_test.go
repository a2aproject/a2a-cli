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

	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/google/go-cmp/cmp"
)

func TestParse(t *testing.T) {
	t.Parallel()
	tests := []struct {
		spec    string
		want    Helper
		wantErr bool
	}{
		{spec: "/usr/bin/helper", want: Helper{Command: []string{"/usr/bin/helper"}}},
		{spec: "example.com=/usr/bin/helper", want: Helper{Scope: "example.com", Command: []string{"/usr/bin/helper"}}},
		{spec: "*.Example.com=helper", want: Helper{Scope: "*.example.com", Command: []string{"helper"}}},
		{spec: "/opt/my helper", want: Helper{Command: []string{"/opt/my helper"}}},
		{spec: "/opt/x=y/helper", want: Helper{Command: []string{"/opt/x=y/helper"}}},
		{spec: "example.com=", wantErr: true},
		{spec: "localhost:8080=helper", wantErr: true},
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
	all := []string{
		"default",
		"*.example.com=wildcard",
		"*.dev.example.com=dev-wildcard",
		"api.example.com=exact",
		"api.example.com=exact-override",
	}
	tests := []struct {
		specs  []string
		url    string
		want   string
		wantOK bool
	}{
		{specs: all, url: "https://api.example.com/agent", want: "exact-override", wantOK: true},
		{specs: all, url: "https://API.example.com:8443", want: "exact-override", wantOK: true},
		{specs: all, url: "https://foo.dev.example.com", want: "dev-wildcard", wantOK: true},
		{specs: all, url: "https://foo.example.com", want: "wildcard", wantOK: true},
		{specs: all, url: "https://example.com", want: "wildcard", wantOK: true},
		{specs: all, url: "https://other.org", want: "default", wantOK: true},
		{specs: []string{"example.com=helper"}, url: "https://other.org", wantOK: false},
	}
	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			t.Parallel()
			var helpers []Helper
			for _, spec := range tc.specs {
				h, err := Parse(spec)
				if err != nil {
					t.Fatalf("Parse(%q) error = %v", spec, err)
				}
				helpers = append(helpers, h)
			}
			got, ok := Select(helpers, tc.url)
			if ok != tc.wantOK {
				t.Fatalf("Select(%q) ok = %v, want %v", tc.url, ok, tc.wantOK)
			}
			if ok && got.Command[0] != tc.want {
				t.Fatalf("Select(%q) = %v, want %s", tc.url, got.Command, tc.want)
			}
		})
	}
}

// writeHelper returns a helper running script with sh. Executing a file
// written while parallel tests fork can fail with ETXTBSY (golang/go#22315).
func writeHelper(t *testing.T, script string) Helper {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("helper scripts need a POSIX shell")
	}
	path := filepath.Join(t.TempDir(), "helper")
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	return Helper{Spec: path, Command: []string{"sh", path}}
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
			h := writeHelper(t, tc.script)
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

func TestSourceHeaders(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		expires  string
		uris     []string
		wantRuns int
	}{
		{name: "reused for the same uri", uris: []string{"https://a.example.com", "https://a.example.com"}, wantRuns: 1},
		{name: "reused until expiry", expires: "2999-01-01T00:00:00Z", uris: []string{"https://a.example.com", "https://a.example.com"}, wantRuns: 1},
		{name: "refreshed after expiry", expires: "2000-01-01T00:00:00Z", uris: []string{"https://a.example.com", "https://a.example.com"}, wantRuns: 2},
		{name: "run per uri", uris: []string{"https://a.example.com", "https://a.example.com/agent"}, wantRuns: 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runs := filepath.Join(t.TempDir(), "runs")
			h := writeHelper(t, `echo run >>`+runs+`; printf '{"headers":{"X":["v"]},"expires":"%s"}' "`+tc.expires+`"`)
			src := &Source{Helpers: []Helper{h}}
			for _, uri := range tc.uris {
				_, resp, err := src.Headers(t.Context(), uri)
				if err != nil {
					t.Fatalf("Source.Headers(%q) error = %v", uri, err)
				}
				if diff := cmp.Diff(map[string][]string{"X": {"v"}}, resp.Headers); diff != "" {
					t.Fatalf("Source.Headers(%q) wrong result (-want +got) diff = %s", uri, diff)
				}
			}
			out, err := os.ReadFile(runs)
			if err != nil {
				t.Fatalf("os.ReadFile() error = %v", err)
			}
			if got := strings.Count(string(out), "run"); got != tc.wantRuns {
				t.Fatalf("helper runs = %d, want %d", got, tc.wantRuns)
			}
		})
	}
}

func TestInterceptor(t *testing.T) {
	t.Parallel()
	helper := writeHelper(t, `uri=$(sed 's/.*"uri":"\([^"]*\)".*/\1/'); printf '{"headers":{"Authorization":["Bearer %s"]}}' "$uri"`)
	tests := []struct {
		name    string
		scope   string
		baseURL string
		params  a2aclient.ServiceParams
		want    a2aclient.ServiceParams
	}{
		{
			name:    "adds the headers for the interface url",
			baseURL: "https://agent.example.com/a2a",
			params:  a2aclient.ServiceParams{},
			want:    a2aclient.ServiceParams{"authorization": {"Bearer https://agent.example.com/a2a"}},
		},
		{
			name:    "keeps an explicit service param",
			baseURL: "https://agent.example.com",
			params:  a2aclient.ServiceParams{"authorization": {"Bearer flag"}},
			want:    a2aclient.ServiceParams{"authorization": {"Bearer flag"}},
		},
		{
			name:    "skips a host outside the scope",
			scope:   "agent.example.com",
			baseURL: "https://other.example.com",
			params:  a2aclient.ServiceParams{},
			want:    a2aclient.ServiceParams{},
		},
		{
			name:    "matches a grpc address without a scheme",
			scope:   "agent.example.com",
			baseURL: "agent.example.com:443",
			params:  a2aclient.ServiceParams{},
			want:    a2aclient.ServiceParams{"authorization": {"Bearer https://agent.example.com:443"}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := helper
			h.Scope = tc.scope
			i := &Interceptor{Source: &Source{Helpers: []Helper{h}}}
			req := &a2aclient.Request{BaseURL: tc.baseURL, ServiceParams: tc.params}
			if _, _, err := i.Before(t.Context(), req); err != nil {
				t.Fatalf("Interceptor.Before() error = %v", err)
			}
			if diff := cmp.Diff(tc.want, req.ServiceParams); diff != "" {
				t.Fatalf("Interceptor.Before() wrong result (-want +got) diff = %s", diff)
			}
		})
	}
}
