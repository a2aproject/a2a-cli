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
# summary: lessons/04-conversations: --context-id, answering input-required with --task-id, chat

fakes() {
  behave_as "Remember this: the release is called Juniper." "remember Juniper"
  behave_as "What is the release called?" "recall"
  behave_as "Run whoami and tell me the user name." "whoami"
  behave_as "Remember this: my favorite color is teal." "remember teal"
  behave_as "What is my favorite color?" "recall"
}

prepare() {
  lesson_agent
}

layout() {
  panes B
  run B "cd ~/a2a-lessons && clear" 30
}

# chat_line types a line at the chat prompt and waits for the turn to end.
# The pane is cleared first, so a completed status from an earlier turn
# doesn't count.
chat_line() {
  clear_pane B
  type_keys B "$1"
  sleep 0.3
  t send-keys -t "$(pane_id B)" Enter
  wait_text B '\[status\] completed' 30
  wait_text B '^> $' 10
  pause 3
}

scene() {
  say B "Step 1: tasks and contexts"
  run B 'a2a send "Remember this: the release is called Juniper."'
  expect_status B completed
  local context
  context="$(last_match B 'Context: +[^ ]+')"
  run B 'a2a send "What is the release called?"'
  pause 3

  clear_pane B
  say B "Step 2: continue the conversation"
  run B "a2a send --context-id ${context##* } \"What is the release called?\""
  expect_status B completed
  pause 3

  clear_pane B
  say B "Step 3: answer a question"
  run B 'a2a send "Run whoami and tell me the user name."'
  expect_status B input-required
  pause 3
  local task
  task="$(last_match B 'a2a send --task-id [^ ]+')"
  run B "a2a send --task-id ${task##* } once"
  expect_status B completed
  pause 3

  clear_pane B
  say B "Step 4: chat"
  start B "a2a chat --stream"
  wait_text B '^> ' 30
  pause 1
  chat_line "Remember this: my favorite color is teal."
  chat_line "What is my favorite color?"
  type_keys B "/exit"
  sleep 0.3
  t send-keys -t "$(pane_id B)" Enter
  wait_text B 'contextId:' 10
  pause 3
}
