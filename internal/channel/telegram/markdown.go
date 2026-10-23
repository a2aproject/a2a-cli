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

package telegram

import (
	"strings"
)

// specialChars must be escaped outside entities in MarkdownV2.
const specialChars = "_*[]()~`>#+-=|{}.!\\"

func escape(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if strings.ContainsRune(specialChars, r) {
			sb.WriteByte('\\')
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// escapeCode escapes the content of code entities.
func escapeCode(s string) string {
	return strings.NewReplacer("\\", "\\\\", "`", "\\`").Replace(s)
}

// ToMarkdownV2 converts the Markdown agents write to Telegram's MarkdownV2:
// code blocks, inline code, bold, headings and links keep their formatting
// and everything else is escaped.
func ToMarkdownV2(md string) string {
	var sb strings.Builder
	lines := strings.Split(md, "\n")
	inCode := false
	for i, line := range lines {
		if i > 0 {
			sb.WriteByte('\n')
		}
		trimmed := strings.TrimSpace(line)
		if lang, ok := strings.CutPrefix(trimmed, "```"); ok {
			if inCode {
				sb.WriteString("```")
			} else {
				sb.WriteString("```" + escapeCode(lang))
			}
			inCode = !inCode
			continue
		}
		if inCode {
			sb.WriteString(escapeCode(line))
			continue
		}
		if h := strings.TrimLeft(trimmed, "#"); h != trimmed && strings.HasPrefix(h, " ") {
			sb.WriteString("*" + inline(strings.TrimSpace(h)) + "*")
			continue
		}
		sb.WriteString(inline(line))
	}
	if inCode {
		sb.WriteString("\n```")
	}
	return sb.String()
}

// inline converts one line outside code blocks.
func inline(s string) string {
	var sb strings.Builder
	for len(s) > 0 {
		switch {
		case s[0] == '`':
			if end := strings.IndexByte(s[1:], '`'); end >= 0 {
				sb.WriteString("`" + escapeCode(s[1:1+end]) + "`")
				s = s[end+2:]
				continue
			}
		case strings.HasPrefix(s, "**"):
			if end := strings.Index(s[2:], "**"); end > 0 {
				sb.WriteString("*" + escape(s[2:2+end]) + "*")
				s = s[end+4:]
				continue
			}
		case s[0] == '[':
			if text, url, rest, ok := cutLink(s); ok {
				sb.WriteString("[" + escape(text) + "](" + strings.NewReplacer("\\", "\\\\", ")", "\\)").Replace(url) + ")")
				s = rest
				continue
			}
		}
		r, size := firstRune(s)
		sb.WriteString(escape(r))
		s = s[size:]
	}
	return sb.String()
}

func cutLink(s string) (text, url, rest string, ok bool) {
	closeText := strings.Index(s, "](")
	if closeText < 1 {
		return "", "", "", false
	}
	closeURL := strings.IndexByte(s[closeText+2:], ')')
	if closeURL < 1 {
		return "", "", "", false
	}
	text = s[1:closeText]
	url = s[closeText+2 : closeText+2+closeURL]
	if strings.ContainsAny(text, "[]") || strings.ContainsAny(url, " \n") {
		return "", "", "", false
	}
	return text, url, s[closeText+3+closeURL:], true
}

func firstRune(s string) (string, int) {
	for i := range s {
		if i > 0 {
			return s[:i], i
		}
	}
	return s, len(s)
}
