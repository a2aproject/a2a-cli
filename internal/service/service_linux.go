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

//go:build linux

package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func unitPath(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "systemd", "user", name+".service"), nil
}

// SystemdUnit renders the systemd user unit of u. systemd expands
// %-specifiers in every value and $VARIABLES in ExecStart, so both are
// escaped; a newline would start a new directive and is rejected.
func SystemdUnit(u Unit) (string, error) {
	values := append([]string{u.Description, u.Exe, u.Dir, u.LogPath}, u.Args...)
	for _, v := range append(values, u.Env...) {
		if strings.ContainsAny(v, "\r\n") {
			return "", fmt.Errorf("systemd unit value %q contains a newline", v)
		}
	}
	spec := strings.NewReplacer("%", "%%").Replace
	exec := strings.NewReplacer("%", "%%", "$", "$$").Replace
	args := append([]string{u.Exe}, u.Args...)
	for i, a := range args {
		args[i] = exec(a)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "[Unit]\nDescription=%s\nAfter=network-online.target\n\n[Service]\n", spec(u.Description))
	fmt.Fprintf(&sb, "ExecStart=%s\n", quoteArgs(args))
	fmt.Fprintf(&sb, "WorkingDirectory=%s\n", spec(u.Dir))
	for _, kv := range u.Env {
		fmt.Fprintf(&sb, "Environment=%s\n", quoteArgs([]string{spec(kv)}))
	}
	fmt.Fprintf(&sb, "StandardOutput=append:%s\nStandardError=append:%s\n", spec(u.LogPath), spec(u.LogPath))
	sb.WriteString("Restart=on-failure\nRestartSec=5\n\n[Install]\nWantedBy=default.target\n")
	return sb.String(), nil
}

func quoteArgs(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsAny(a, " \t\"'\\") {
			a = `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(a) + `"`
		}
		out[i] = a
	}
	return strings.Join(out, " ")
}

// Install writes and starts a systemd user service. It returns a note for
// the user, such as how to keep it running after logout.
func Install(u Unit) (string, error) {
	path, err := unitPath(u.Name)
	if err != nil {
		return "", err
	}
	unit, err := SystemdUnit(u)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(unit), 0o644); err != nil {
		return "", err
	}
	if err := run("systemctl", "--user", "daemon-reload"); err != nil {
		return "", err
	}
	if err := run("systemctl", "--user", "enable", u.Name+".service"); err != nil {
		return "", err
	}
	// restart starts a stopped service and picks up a changed unit.
	if err := run("systemctl", "--user", "restart", u.Name+".service"); err != nil {
		return "", err
	}
	return "to keep it running after you log out: loginctl enable-linger " + os.Getenv("USER"), nil
}

// Installed reports whether the service exists.
func Installed(name string) bool {
	path, err := unitPath(name)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// Uninstall stops and removes the service.
func Uninstall(name string) error {
	path, err := unitPath(name)
	if err != nil {
		return err
	}
	stopErr := run("systemctl", "--user", "disable", "--now", name+".service")
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Join(stopErr, err)
	}
	return errors.Join(stopErr, run("systemctl", "--user", "daemon-reload"))
}
