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

//go:build !unix

package fsutil

import "os"

// MkdirPrivate creates dir with mode 0700 if needed.
func MkdirPrivate(dir string) error { return os.MkdirAll(dir, 0o700) }

// CheckPrivate checks that dir exists.
func CheckPrivate(dir string) error {
	_, err := os.Stat(dir)
	return err
}
