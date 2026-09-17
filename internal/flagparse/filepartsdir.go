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

package flagparse

import (
	"os"

	"github.com/a2aproject/a2a-cli/internal/clierr"
	"github.com/spf13/pflag"
)

// AttachFilePartsOutDir registers the --save-fileparts flag.
func AttachFilePartsOutDir(f *pflag.FlagSet) {
	_ = f.String("save-fileparts", "", "A directory for storing raw-bytes message and artifact parts as files.")
}

// LookupFilePartsOutDir returns a value of the flag registered using [AttachFilePartsOutDir].
// The directory is not created here; the caller creates it lazily when the first
// file part is written.
func LookupFilePartsOutDir(fs *pflag.FlagSet) (string, error) {
	f := fs.Lookup("save-fileparts")
	if f == nil || f.Value.String() == "" {
		return "", nil
	}
	p := f.Value.String()
	if s, err := os.Stat(p); err == nil && !s.IsDir() {
		return "", clierr.Usage("--save-fileparts must point to a directory")
	}
	return p, nil
}
