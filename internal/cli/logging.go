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
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"strings"
)

// oauthPackage is the import path of the a2a-go OAuth package, whose
// diagnostics are shown only with --verbose.
const oauthPackage = "github.com/a2aproject/a2a-go/v2/a2aclient/oauth."

// logHandler routes the logs of a2a-go, which it writes to the logger in the
// request context. With --verbose every record is printed like [globalConfig.logf]
// output. Otherwise records go to the default slog handler as before, except the
// OAuth diagnostics, which are verbose-only.
type logHandler struct {
	cfg   *globalConfig
	next  slog.Handler
	attrs []slog.Attr
}

func newLogHandler(cfg *globalConfig) *logHandler {
	return &logHandler{cfg: cfg, next: slog.Default().Handler()}
}

// Enabled implements [slog.Handler].
func (h *logHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.cfg.verbose || h.next.Enabled(ctx, level)
}

// Handle implements [slog.Handler].
func (h *logHandler) Handle(ctx context.Context, r slog.Record) error {
	if h.cfg.verbose {
		var b strings.Builder
		b.WriteString(r.Message)
		write := func(a slog.Attr) bool {
			fmt.Fprintf(&b, " %s=%v", a.Key, a.Value)
			return true
		}
		for _, a := range h.attrs {
			write(a)
		}
		r.Attrs(write)
		h.cfg.logf("%s", b.String())
		return nil
	}
	if fromPackage(r.PC, oauthPackage) {
		return nil
	}
	return h.next.Handle(ctx, r)
}

// WithAttrs implements [slog.Handler].
func (h *logHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &logHandler{cfg: h.cfg, next: h.next.WithAttrs(attrs), attrs: append(h.attrs[:len(h.attrs):len(h.attrs)], attrs...)}
}

// WithGroup implements [slog.Handler]. Groups only affect the default handler.
func (h *logHandler) WithGroup(name string) slog.Handler {
	return &logHandler{cfg: h.cfg, next: h.next.WithGroup(name), attrs: h.attrs}
}

// fromPackage reports whether the function at pc belongs to the package with
// the given import path prefix.
func fromPackage(pc uintptr, prefix string) bool {
	if pc == 0 {
		return false
	}
	frame, _ := runtime.CallersFrames([]uintptr{pc}).Next()
	return strings.HasPrefix(frame.Function, prefix)
}
