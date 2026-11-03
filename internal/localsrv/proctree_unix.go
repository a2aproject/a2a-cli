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

//go:build !windows

package localsrv

import (
	"errors"
	"os/exec"
	"syscall"
)

// processTree runs the command in its own process group so that the command
// and every process it spawns can be killed together.
type processTree struct {
	cmd *exec.Cmd
}

func (t *processTree) configure() {
	t.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func (t *processTree) attach() error {
	return nil
}

func (t *processTree) kill() error {
	err := syscall.Kill(-t.cmd.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

func (t *processTree) close() error {
	return nil
}
