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

package harness

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"io"
	"os/exec"
	"runtime/debug"
	"strings"
)

//go:embed images/*/Dockerfile
var dockerfiles embed.FS

// Dockerfile returns the embedded Dockerfile of a harness image.
func Dockerfile(harness string) ([]byte, bool) {
	data, err := dockerfiles.ReadFile("images/" + harness + "/Dockerfile")
	return data, err == nil
}

// LocalImage is the tag of a harness image built on this machine for the
// a2a CLI version cliVersion.
func LocalImage(harness, cliVersion string) string {
	tag := "local"
	if cliVersion != "latest" {
		tag = strings.NewReplacer("+", "-", "/", "-").Replace(cliVersion)
	}
	return "a2a-harness-" + harness + ":" + tag
}

// CLIVersion is the a2a CLI version harness images are built with: the commit
// this binary was built from, the module version when it was installed with
// go install, or "latest" when neither is known.
// TODO: pin to a release once harness images are published per release.
func CLIVersion() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "latest"
	}
	return cliVersion(bi)
}

func cliVersion(bi *debug.BuildInfo) string {
	for _, s := range bi.Settings {
		if s.Key == "vcs.revision" && s.Value != "" {
			return s.Value
		}
	}
	if v := bi.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	return "latest"
}

// EnsureImage makes image available to docker: it uses a local copy, pulls
// it, or builds the harness's embedded Dockerfile when the pull fails. It
// returns the image to run and a line describing what it did.
func EnsureImage(ctx context.Context, image, harness string, progress io.Writer) (string, string, error) {
	if image == "" {
		return "", "", fmt.Errorf("harness %q has no container image; pass --image or --sandbox local", harness)
	}
	if imageExists(ctx, image) {
		return image, "using " + image, nil
	}
	pull := exec.CommandContext(ctx, "docker", "pull", image)
	pull.Stdout, pull.Stderr = progress, progress
	pullErr := pull.Run()
	if pullErr == nil {
		return image, "pulled " + image, nil
	}
	df, ok := Dockerfile(harness)
	if !ok {
		return "", "", fmt.Errorf("docker pull %s: %w", image, pullErr)
	}
	version := CLIVersion()
	local := LocalImage(harness, version)
	if imageExists(ctx, local) {
		return local, "using " + local, nil
	}
	build := exec.CommandContext(ctx, "docker", "build", "-t", local, "--build-arg", "A2A_CLI_VERSION="+version, "-")
	build.Stdin = bytes.NewReader(df)
	build.Stdout, build.Stderr = progress, progress
	if err := build.Run(); err != nil {
		return "", "", fmt.Errorf("docker pull %s failed (%v), and building it locally failed: %w", image, pullErr, err)
	}
	return local, "built " + local + " (" + strings.TrimPrefix(image, ImageRepo) + " is not published)", nil
}

func imageExists(ctx context.Context, image string) bool {
	return exec.CommandContext(ctx, "docker", "image", "inspect", image).Run() == nil
}
