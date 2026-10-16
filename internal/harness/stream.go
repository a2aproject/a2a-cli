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

package harness

import (
	"github.com/a2aproject/a2a-go/v2/a2a"
)

// TextStream turns incremental harness text into artifact events: the first
// chunk creates the artifact, later chunks append to it.
type TextStream struct {
	Name       string
	artifactID a2a.ArtifactID
	needsBreak bool
}

// Write returns the event for the next chunk of text, or nil for empty text.
func (s *TextStream) Write(info a2a.TaskInfoProvider, text string) *a2a.TaskArtifactUpdateEvent {
	if text == "" {
		return nil
	}
	if s.needsBreak {
		text = "\n\n" + text
		s.needsBreak = false
	}
	if s.artifactID == "" {
		evt := a2a.NewArtifactEvent(info, a2a.NewTextPart(text))
		evt.Artifact.Name = s.Name
		s.artifactID = evt.Artifact.ID
		return evt
	}
	evt := a2a.NewArtifactUpdateEvent(info, s.artifactID, a2a.NewTextPart(text))
	evt.Append = true
	return evt
}

// Break separates the next chunk from previous text with a blank line, used
// when the harness starts a new message or text block.
func (s *TextStream) Break() {
	if s.artifactID != "" {
		s.needsBreak = true
	}
}

// Started reports whether any text was written.
func (s *TextStream) Started() bool { return s.artifactID != "" }
