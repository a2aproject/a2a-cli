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

package cli

import (
	"bytes"
	"context"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2aclient/oauth"
	"github.com/a2aproject/a2a-go/v2/a2aclient/oauth/oauthtest"
)

// recordingHandler keeps the messages it handles.
type recordingHandler struct{ msgs *[]string }

func (h recordingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h recordingHandler) Handle(_ context.Context, r slog.Record) error {
	*h.msgs = append(*h.msgs, r.Message)
	return nil
}
func (h recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h recordingHandler) WithGroup(string) slog.Handler      { return h }

func TestLogHandler(t *testing.T) {
	t.Parallel()
	oauthPC := reflect.ValueOf(oauth.NewGrantKey).Pointer()
	otherPC := reflect.ValueOf(TestLogHandler).Pointer()
	record := func(pc uintptr, msg string) slog.Record {
		r := slog.NewRecord(time.Now(), slog.LevelWarn, msg, pc)
		r.AddAttrs(slog.String("resource", "https://agent.example.com"))
		return r
	}

	var stderr bytes.Buffer
	var passed []string
	quiet := &logHandler{cfg: &globalConfig{errOut: &stderr}, next: recordingHandler{&passed}}
	for _, r := range []slog.Record{record(oauthPC, "from oauth"), record(otherPC, "from elsewhere")} {
		if err := quiet.Handle(t.Context(), r); err != nil {
			t.Fatalf("logHandler.Handle() error = %v", err)
		}
	}
	if len(passed) != 1 || passed[0] != "from elsewhere" || stderr.Len() != 0 {
		t.Fatalf("logHandler without --verbose passed %q and printed %q, want only the non-OAuth record passed on", passed, stderr.String())
	}

	passed = nil
	verbose := (&logHandler{cfg: &globalConfig{errOut: &stderr, verbose: true}, next: recordingHandler{&passed}}).WithAttrs([]slog.Attr{slog.String("component", "oauth")})
	if !verbose.Enabled(t.Context(), slog.LevelDebug) {
		t.Fatal("logHandler.Enabled(debug) with --verbose = false, want true")
	}
	if err := verbose.Handle(t.Context(), record(oauthPC, "refreshing the OAuth access token")); err != nil {
		t.Fatalf("logHandler.Handle() error = %v", err)
	}
	if want := "# refreshing the OAuth access token component=oauth resource=https://agent.example.com\n"; stderr.String() != want || len(passed) != 0 {
		t.Fatalf("logHandler with --verbose printed %q and passed %q, want %q", stderr.String(), passed, want)
	}
}

func TestOAuthVerboseDiagnostics(t *testing.T) {
	t.Parallel()
	for _, verbose := range []bool{false, true} {
		e := newOAuthEnv(t, oauthtest.ServerOptions{}, oauthtest.AgentOptions{Card: oauthtest.CardMetadataURL})
		args := []string{"send", "-a", e.agent.URL, e.clients(), "hello"}
		if verbose {
			args = append([]string{"-v"}, args...)
		}
		_, stderr := e.mustRun(t, args...)
		for _, want := range []string{"# agent card requires OAuth authorization issuer=" + e.as.URL, "# authorizing resource=" + e.agent.URL} {
			if got := strings.Contains(stderr, want); got != verbose {
				t.Errorf("a2a send (verbose %v) stderr contains %q = %v, want %v\nstderr:\n%s", verbose, want, got, verbose, stderr)
			}
		}
	}
}
