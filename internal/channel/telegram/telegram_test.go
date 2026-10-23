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
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/a2aproject/a2a-cli/internal/channel"
)

// fakeBotAPI serves the Bot API methods the channel uses and records calls.
type fakeBotAPI struct {
	t       *testing.T
	mu      sync.Mutex
	updates []string
	calls   map[string][]string
}

func (f *fakeBotAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	body, err := io.ReadAll(r.Body)
	if err != nil {
		f.t.Errorf("reading %s: %v", method, err)
	}
	f.mu.Lock()
	f.calls[method] = append(f.calls[method], string(body))
	var result string
	switch method {
	case "getMe":
		result = `{"id":99,"is_bot":true,"first_name":"Coder","username":"my_coder_bot"}`
	case "getUpdates":
		result = "[" + strings.Join(f.updates, ",") + "]"
		f.updates = nil
	case "sendMessage", "editMessageText":
		result = `{"message_id":7,"date":0,"chat":{"id":42,"type":"private"}}`
	default:
		result = "true"
	}
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if _, err := io.WriteString(w, `{"ok":true,"result":`+result+`}`); err != nil {
		f.t.Errorf("writing %s: %v", method, err)
	}
}

func (f *fakeBotAPI) called(method string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls[method]...)
}

func TestChannel(t *testing.T) {
	t.Parallel()
	api := &fakeBotAPI{t: t, calls: map[string][]string{}, updates: []string{
		`{"update_id":1,"message":{"message_id":5,"date":0,"chat":{"id":42,"type":"private"},"from":{"id":42,"is_bot":false,"first_name":"Anna","username":"anna"},"text":"/start K7Q2-9XF4"}}`,
		`{"update_id":2,"callback_query":{"id":"cb1","from":{"id":42,"is_bot":false,"first_name":"Anna"},"data":"h:abcd1234:1","message":{"message_id":7,"date":1700000000,"chat":{"id":42,"type":"private"}}}}`,
	}}
	srv := httptest.NewServer(api)
	defer srv.Close()
	c := New("123:abc")
	c.ServerURL = srv.URL

	if got := c.Describe(); got != "@my_coder_bot" {
		t.Fatalf("Describe() = %q, want @my_coder_bot", got)
	}
	if got := c.LinkHint("K7Q2-9XF4"); got != "https://t.me/my_coder_bot?start=K7Q2-9XF4" {
		t.Fatalf("LinkHint() = %q", got)
	}

	ctx := t.Context()
	in := make(chan channel.Inbound, 4)
	go func() {
		if err := c.Run(ctx, in); err != nil {
			t.Errorf("Run() error = %v", err)
		}
	}()
	var got []channel.Inbound
	for len(got) < 2 {
		select {
		case m := <-in:
			got = append(got, m)
		case <-time.After(10 * time.Second):
			t.Fatalf("Run() delivered %d messages, want 2", len(got))
		}
	}
	want := []channel.Inbound{
		{Channel: "telegram", MessageID: "42:5", Identity: "tg:42", Name: "@anna", Conv: "42", Direct: true, Text: "/start K7Q2-9XF4", Authentic: true},
		{Channel: "telegram", MessageID: "cb:cb1", Identity: "tg:42", Name: "Anna", Conv: "42", Direct: true, Authentic: true,
			Action: &channel.Action{QuestionID: "abcd1234", Option: 1}, ReplyTo: channel.Ref{Conv: "42", ID: "7"}},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("Run() wrong messages (-want +got) diff = %s", diff)
	}

	ref, err := c.Send(ctx, channel.Outbound{Conv: "42", Text: "Run **git push**?", Question: &channel.Question{ID: "abcd1234", Options: []string{"Allow", "Deny"}}})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if ref.ID != "7" {
		t.Fatalf("Send() ref = %+v, want message 7", ref)
	}
	sent := api.called("sendMessage")
	if len(sent) != 1 {
		t.Fatalf("sendMessage calls = %d, want 1", len(sent))
	}
	for _, want := range []string{`Run *git push*?`, "MarkdownV2", `h:abcd1234:0`, `h:abcd1234:1`} {
		if !strings.Contains(unescapeJSON(sent[0]), want) {
			t.Fatalf("sendMessage = %s, want it to contain %q", sent[0], want)
		}
	}
	if err := c.Edit(ctx, ref, channel.Outbound{Conv: "42", Text: "✓ Allow by Anna"}); err != nil {
		t.Fatalf("Edit() error = %v", err)
	}
	if len(api.called("answerCallbackQuery")) != 1 {
		t.Fatalf("answerCallbackQuery was not called for the button click")
	}
}

// unescapeJSON undoes JSON string escaping in multipart field values.
func unescapeJSON(s string) string {
	var out string
	if err := json.Unmarshal([]byte(`"`+strings.ReplaceAll(s, `"`, `\"`)+`"`), &out); err != nil {
		return s
	}
	return out
}
