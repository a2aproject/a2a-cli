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
	"log/slog"
	"net/http"
	"strings"

	"github.com/a2aproject/a2a-cli/skills"
)

// Skill returns the a2a-relay SKILL.md with links to the relay at base. An
// empty base leaves a placeholder.
func Skill(base string) string {
	if base == "" {
		base = "https://<relay>"
	}
	return strings.ReplaceAll(skills.A2ARelay, "{{BASE}}", strings.TrimSuffix(base, "/"))
}

func (s *Server) handleSkill(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	if _, err := w.Write([]byte(Skill(s.base(r)))); err != nil {
		slog.Debug("writing skill", "error", err)
	}
}
