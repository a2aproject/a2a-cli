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
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/a2aproject/a2a-cli/internal/agentdir"
	"github.com/a2aproject/a2a-cli/internal/service"
)

// maxLogSize is the size at which the server log is rotated on start.
const maxLogSize = 10 << 20

// Foreground runs the agent of dir until ctx ends. Progress goes to out;
// logs and the harness's stderr go to state/server.log, and also to stderr
// when verbose. ready, if set, is called once the agent serves.
func Foreground(ctx context.Context, dir agentdir.Dir, out io.Writer, verbose bool, ready func()) error {
	cfg, err := dir.LoadConfig()
	if err != nil {
		return err
	}
	secrets, err := dir.LoadSecrets()
	if err != nil {
		return err
	}
	if err := dir.Init(); err != nil {
		return err
	}
	logFile, err := openLog(dir.LogPath())
	if err != nil {
		return err
	}
	defer func() {
		if err := logFile.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "closing log: %v\n", err)
		}
	}()
	var logOut io.Writer = logFile
	level := slog.LevelInfo
	if verbose {
		logOut = io.MultiWriter(logFile, os.Stderr)
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(logOut, &slog.HandlerOptions{Level: level})))
	slog.Info("starting agent server", "name", cfg.Name, "harness", cfg.Harness, "dir", dir.Root)
	opts := Options{Dir: dir, Config: cfg, Secrets: secrets, Out: out, HarnessLog: logOut}
	if ready != nil {
		opts.Ready = func(*Server) { ready() }
	}
	err = Run(ctx, opts)
	if err != nil {
		slog.Error("agent server stopped", "error", err)
	}
	return err
}

func openLog(path string) (*os.File, error) {
	if info, err := os.Stat(path); err == nil && info.Size() > maxLogSize {
		if err := os.Rename(path, path+".1"); err != nil {
			return nil, err
		}
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
}

// ServiceName is the name of the agent's background service.
func ServiceName(dir agentdir.Dir, cfg *agentdir.Config) string {
	return "a2a-" + cfg.Name + "-" + dir.ID()
}

// Background starts the agent detached from the terminal, or as a user
// service when asService, and relays its progress to out until it serves.
func Background(ctx context.Context, dir agentdir.Dir, asService bool, out io.Writer) error {
	cfg, err := dir.LoadConfig()
	if err != nil {
		return err
	}
	if err := dir.Init(); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if _, err := NewClient(dir).Status(ctx); err == nil {
		return fmt.Errorf("the agent is already running (a2a server status); stop it first with `a2a server stop`")
	}
	var offset int64
	if info, err := os.Stat(dir.LogPath()); err == nil {
		offset = info.Size()
	}
	u := service.Unit{
		Name:        ServiceName(dir, cfg),
		Description: fmt.Sprintf("a2a server %s (%s)", cfg.Name, dir.Root),
		Exe:         exe,
		Args:        []string{"server", "--dir", dir.Root},
		Dir:         dir.Root,
		LogPath:     dir.LogPath(),
		Env:         []string{"PATH=" + os.Getenv("PATH")},
	}
	var note string
	alive := func() bool { return true }
	if asService {
		if note, err = service.Install(u); err != nil {
			return err
		}
	} else {
		pid, err := service.Detach(u)
		if err != nil {
			return err
		}
		alive = func() bool { return processAlive(pid) }
	}
	if err := waitReady(ctx, dir, offset, alive, out); err != nil {
		return err
	}
	msg := "✓ running in the background (a2a server logs, a2a server stop)\n"
	if asService {
		msg = "✓ running as a user service (a2a server logs)\n"
		if note != "" {
			msg += "  " + note + "\n"
		}
	}
	_, err = io.WriteString(out, msg)
	return err
}

// waitReady relays progress lines from the log until the control socket
// answers or the process dies.
func waitReady(ctx context.Context, dir agentdir.Dir, offset int64, alive func() bool, out io.Writer) error {
	deadline := time.Now().Add(15 * time.Minute)
	client := NewClient(dir)
	var tail []string
	for {
		lines, next, err := readFrom(dir.LogPath(), offset)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		offset = next
		for _, l := range lines {
			if isProgress(l) {
				if _, err := fmt.Fprintln(out, l); err != nil {
					return err
				}
			}
			tail = append(tail, l)
			if len(tail) > 20 {
				tail = tail[1:]
			}
		}
		sctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		_, statusErr := client.Status(sctx)
		cancel()
		if statusErr == nil {
			// Print what the server logged while becoming ready.
			lines, _, err := readFrom(dir.LogPath(), offset)
			if err != nil {
				return nil
			}
			for _, l := range lines {
				if !isProgress(l) {
					continue
				}
				if _, err := fmt.Fprintln(out, l); err != nil {
					return err
				}
			}
			return nil
		}
		if !alive() {
			return fmt.Errorf("the agent server exited:\n%s", strings.Join(tail, "\n"))
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the agent server did not start in time; see %s", dir.LogPath())
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
}

// isProgress reports whether a log line is a progress line for the user
// rather than a structured log record.
func isProgress(line string) bool {
	return line != "" && !strings.HasPrefix(line, "time=")
}

func readFrom(path string, offset int64) ([]string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, offset, err
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Debug("closing log", "error", err)
		}
	}()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, offset, err
	}
	var lines []string
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			// A partial line is read again once it is complete.
			return lines, offset, nil
		}
		offset += int64(len(line))
		lines = append(lines, strings.TrimRight(line, "\r\n"))
	}
}

// FollowLog prints the last n lines of the log, then new lines until ctx
// ends if follow is set.
func FollowLog(ctx context.Context, dir agentdir.Dir, n int, follow bool, out io.Writer) error {
	data, err := os.ReadFile(dir.LogPath())
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("no log yet at %s", dir.LogPath())
	}
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	if err := printLines(out, lines); err != nil {
		return err
	}
	if !follow {
		return nil
	}
	offset := int64(len(data))
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(500 * time.Millisecond):
		}
		lines, next, err := readFrom(dir.LogPath(), offset)
		if err != nil {
			return err
		}
		if next < offset {
			next = 0
		}
		offset = next
		if err := printLines(out, lines); err != nil {
			return err
		}
	}
}

func printLines(out io.Writer, lines []string) error {
	for _, l := range lines {
		if _, err := fmt.Fprintln(out, l); err != nil {
			return err
		}
	}
	return nil
}
