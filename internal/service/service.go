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

// Package service runs an agent server in the background: detached from the
// terminal, or as a launchd or systemd user service that starts at login.
package service

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Unit describes a background agent server process.
type Unit struct {
	// Name identifies the service, such as a2a-coder-1a2b3c.
	Name        string
	Description string
	Exe         string
	Args        []string
	Dir         string
	LogPath     string
	// Env holds NAME=value pairs, such as PATH so that docker is found.
	Env []string
}

// ErrUnsupported is returned on platforms without a supported service manager.
var ErrUnsupported = errors.New("--service is supported on Linux (systemd) and macOS (launchd); use --detach instead")

// Detach starts the unit's process in its own session, with output appended
// to the log. The returned channel is closed when the process exits.
func Detach(u Unit) (<-chan struct{}, error) {
	if err := os.MkdirAll(filepath.Dir(u.LogPath), 0o700); err != nil {
		return nil, err
	}
	log, err := os.OpenFile(u.LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	devnull, err := os.Open(os.DevNull)
	if err != nil {
		return nil, errors.Join(err, log.Close())
	}
	cmd := exec.Command(u.Exe, u.Args...)
	cmd.Dir = u.Dir
	cmd.Env = append(os.Environ(), u.Env...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devnull, log, log
	setsid(cmd)
	startErr := cmd.Start()
	closeErr := errors.Join(log.Close(), devnull.Close())
	if startErr != nil {
		return nil, startErr
	}
	// Waiting reaps the process if it exits while this one runs, so a crash
	// isn't hidden by a zombie that still looks alive.
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		if err := cmd.Wait(); err != nil {
			slog.Debug("detached process exited", "error", err)
		}
	}()
	return exited, closeErr
}

func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
