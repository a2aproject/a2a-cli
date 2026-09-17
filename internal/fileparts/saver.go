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

// Package fileparts provides utilities for persisting raw byte parts carried
// by agent responses to a local directory.
package fileparts

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

// Write records a single write performed by a Saver.
type Write struct {
	// Path is the location the bytes were written to.
	Path string
	// Bytes is the number of bytes written in this operation.
	Bytes int
	// Written reports whether this operation was the first write to the file
	// during the current run.
	Written bool
}

// Saver writes raw file parts of agent responses into a directory.
// Parts carrying raw bytes without a filename get a filename derived from the
// message or artifact they are a part of.
//
// Files are managed according to the following rules:
//  1. Parts from the same Message/Artifact are always written together grouped by the filename.
//  2. When {filename} does not exist a new file is created.
//  3. When {filename} exists but hasn't been seen by the current CLI invocation it gets overwritten.
//  4. When {filename} exists and is associated with the same Artifact.ID the behavior depends on the
//     artifact update event append flag value: parts are appended to the existing file if append=true,
//     and overwrite the content otherwise.
//  5. When {filename} exists but is associated with a different Artifact.ID a new file is created
//     with ' (n)' suffix where n is the number of files with the same name. This way when an agent
//     creates multiple artifacts containing different file revisions we persist all of them.
type Saver struct {
	dir        string
	nameToPath map[string][]reservedPath
}

// NewSaver returns a [Saver] for the provided directory.
func NewSaver(dir string) *Saver {
	return &Saver{dir: dir, nameToPath: make(map[string][]reservedPath)}
}

// Save writes every raw file part carried by event and returns a record of each write.
// The method returns as soon as the first error is encountered. An error can be returned
// with non-empty [Write] slice if some writes succeeded.
func (s *Saver) Save(e a2a.Event) ([]Write, error) {
	switch v := e.(type) {
	case *a2a.Task:
		return s.saveTask(v)
	case *a2a.Message:
		return s.saveMessage(v)
	case *a2a.TaskArtifactUpdateEvent:
		return s.saveArtifact(v.Artifact, v.Append)
	case *a2a.TaskStatusUpdateEvent:
		return s.saveMessage(v.Status.Message)
	default:
		return nil, nil
	}
}

func (s *Saver) saveTask(t *a2a.Task) ([]Write, error) {
	var saved []Write
	for _, art := range t.Artifacts {
		files, err := s.saveArtifact(art, false)
		saved = append(saved, files...)
		if err != nil {
			return saved, err
		}
	}
	files, err := s.saveMessage(t.Status.Message)
	saved = append(saved, files...)
	return saved, err
}

func (s *Saver) saveArtifact(art *a2a.Artifact, appendChunk bool) ([]Write, error) {
	if art == nil {
		return nil, nil
	}
	fallbackName := art.Name
	if fallbackName == "" {
		fallbackName = string(art.ID)
	}
	return s.save("art:"+string(art.ID), fallbackName, art.Parts, appendChunk)
}

func (s *Saver) saveMessage(m *a2a.Message) ([]Write, error) {
	if m == nil {
		return nil, nil
	}
	return s.save("msg:"+m.ID, "status-msg", m.Parts, false)
}

func (s *Saver) save(owner, fallback string, parts a2a.ContentParts, appendChunk bool) ([]Write, error) {
	grouped := map[string][]byte{}
	var order []string
	for _, part := range parts {
		raw := part.Raw()
		if raw == nil {
			continue
		}
		name := resolveName(part, fallback)
		if _, seen := grouped[name]; !seen {
			order = append(order, name)
		}
		grouped[name] = append(grouped[name], raw...)
	}

	var saved []Write
	for _, name := range order {
		bytes := grouped[name]

		path, reservedNew := s.reservePath(owner, name)
		mode := os.O_TRUNC
		if appendChunk && !reservedNew {
			mode = os.O_APPEND
		}
		if err := writeFile(path, bytes, mode); err != nil {
			return saved, err
		}
		saved = append(saved, Write{
			Path:    path,
			Bytes:   len(bytes),
			Written: reservedNew,
		})
	}
	return saved, nil
}

// reservePath returns (path, true) when a new path was reserved and (path, false) if
// an existing reservation was reused.
func (s *Saver) reservePath(owner, name string) (string, bool) {
	paths := s.nameToPath[name]
	existingIdx := slices.IndexFunc(paths, func(p reservedPath) bool {
		return p.owner == owner
	})
	if existingIdx >= 0 {
		return paths[existingIdx].path, false
	}
	var path string
	if len(paths) == 0 {
		path = filepath.Join(s.dir, name)
	} else {
		ext := filepath.Ext(name)
		stem := strings.TrimSuffix(name, ext)
		nextName := fmt.Sprintf("%s (%d)%s", stem, len(paths), ext)
		path = filepath.Join(s.dir, nextName)
	}
	s.nameToPath[name] = append(paths, reservedPath{path: path, owner: owner})
	return path, true
}

func writeFile(path string, data []byte, mode int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating directory for %q: %w", path, err)
	}
	flag := os.O_CREATE | os.O_WRONLY | mode
	f, err := os.OpenFile(path, flag, 0o644)
	if err != nil {
		return fmt.Errorf("opening %q: %w", path, err)
	}
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return fmt.Errorf("file %q operation: %w", path, err)
	}
	return nil
}

// resolveName returns a safe name where any directory components from an untrusted name are
// removed so that a part cannot be written outside the target directory.
func resolveName(part *a2a.Part, fallback string) string {
	name := cleanName(part.Filename)
	if name == "" {
		name = cleanName(fallback)
	}
	if name == "" {
		name = "filepart"
	}
	if mediaType := part.MediaType; filepath.Ext(name) == "" && mediaType != "" {
		if i := strings.IndexByte(mediaType, ';'); i >= 0 { // ; separates type parameters from type
			mediaType = mediaType[:i]
		}
		mediaType = strings.ToLower(strings.TrimSpace(mediaType))
		if ext, ok := preferredExtensions[mediaType]; ok {
			name += ext
		}
	}
	return name
}

// cleanName reduces an untrusted path to a single safe file name.
func cleanName(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	base := strings.TrimSpace(filepath.Base(s))
	switch base {
	case ".", "..", string(filepath.Separator):
		return ""
	}
	return base
}

// preferredExtensions maps common media types to a canonical extension, since
// mime.ExtensionsByType returns extensions in an order that is not always the
// most recognizable one (e.g. ".jfif" for image/jpeg).
var preferredExtensions = map[string]string{
	"text/plain":       ".txt",
	"text/html":        ".html",
	"text/csv":         ".csv",
	"text/markdown":    ".md",
	"application/json": ".json",
	"application/pdf":  ".pdf",
	"application/xml":  ".xml",
	"application/zip":  ".zip",
	"application/gzip": ".gz",
	"image/png":        ".png",
	"image/jpeg":       ".jpg",
	"image/gif":        ".gif",
	"image/webp":       ".webp",
	"image/svg+xml":    ".svg",
	"audio/mpeg":       ".mp3",
	"audio/wav":        ".wav",
	"audio/ogg":        ".ogg",
	"video/mp4":        ".mp4",
	"video/webm":       ".webm",
}

type reservedPath struct {
	owner string // msg or artifact
	path  string
}
