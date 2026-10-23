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

package agentsrv

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"

	"github.com/a2aproject/a2a-cli/internal/auth"
	"github.com/a2aproject/a2a-cli/internal/channel"
)

func TestRendererSplitsStreamToChannelLimit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		maxText int
		text    string
	}{
		{name: "default limit", maxText: 400, text: strings.Repeat("line of text\n", 100)},
		{name: "limit below the progress reserve", maxText: 150, text: strings.Repeat("line of text\n", 50)},
		{name: "one byte limit", maxText: 1, text: "hello\nworld"},
		{name: "rune wider than the limit", maxText: 3, text: "héllo wörld"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ch := newFakeChannel("fake")
			ch.maxText = tc.maxText
			rd := newRenderer(nil, ch, "conv", channel.Ref{}, auth.Principal{})
			done := make(chan struct{})
			go func() {
				defer close(done)
				rd.onArtifact(&a2a.Artifact{ID: "a", Parts: a2a.ContentParts{a2a.NewTextPart(tc.text)}}, false)
				rd.flush(context.Background(), true)
			}()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Fatalf("flush() did not return")
			}
			var got strings.Builder
			for _, id := range ch.order {
				text := ch.msgs[id].Text
				if len(text) > tc.maxText {
					t.Fatalf("flush() sent %d bytes, want at most %d", len(text), tc.maxText)
				}
				got.WriteString(text)
			}
			if got.String() != tc.text {
				t.Fatalf("flush() sent %q, want %q", got.String(), tc.text)
			}
		})
	}
}
