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

package email

import (
	"bytes"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/a2aproject/a2a-cli/internal/channel"
)

func TestAuthentic(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		results []string
		want    bool
	}{
		{name: "dmarc pass", results: []string{"mx.example.com; dkim=pass header.d=other.org; dmarc=pass header.from=alice.org"}, want: true},
		{name: "aligned subdomain dkim", results: []string{"mx.example.com; dkim=pass header.d=mail.alice.org"}, want: true},
		{name: "aligned dkim", results: []string{"mx.example.com; dkim=pass header.d=alice.org; dmarc=none"}, want: true},
		{name: "dkim of a parent domain", results: []string{"mx.example.com; dkim=pass header.d=org"}, want: false},
		{name: "unaligned dkim", results: []string{"mx.example.com; dkim=pass header.d=evil.com; dmarc=fail"}, want: false},
		{name: "untrusted server", results: []string{"evil.com; dmarc=pass"}, want: false},
		{name: "no results", want: false},
	}
	for _, tc := range tests {
		if got := Authentic(tc.results, "mx.example.com", "alice.org"); got != tc.want {
			t.Errorf("Authentic(%s) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestStripQuoted(t *testing.T) {
	t.Parallel()
	in := "approve\r\n\r\nOn Mon, 1 Jan 2026, coder wrote:\r\n> coder wants to run git push\r\n"
	if got := StripQuoted(in); got != "approve" {
		t.Fatalf("StripQuoted() = %q, want %q", got, "approve")
	}
}

const rawMail = "From: Alice <Alice@alice.org>\r\n" +
	"To: coder@example.com\r\n" +
	"Subject: Re: fix the test\r\n" +
	"Message-ID: <m2@alice.org>\r\n" +
	"In-Reply-To: <m1@example.com>\r\n" +
	"References: <root@alice.org> <m1@example.com>\r\n" +
	"Authentication-Results: mx.example.com; dmarc=pass header.from=alice.org\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"\r\n" +
	"Please push it.\r\n" +
	"> quoted\r\n"

func TestParse(t *testing.T) {
	t.Parallel()
	c := New(Config{Address: "coder@example.com", TrustedAuthServ: "mx.example.com"}, nil)
	got, ok, err := c.parse([]byte(rawMail))
	if err != nil || !ok {
		t.Fatalf("parse() = %v, %v, want a message", ok, err)
	}
	want := channel.Inbound{
		Channel: "email", MessageID: "m2@alice.org", Identity: "mail:alice@alice.org", Name: "Alice",
		Conv: ConvKey("alice@alice.org", "root@alice.org", "Re: fix the test"), Direct: true, Authentic: true,
		Text: "Please push it.",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("parse() wrong result (-want +got) diff = %s", diff)
	}

	for name, header := range map[string]string{
		"auto-reply": "Auto-Submitted: auto-replied\r\n",
		"list":       "List-Id: <dev.lists.example.com>\r\n",
		"own mail":   "",
	} {
		raw := header + rawMail
		if name == "own mail" {
			raw = strings.Replace(rawMail, "Alice <Alice@alice.org>", "coder@example.com", 1)
		}
		if _, ok, err := c.parse([]byte(raw)); err != nil || ok {
			t.Errorf("parse(%s) = %v, %v, want dropped", name, ok, err)
		}
	}
}

func TestWriteMessage(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := writeMessage(&buf, mailHeader("coder@example.com", "alice@alice.org"), channel.Outbound{
		Text: "Done.", Files: []channel.File{{Name: "a.txt", MediaType: "text/plain", Data: []byte("x")}},
	}); err != nil {
		t.Fatalf("writeMessage() error = %v", err)
	}
	for _, want := range []string{"Done.", `filename=a.txt`, "To: <alice@alice.org>"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("writeMessage() = %s, want it to contain %q", buf.String(), want)
		}
	}
}
