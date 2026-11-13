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
# summary: docs/assets/demo.gif: the home page demo; card, send, stream and a conversation

fakes() {
  answer_for "What can you do?" "I'm a coding agent: I read, write and run code in this repository."
  answer_for "Find the slowest test, and tell me why it is slow." "TestStream takes 0.4s: it waits for real SSE keep-alives instead of a fake clock."
}

prepare() {
  mkdir -p "$HOME/my-project"
}

layout() {
  panes B
  split_pane A B above 22
  title A "agent"
  title B "a2a"
}

scene() {
  run A "cd ~/my-project"
  start A "a2a-serve start claude --name my-project --secret ANTHROPIC_API_KEY"
  wait_text A 'local +http'
  pause 1

  run B 'AGENT="$(a2a-serve status --dir my-project --url=local)"'
  run B 'a2a card get "$AGENT"'
  pause 3

  clear_pane B
  run B 'a2a send -a "$AGENT" "What can you do?"'
  expect_status B completed
  pause 3

  clear_pane B
  run B 'a2a send -a "$AGENT" --stream "Find the slowest test, and tell me why it is slow."'
  wait_text B '\[status\] completed' 10
  pause 4
}
