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
# summary: lessons/05-credentials: the credential helper, auth get, a wrong token

prepare() {
  lesson_agent
}

layout() {
  panes B
  run B "cd ~/a2a-lessons && clear" 30
}

scene() {
  say B "Step 1: find the credential helper"
  run B "a2a config show | grep credential-helper"
  run B "cat ~/.config/a2a-cli/config.yaml"
  pause 3

  clear_pane B
  say B "Step 2: look at the headers"
  run B 'a2a auth get "$(a2a-serve status --dir agent --url=local)"'
  wait_text B 'Authorization +<redacted>' 5
  pause 3

  clear_pane B
  say B "Step 3: send a wrong token"
  run B 'a2a send --auth "Bearer not-a-real-token" "hello"'
  wait_text B "doesn't recognize the token" 5
  run B 'echo "exit code $?"'
  run B 'a2a card get --auth "Bearer not-a-real-token"'
  pause 4
}
