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
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/pflag"

	"github.com/a2aproject/a2a-go/v2/a2aclient"
)

func TestServiceParamsParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		args     []string
		want     a2aclient.ServiceParams
		wantAuth string
		wantErr  bool
	}{
		{
			name: "single svc-param",
			args: []string{"--svc-param", "X-Trace=abc"},
			want: a2aclient.ServiceParams{"x-trace": {"abc"}},
		},
		{
			name: "value may contain equals",
			args: []string{"--svc-param", "k=a=b"},
			want: a2aclient.ServiceParams{"k": {"a=b"}},
		},
		{
			name: "repeated keys append in order",
			args: []string{"--svc-param", "k=1", "--svc-param", "k=2"},
			want: a2aclient.ServiceParams{"k": {"1", "2"}},
		},
		{
			name:     "auth maps to Authorization",
			args:     []string{"--auth", "Bearer tok"},
			want:     a2aclient.ServiceParams{"authorization": {"Bearer tok"}},
			wantAuth: "Bearer tok",
		},
		{
			name:     "svc-param and auth merge together",
			args:     []string{"--svc-param", "X-Trace=abc", "--auth", "Bearer tok"},
			want:     a2aclient.ServiceParams{"x-trace": {"abc"}, "authorization": {"Bearer tok"}},
			wantAuth: "Bearer tok",
		},
		{
			name:    "missing separator is an error",
			args:    []string{"--svc-param", "nokey"},
			wantErr: true,
		},
		{
			name:    "empty key is an error",
			args:    []string{"--svc-param", "=value"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var sp ServiceParams
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			sp.Attach(fs)

			err := fs.Parse(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("fs.Parse(%v) error = nil, want error", tt.args)
				}
				return
			}
			if err != nil {
				t.Fatalf("fs.Parse(%v) error = %v, want nil", tt.args, err)
			}
			if diff := cmp.Diff(tt.want, sp.Params()); diff != "" {
				t.Fatalf("sp.Params() wrong result (-want +got) diff = %s", diff)
			}
			if sp.Auth() != tt.wantAuth {
				t.Fatalf("sp.Auth() = %q, want %q", sp.Auth(), tt.wantAuth)
			}
		})
	}
}

func TestServiceParamsIsIdempotent(t *testing.T) {
	t.Parallel()

	var sp ServiceParams
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	sp.Attach(fs)
	if err := fs.Parse([]string{"--svc-param", "k=v", "--auth", "tok"}); err != nil {
		t.Fatalf("fs.Parse() error = %v, want nil", err)
	}

	first := sp.Params()
	first.Append("k", "leaked")

	want := a2aclient.ServiceParams{"k": {"v"}, "authorization": {"tok"}}
	if diff := cmp.Diff(want, sp.Params()); diff != "" {
		t.Fatalf("sp.Params() wrong result after mutating a prior result (-want +got) diff = %s", diff)
	}
}

func TestServiceParamsEmpty(t *testing.T) {
	t.Parallel()

	var sp ServiceParams
	if sp.Params() != nil {
		t.Fatalf("sp.Params() = %v, want nil", sp.Params())
	}
	if sp.Auth() != "" {
		t.Fatalf("sp.Auth() = %q, want empty", sp.Auth())
	}
}
