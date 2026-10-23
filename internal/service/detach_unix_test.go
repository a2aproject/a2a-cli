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

//go:build unix

package service

import (
	"path/filepath"
	"testing"
	"time"
)

func TestDetachReportsExit(t *testing.T) {
	t.Parallel()
	exited, err := Detach(Unit{Exe: "sh", Args: []string{"-c", "exit 3"}, LogPath: filepath.Join(t.TempDir(), "log")})
	if err != nil {
		t.Fatalf("Detach() error = %v", err)
	}
	select {
	case <-exited:
	case <-time.After(10 * time.Second):
		t.Fatalf("Detach() exited channel not closed after the process exited")
	}
}
