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
	"net/url"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestNormalizeResource(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"https://Agent.Example.com/":         "https://agent.example.com",
		"https://agent.example.com:443/a2a/": "https://agent.example.com/a2a",
		"http://127.0.0.1:8080/a?x=1#frag":   "http://127.0.0.1:8080/a",
		"http://localhost:80":                "http://localhost",
		"agent.example.com:50051":            "https://agent.example.com:50051",
		"":                                   "",
	}
	for in, want := range tests {
		if got := NormalizeResource(in); got != want {
			t.Errorf("NormalizeResource(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResourceCovers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		resource, target string
		want             bool
	}{
		{"https://agent.example.com", "https://agent.example.com/message:send", true},
		{"https://agent.example.com/a2a", "https://agent.example.com/a2a/v1/tasks", true},
		{"https://agent.example.com/a2a", "https://agent.example.com/a2a", true},
		{"https://agent.example.com/a2a", "https://agent.example.com/a2ab", false},
		{"https://agent.example.com", "https://other.example.com", false},
		{"https://agent.example.com", "http://agent.example.com", false},
	}
	for _, tc := range tests {
		if got := resourceCovers(tc.resource, tc.target); got != tc.want {
			t.Errorf("resourceCovers(%q, %q) = %v, want %v", tc.resource, tc.target, got, tc.want)
		}
	}
}

func TestCheckSecureURL(t *testing.T) {
	t.Parallel()
	tests := map[string]bool{
		"https://login.example.com/token": true,
		"http://127.0.0.1:9000/token":     true,
		"http://localhost/token":          true,
		"http://[::1]:9000/token":         true,
		"http://login.example.com/token":  false,
		"ftp://login.example.com":         false,
		"/token":                          false,
	}
	for in, ok := range tests {
		if err := checkSecureURL(in); (err == nil) != ok {
			t.Errorf("checkSecureURL(%q) error = %v, want ok %v", in, err, ok)
		}
	}
}

func TestServerMetadataURLs(t *testing.T) {
	t.Parallel()
	tests := map[string][]string{
		"https://login.example.com": {
			"https://login.example.com/.well-known/oauth-authorization-server",
			"https://login.example.com/.well-known/openid-configuration",
		},
		"https://login.example.com/tenant/v2.0/": {
			"https://login.example.com/.well-known/oauth-authorization-server/tenant/v2.0",
			"https://login.example.com/tenant/v2.0/.well-known/openid-configuration",
		},
	}
	for in, want := range tests {
		if diff := cmp.Diff(want, serverMetadataURLs(in)); diff != "" {
			t.Errorf("serverMetadataURLs(%q) wrong result (-want +got) diff = %s", in, diff)
		}
	}
}

func TestResourceMetadataURLs(t *testing.T) {
	t.Parallel()
	tests := map[string][]string{
		"https://agent.example.com": {"https://agent.example.com/.well-known/oauth-protected-resource"},
		"https://agent.example.com/a2a/v1/": {
			"https://agent.example.com/.well-known/oauth-protected-resource/a2a/v1",
			"https://agent.example.com/.well-known/oauth-protected-resource",
		},
	}
	for in, want := range tests {
		u, err := url.Parse(in)
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(want, resourceMetadataURLs(u)); diff != "" {
			t.Errorf("resourceMetadataURLs(%q) wrong result (-want +got) diff = %s", in, diff)
		}
	}
}
