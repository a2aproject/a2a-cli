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
#
# summary: examples/credential-helper: a token-file helper, checked with auth get

prepare() {
  mkdir -p "$HOME/credential-helper"
  cp "$REPO_ROOT/examples/credential-helper/token-file-helper" "$HOME/credential-helper/"
}

layout() {
  panes B
  run B "cd ~/credential-helper && clear" 30
}

scene() {
  say B "Write a token file"
  run B "mkdir -p ~/.config/a2a-cli"
  run B "printf 'agent.example.com s3cr3t-1\nreview.example.org s3cr3t-2\n' > ~/.config/a2a-cli/tokens"
  run B "chmod 600 ~/.config/a2a-cli/tokens"
  pause 1

  clear_pane B
  say B "Ask a2a what it would send"
  run B 'a2a auth get https://agent.example.com --credential-helper "$PWD/token-file-helper" --reveal'
  wait_text B 'Bearer s3cr3t-1' 5
  run B 'a2a auth get https://other.example.net --credential-helper "$PWD/token-file-helper"'
  wait_text B 'no headers' 5
  pause 2

  clear_pane B
  say B "Scope it"
  run B 'a2a auth get https://review.example.org --credential-helper "agent.example.com=$PWD/token-file-helper"'
  wait_text B 'no credential helper for' 5
  pause 2

  clear_pane B
  say B "How it works"
  run B "echo '{\"uri\": \"https://agent.example.com\"}' | ./token-file-helper get"
  pause 4
}
