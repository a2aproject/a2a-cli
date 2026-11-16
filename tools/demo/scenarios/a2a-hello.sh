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
# summary: examples/a2a-hello: build a command plugin, enable plugins, run a2a hello

prepare() {
  mkdir -p "$HOME/bin"
  ln -sfn "$REPO_ROOT" "$HOME/a2a-cli"
  # A fresh HOME: plugins start disabled, as they do for a new user.
  a2a plugin set-enabled false >/dev/null
}

layout() {
  panes B
  run B "cd ~/a2a-cli/examples/a2a-hello && clear" 30
}

scene() {
  say B "Build and install"
  run B "go build -o ~/bin/a2a-hello ." 300
  run B "a2a plugin set-enabled true"
  pause 1

  clear_pane B
  say B "Try it"
  run B "a2a plugin list"
  wait_text B 'hello +1\.0\.0' 5
  run B "a2a hello"
  run B "a2a hello Alice"
  wait_text B 'Hello, Alice!' 5
  pause 4
}
