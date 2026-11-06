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

//go:build !(darwin || dragonfly || freebsd || linux || netbsd || openbsd || windows)

package oauthstore

import "os"

// tryLockFile is a no-op where no advisory file lock is available: the store is
// then not protected against concurrent CLI processes.
func tryLockFile(*os.File) (bool, error) { return true, nil }

func unlockFile(*os.File) error { return nil }
