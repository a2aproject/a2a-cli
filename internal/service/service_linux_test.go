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

//go:build linux

package service

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestSystemdUnitEscaping(t *testing.T) {
	t.Parallel()
	got, err := SystemdUnit(Unit{
		Description: "a2a server coder (/home/u/100%$x)",
		Exe:         "/usr/local/bin/a2a",
		Args:        []string{"server", "--dir", "/home/u/100%$x"},
		Dir:         "/home/u/100%$x",
		LogPath:     "/home/u/100%$x/server.log",
	})
	if err != nil {
		t.Fatalf("SystemdUnit() error = %v", err)
	}
	want := `[Unit]
Description=a2a server coder (/home/u/100%%$x)
After=network-online.target

[Service]
ExecStart=/usr/local/bin/a2a server --dir /home/u/100%%$$x
WorkingDirectory=/home/u/100%%$x
StandardOutput=append:/home/u/100%%$x/server.log
StandardError=append:/home/u/100%%$x/server.log
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
`
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("SystemdUnit() wrong result (-want +got) diff = %s", diff)
	}
}

func TestSystemdUnitRejectsNewlines(t *testing.T) {
	t.Parallel()
	if _, err := SystemdUnit(Unit{Description: "x", Exe: "/bin/a2a", Dir: "/home/u/a\nExecStartPre=/bin/evil", LogPath: "/tmp/l"}); err == nil {
		t.Fatalf("SystemdUnit() error = nil, want error")
	}
}

func TestSystemdUnit(t *testing.T) {
	t.Parallel()
	got, err := SystemdUnit(Unit{
		Description: "a2a server coder (/home/u/my app)",
		Exe:         "/usr/local/bin/a2a",
		Args:        []string{"server", "--dir", "/home/u/my app"},
		Dir:         "/home/u/my app",
		LogPath:     "/home/u/my app/.a2a/state/server.log",
		Env:         []string{"PATH=/usr/bin:/bin"},
	})
	if err != nil {
		t.Fatalf("SystemdUnit() error = %v", err)
	}
	want := `[Unit]
Description=a2a server coder (/home/u/my app)
After=network-online.target

[Service]
ExecStart=/usr/local/bin/a2a server --dir "/home/u/my app"
WorkingDirectory=/home/u/my app
Environment=PATH=/usr/bin:/bin
StandardOutput=append:/home/u/my app/.a2a/state/server.log
StandardError=append:/home/u/my app/.a2a/state/server.log
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
`
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("SystemdUnit() wrong result (-want +got) diff = %s", diff)
	}
}
