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

package oauthstore

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

const (
	renameAttempts = 20
	renameWait     = 25 * time.Millisecond
)

// renameFile replaces to with from. Windows refuses to replace a file another
// process has open, such as an a2a process reading grants.json without the
// lock, which readers don't take. Those reads are short, so the rename is
// retried for up to half a second instead of losing the write, which may hold a
// refresh token the server has just rotated.
func renameFile(from, to string) error {
	return retryRename(os.Rename, isSharingError, from, to, renameAttempts, renameWait)
}

func isSharingError(err error) bool {
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_SHARING_VIOLATION)
}
