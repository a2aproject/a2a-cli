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

//go:build windows

package localsrv

import (
	"errors"
	"os"
	"os/exec"
	"sync"

	"golang.org/x/sys/windows"
)

// processTree puts the command into a job object so that the command and
// every process it spawns can be terminated together.
type processTree struct {
	cmd *exec.Cmd

	mu  sync.Mutex
	job windows.Handle
}

func (t *processTree) configure() {}

func (t *processTree) attach() error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	proc, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(t.cmd.Process.Pid))
	if err != nil {
		return errors.Join(err, windows.CloseHandle(job))
	}
	if err := windows.AssignProcessToJobObject(job, proc); err != nil {
		return errors.Join(err, windows.CloseHandle(proc), windows.CloseHandle(job))
	}
	if err := windows.CloseHandle(proc); err != nil {
		return errors.Join(err, windows.CloseHandle(job))
	}

	t.mu.Lock()
	t.job = job
	t.mu.Unlock()
	return nil
}

func (t *processTree) kill() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.job != 0 {
		return windows.TerminateJobObject(t.job, 1)
	}
	// Cancellation can arrive before the process is assigned to the job.
	if err := t.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

func (t *processTree) close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.job == 0 {
		return nil
	}
	err := windows.CloseHandle(t.job)
	t.job = 0
	return err
}
