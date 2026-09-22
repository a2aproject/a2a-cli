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

package clicfg

import (
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestStoreEnviron(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		base   []string
		local  map[string]string
		global map[string]string
		want   []string
	}{
		{
			name:   "appends file-only keys sorted after base",
			base:   []string{"PATH=/bin"},
			global: map[string]string{"GLOBAL_ONLY": "g"},
			local:  map[string]string{"LOCAL_ONLY": "l"},
			want:   []string{"PATH=/bin", "GLOBAL_ONLY=g", "LOCAL_ONLY=l"},
		},
		{
			name:   "base takes precedence over files",
			base:   []string{"A2ACLI_TENANT=from-env"},
			local:  map[string]string{"A2ACLI_TENANT": "from-local"},
			global: map[string]string{"A2ACLI_TENANT": "from-global"},
			want:   []string{"A2ACLI_TENANT=from-env"},
		},
		{
			name:   "local file takes precedence over global",
			base:   []string{},
			local:  map[string]string{"SLIM_TOKEN": "local"},
			global: map[string]string{"SLIM_TOKEN": "global"},
			want:   []string{"SLIM_TOKEN=local"},
		},
		{
			name: "no files leaves base unchanged",
			base: []string{"A=1", "B=2"},
			want: []string{"A=1", "B=2"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			opts := LoadOpts{
				WorkingDir: dir,
				GlobalPath: filepath.Join(dir, "missing-global.env"),
				LookupEnv:  makeEnv(nil),
			}
			if tc.global != nil {
				opts.GlobalPath = filepath.Join(dir, ".env.global")
				writeDotenv(t, opts.GlobalPath, tc.global)
			}
			if tc.local != nil {
				writeDotenv(t, filepath.Join(dir, ".env"), tc.local)
			}

			store, err := Load(opts)
			if err != nil {
				t.Fatalf("Load() error = %v, want nil", err)
			}

			got := store.Environ(tc.base)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("Store.Environ() wrong result (-want +got) diff = %s", diff)
			}
		})
	}
}
