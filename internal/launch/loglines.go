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

package launch

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
)

// logLines writes each line written to it as a log record, so the lines stay
// in the server log without being relayed as progress.
type logLines struct {
	msg string

	mu  sync.Mutex
	buf []byte
}

func (w *logLines) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			return len(p), nil
		}
		w.log(string(w.buf[:i]))
		w.buf = w.buf[i+1:]
	}
}

func (w *logLines) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.log(string(w.buf))
	w.buf = nil
}

func (w *logLines) log(line string) {
	line = strings.TrimRight(line, "\r")
	if line == "" {
		return
	}
	slog.Info(w.msg, "output", line)
}
