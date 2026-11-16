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
# summary: examples/a2a-transport-echo: build a transport plugin, list it, send through it

prepare() {
  mkdir -p "$HOME/bin"
  ln -sfn "$REPO_ROOT" "$HOME/a2a-cli"
}

layout() {
  panes B
  run B "cd ~/a2a-cli/examples/a2a-transport-echo && clear" 30
}

scene() {
  say B "Build and install"
  run B "go build -o ~/bin/a2a-transport-echo ." 300
  pause 1

  clear_pane B
  say B "Try it"
  run B "a2a transport list"
  wait_text B 'echo +1\.0\.0' 5
  run B 'a2a send --transport echo --endpoint echo://demo "hello there"'
  expect_status B completed
  pause 2
  clear_pane B
  run B 'a2a send --transport echo --endpoint echo://demo --stream "stream me"'
  wait_text B '\[status\] completed' 10
  pause 4
}
