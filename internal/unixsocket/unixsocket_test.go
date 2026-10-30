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

package unixsocket

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestTransport(t *testing.T) {
	t.Parallel()
	// t.TempDir paths can exceed the Unix socket path length limit.
	dir, err := os.MkdirTemp("", "a2a")
	if err != nil {
		t.Fatalf("os.MkdirTemp() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("os.RemoveAll(%q) error = %v", dir, err)
		}
	})
	sock := filepath.Join(dir, "a2a.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("net.Listen(%q) error = %v", sock, err)
	}
	server := &httptest.Server{Listener: ln, Config: &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.WriteString(w, r.Host); err != nil {
			t.Errorf("io.WriteString() error = %v", err)
		}
	})}}
	server.Start()
	t.Cleanup(server.Close)

	client := &http.Client{Transport: Transport(sock)}
	for _, host := range []string{"agent.invalid", "other.invalid:8080"} {
		t.Run(host, func(t *testing.T) {
			t.Parallel()
			resp, err := client.Get("http://" + host + "/")
			if err != nil {
				t.Fatalf("client.Get(%q) error = %v", host, err)
			}
			defer func() {
				if err := resp.Body.Close(); err != nil {
					t.Errorf("resp.Body.Close() error = %v", err)
				}
			}()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("io.ReadAll() error = %v", err)
			}
			if got := string(body); got != host {
				t.Fatalf("client.Get(%q) Host = %q, want %q", host, got, host)
			}
		})
	}
}
