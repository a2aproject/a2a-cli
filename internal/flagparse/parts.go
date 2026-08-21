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
	"encoding/json"
	"fmt"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/spf13/pflag"
)

// partKind identifies which content flag produced a part spec.
type partKind int

const (
	kindText partKind = iota
	kindFile
	kindData
)

type partSpec struct {
	kind      partKind
	value     string
	mediaType string
}

// Parts accumulates part specs in the order their flags appear on the
// command line. Set is called on each flag Value in order, so interleaved
// --text-part/--file-part/--data-part flags preserve their sequence, and
// --media-type binds to the part flag immediately preceding it.
type Parts struct {
	specs []partSpec
}

// Attach registers --text-part, --file-part, --data-part and --media-type flag
// handlers on the provided flag set.
func (p *Parts) Attach(f *pflag.FlagSet) {
	f.Var(&textPartValue{p}, "text-part", "Add a text part (repeatable, order-preserving)")
	f.Var(&filePartValue{p}, "file-part", "Add a file part from a local path (inlined) or URL (by reference); repeatable")
	f.Var(&dataPartValue{p}, "data-part", "Add a JSON data part from a file path or an inline JSON string (repeatable)")
	f.Var(&mediaTypeValue{p}, "media-type", "Media type for the immediately preceding part flag")
}

// Parse converts the accumulated part specs into A2A parts, preserving the
// order in which their flags appeared on the command line.
func (p *Parts) Parse() ([]*a2a.Part, error) {
	if len(p.specs) == 0 {
		return nil, nil
	}
	out := make([]*a2a.Part, 0, len(p.specs))
	for _, s := range p.specs {
		part, err := s.toPart()
		if err != nil {
			return nil, err
		}
		out = append(out, part)
	}
	return out, nil
}

func (p *Parts) add(kind partKind, value string) {
	p.specs = append(p.specs, partSpec{kind: kind, value: value})
}

func (p *Parts) setMediaType(mediaType string) error {
	if len(p.specs) == 0 {
		return fmt.Errorf("--media-type must follow a --text-part, --file-part, or --data-part flag")
	}
	p.specs[len(p.specs)-1].mediaType = mediaType
	return nil
}

func (s partSpec) toPart() (*a2a.Part, error) {
	switch s.kind {
	case kindText:
		p := a2a.NewTextPart(s.value)
		if s.mediaType != "" {
			p.MediaType = s.mediaType
		}
		return p, nil
	case kindFile:
		return buildFilePart(s.value, s.mediaType)
	case kindData:
		return buildDataPart(s.value, s.mediaType)
	default:
		return nil, fmt.Errorf("unknown part kind %d", s.kind)
	}
}

// buildFilePart turns a --file-part value into a Part: a local filesystem path
// is inlined as raw bytes, while a URL is passed by reference.
func buildFilePart(ref, mediaType string) (*a2a.Part, error) {
	if u, err := url.Parse(ref); err == nil {
		switch strings.ToLower(u.Scheme) {
		case "http", "https", "s3", "gs", "ftp", "ftps":
			return a2a.NewFileURLPart(a2a.URL(ref), mediaType), nil
		}
	}

	path := strings.TrimPrefix(ref, "file://")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading --file-part %q: %w", ref, err)
	}
	p := a2a.NewRawPart(data)
	p.Filename = filepath.Base(path)
	if mediaType == "" {
		mediaType = mime.TypeByExtension(filepath.Ext(path))
	}
	if mediaType != "" {
		p.MediaType = mediaType
	}
	return p, nil
}

// buildDataPart turns a --data-part value into a structured DataPart. Value
// can be a file path or an inline JSON.
func buildDataPart(ref, mediaType string) (*a2a.Part, error) {
	var data any
	if err := json.Unmarshal(RawOrInline(ref), &data); err != nil {
		return nil, fmt.Errorf("--data-part %q is not a readable file or valid JSON: %w", ref, err)
	}
	p := a2a.NewDataPart(data)
	if mediaType != "" {
		p.MediaType = mediaType
	}
	return p, nil
}

// RawOrInline returns the contents of ref when it names a readable file, and
// otherwise returns ref itself as bytes. It lets a flag value accept either a
// file path or an inline literal.
func RawOrInline(ref string) []byte {
	if raw, err := os.ReadFile(ref); err == nil {
		return raw
	}
	return []byte(ref)
}

// pflag.Value adapters. Each appends to the shared Parts as pflag encounters
// the flag, preserving command-line order across the part flags.

type textPartValue struct{ b *Parts }

func (v *textPartValue) String() string     { return "" }
func (v *textPartValue) Set(s string) error { v.b.add(kindText, s); return nil }
func (v *textPartValue) Type() string       { return "string" }

type filePartValue struct{ b *Parts }

func (v *filePartValue) String() string     { return "" }
func (v *filePartValue) Set(s string) error { v.b.add(kindFile, s); return nil }
func (v *filePartValue) Type() string       { return "path|url" }

type dataPartValue struct{ b *Parts }

func (v *dataPartValue) String() string     { return "" }
func (v *dataPartValue) Set(s string) error { v.b.add(kindData, s); return nil }
func (v *dataPartValue) Type() string       { return "path|json" }

type mediaTypeValue struct{ b *Parts }

func (v *mediaTypeValue) String() string     { return "" }
func (v *mediaTypeValue) Set(s string) error { return v.b.setMediaType(s) }
func (v *mediaTypeValue) Type() string       { return "string" }
