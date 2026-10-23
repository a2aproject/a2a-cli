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

//go:build unix

package fsutil

import (
	"fmt"
	"os"
	"syscall"
)

// MkdirPrivate creates dir with mode 0700 if needed and checks that it is a
// real directory owned by the current user and closed to others. A directory
// in a shared location such as /tmp may have been created by someone else
// to intercept the sockets placed in it.
func MkdirPrivate(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return CheckPrivate(dir)
}

// CheckPrivate checks that dir is a real directory owned by the current user
// and closed to others.
func CheckPrivate(dir string) error {
	fi, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("can't check the owner of %s", dir)
	}
	if int(st.Uid) != os.Getuid() {
		return fmt.Errorf("%s is owned by another user (uid %d); remove it or set XDG_RUNTIME_DIR", dir, st.Uid)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s is accessible to other users (mode %v); run chmod 700 %s", dir, fi.Mode().Perm(), dir)
	}
	return nil
}
