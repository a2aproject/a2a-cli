// Copyright 2025 The A2A Authors
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

// Package testutil provides shared helpers for tests across the a2a CLI.
package testutil

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

// AllArtifactText concatenates the text of every part of every artifact on a task.
func AllArtifactText(task *a2a.Task) string {
	var sb strings.Builder
	for _, art := range task.Artifacts {
		for _, p := range art.Parts {
			sb.WriteString(p.Text())
		}
	}
	return sb.String()
}

// MustWriteTmpCardFile creates a card.json file in a temporary directory on the local
// file system and returns its path.
func MustWriteTmpCardFile(t *testing.T, card *a2a.AgentCard) string {
	t.Helper()
	cardBytes, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("json.Marshal(card) error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "card.json")
	if err := os.WriteFile(path, cardBytes, os.ModePerm); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	return path
}
