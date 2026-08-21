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
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
)

func parseParts(t *testing.T, args ...string) *Parts {
	t.Helper()
	var p Parts
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	p.Attach(fs)
	if err := fs.Parse(args); err != nil {
		t.Fatalf("fs.Parse(%v) error = %v", args, err)
	}
	return &p
}

func TestPartsParse(t *testing.T) {
	t.Parallel()

	t.Run("preserves command-line order across flags", func(t *testing.T) {
		t.Parallel()
		parts, err := parseParts(t, "--text-part", "first", "--file-part", "https://example.com/pic.png", "--text-part", "third").Parse()
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if len(parts) != 3 {
			t.Fatalf("len(parts) = %d, want 3", len(parts))
		}
		if parts[0].Text() != "first" {
			t.Errorf("parts[0].Text() = %q, want %q", parts[0].Text(), "first")
		}
		if parts[1].URL() != "https://example.com/pic.png" {
			t.Errorf("parts[1].URL() = %q, want the image URL", parts[1].URL())
		}
		if parts[2].Text() != "third" {
			t.Errorf("parts[2].Text() = %q, want %q", parts[2].Text(), "third")
		}
	})

	t.Run("local file is inlined and media-type binds to it", func(t *testing.T) {
		t.Parallel()
		binPath := filepath.Join(t.TempDir(), "report.bin")
		if err := os.WriteFile(binPath, []byte("rawbytes"), 0o644); err != nil {
			t.Fatalf("os.WriteFile() error = %v", err)
		}

		parts, err := parseParts(t, "--file-part", binPath, "--media-type", "application/pdf").Parse()
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if len(parts) != 1 {
			t.Fatalf("len(parts) = %d, want 1", len(parts))
		}
		p := parts[0]
		if string(p.Raw()) != "rawbytes" {
			t.Errorf("part raw bytes = %q, want %q", string(p.Raw()), "rawbytes")
		}
		if p.MediaType != "application/pdf" {
			t.Errorf("part.MediaType = %q, want %q", p.MediaType, "application/pdf")
		}
		if p.Filename != "report.bin" {
			t.Errorf("part.Filename = %q, want %q", p.Filename, "report.bin")
		}
	})

	t.Run("data part from inline JSON", func(t *testing.T) {
		t.Parallel()
		parts, err := parseParts(t, "--data-part", `{"key":"value"}`).Parse()
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		data, ok := parts[0].Data().(map[string]any)
		if !ok {
			t.Fatalf("parts[0].Data() type = %T, want map", parts[0].Data())
		}
		if data["key"] != "value" {
			t.Errorf("data[key] = %v, want %q", data["key"], "value")
		}
	})

	t.Run("data part from file", func(t *testing.T) {
		t.Parallel()
		dataPath := filepath.Join(t.TempDir(), "payload.json")
		if err := os.WriteFile(dataPath, []byte(`{"from":"file"}`), 0o644); err != nil {
			t.Fatalf("os.WriteFile() error = %v", err)
		}
		parts, err := parseParts(t, "--data-part", dataPath).Parse()
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		data, ok := parts[0].Data().(map[string]any)
		if !ok {
			t.Fatalf("parts[0].Data() type = %T, want map", parts[0].Data())
		}
		if data["from"] != "file" {
			t.Errorf("data[from] = %v, want %q", data["from"], "file")
		}
	})

	t.Run("data that is neither a file nor valid JSON errors on Parse", func(t *testing.T) {
		t.Parallel()
		if _, err := parseParts(t, "--data-part", "not json and not a file").Parse(); err == nil {
			t.Fatal("Parse() with invalid --data-part should fail")
		}
	})
}

func TestPartsMediaTypeWithoutPrecedingFlagErrors(t *testing.T) {
	t.Parallel()

	var p Parts
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	p.Attach(fs)
	if err := fs.Parse([]string{"--media-type", "text/plain"}); err == nil {
		t.Fatal("fs.Parse(--media-type with no preceding part) error = nil, want error")
	}
}
