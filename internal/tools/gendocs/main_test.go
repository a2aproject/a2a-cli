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

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/a2aproject/a2a-cli/internal/cli"
)

func TestReferenceIsUpToDate(t *testing.T) {
	t.Parallel()
	want, err := cli.ReferencePages()
	if err != nil {
		t.Fatalf("cli.ReferencePages() error = %v", err)
	}
	dir := filepath.Join("..", "..", "..", DefaultDir)
	paths, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		got[filepath.Base(path)] = string(b)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("%s is out of date; run `go run ./internal/tools/gendocs` (-want +got) diff = %s", DefaultDir, diff)
	}
}
