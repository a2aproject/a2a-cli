#!/usr/bin/env bash
# Copyright 2026 The A2A Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Assembles the a2a-cli Agent Plugin into <out-dir> from agent-plugin/ and the
# skills under skills/. The output directory is replaced, so files removed here
# are removed from the published plugin too.
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: $0 <out-dir> <version>" >&2
  exit 2
fi

out=$1
version=$2
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

rm -rf "$out"
mkdir -p "$out/skills"

for skill_md in "$root"/skills/*/SKILL.md; do
  skill_dir=$(dirname "$skill_md")
  name=$(basename "$skill_dir")
  cp -R "$skill_dir" "$out/skills/$name"
  # The skill README documents standalone installation, which the plugin replaces.
  rm -f "$out/skills/$name/README.md"
done

cp -R "$root/agent-plugin/hooks" "$out/hooks"
cp "$root/agent-plugin/README.md" "$out/README.md"
cp "$root/LICENSE" "$out/LICENSE"
jq --arg v "$version" '{"$schema": ."$schema", name: .name, version: $v} + .' \
  "$root/agent-plugin/plugin.json" > "$out/plugin.json"
