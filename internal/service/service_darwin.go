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

//go:build darwin

package service

import (
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func label(name string) string { return "org.a2aproject." + name }

func plistPath(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", label(name)+".plist"), nil
}

func xmlEscape(s string) string {
	var sb strings.Builder
	if err := xml.EscapeText(&sb, []byte(s)); err != nil {
		return s
	}
	return sb.String()
}

// LaunchdPlist renders the launchd agent of u. It restarts the server only
// when it fails, so `a2a server stop` keeps it stopped.
func LaunchdPlist(u Unit) string {
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
`)
	fmt.Fprintf(&sb, "<key>Label</key><string>%s</string>\n<key>ProgramArguments</key><array>\n", xmlEscape(label(u.Name)))
	for _, a := range append([]string{u.Exe}, u.Args...) {
		fmt.Fprintf(&sb, "  <string>%s</string>\n", xmlEscape(a))
	}
	sb.WriteString("</array>\n")
	fmt.Fprintf(&sb, "<key>WorkingDirectory</key><string>%s</string>\n", xmlEscape(u.Dir))
	if len(u.Env) > 0 {
		sb.WriteString("<key>EnvironmentVariables</key><dict>\n")
		for _, kv := range u.Env {
			k, v, _ := strings.Cut(kv, "=")
			fmt.Fprintf(&sb, "  <key>%s</key><string>%s</string>\n", xmlEscape(k), xmlEscape(v))
		}
		sb.WriteString("</dict>\n")
	}
	fmt.Fprintf(&sb, "<key>StandardOutPath</key><string>%s</string>\n<key>StandardErrorPath</key><string>%s</string>\n", xmlEscape(u.LogPath), xmlEscape(u.LogPath))
	sb.WriteString("<key>RunAtLoad</key><true/>\n<key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>\n</dict></plist>\n")
	return sb.String()
}

func domain() string { return fmt.Sprintf("gui/%d", os.Getuid()) }

// Install writes and loads a launchd agent.
func Install(u Unit) (string, error) {
	path, err := plistPath(u.Name)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if Installed(u.Name) {
		if err := run("launchctl", "bootout", domain(), path); err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(path, []byte(LaunchdPlist(u)), 0o644); err != nil {
		return "", err
	}
	return "", run("launchctl", "bootstrap", domain(), path)
}

// Installed reports whether the service exists.
func Installed(name string) bool {
	path, err := plistPath(name)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// Uninstall unloads and removes the service.
func Uninstall(name string) error {
	path, err := plistPath(name)
	if err != nil {
		return err
	}
	stopErr := run("launchctl", "bootout", domain(), path)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Join(stopErr, err)
	}
	return stopErr
}
