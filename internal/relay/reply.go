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

package relay

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

// FileArtifact turns a file into an artifact named after it. Text files
// become text parts, everything else raw bytes.
func FileArtifact(path string) (*a2a.Artifact, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	name := filepath.Base(path)
	mediaType := mime.TypeByExtension(filepath.Ext(path))
	if mediaType == "" {
		mediaType = http.DetectContentType(data)
	}
	mediaType = strings.Split(mediaType, ";")[0]
	var part *a2a.Part
	if utf8.Valid(data) && (strings.HasPrefix(mediaType, "text/") || strings.HasSuffix(mediaType, "json") || strings.HasSuffix(mediaType, "xml")) {
		part = a2a.NewTextPart(string(data))
	} else {
		part = a2a.NewRawPart(data)
	}
	part.Filename, part.MediaType = name, mediaType
	return &a2a.Artifact{ID: a2a.NewArtifactID(), Name: name, Parts: []*a2a.Part{part}}, nil
}

// ContentKey derives an idempotency key from the task and the content of
// the event, so that a retried reply is applied once.
func ContentKey(taskID string, ev Event) string {
	h := sha256.New()
	h.Write(fmt.Appendf(nil, "%s\x00%s\x00%s", taskID, ev.State, ev.Text))
	for _, a := range ev.Artifacts {
		h.Write(fmt.Appendf(nil, "\x00%s", a.Name))
		for _, p := range a.Parts {
			h.Write(fmt.Appendf(nil, "\x00%s\x00%s\x00", p.Filename, p.Text()))
			h.Write(p.Raw())
		}
	}
	return "auto-" + hex.EncodeToString(h.Sum(nil))[:32]
}
