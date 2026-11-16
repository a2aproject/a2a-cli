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
# summary: lessons/01-first-message: start an agent, read its card, send, stream, save it in .env

fakes() {
  answer_for "In one sentence, what can you do?" "I can read, write and run code in this workspace."
  behave_as "List the files in your workspace." "tool"
  answer_for "Thanks!" "Glad to help."
}

layout() {
  panes B
  split_pane A B above 25
  title A "terminal A: agent"
  title B "terminal B"
}

scene() {
  say A "Step 1: start an agent"
  run A "mkdir -p ~/a2a-lessons/agent && cd ~/a2a-lessons/agent"
  start A "a2a-serve start opencode --name lesson-agent --model openrouter/~google/gemini-flash-latest --secret OPENROUTER_API_KEY"
  wait_text A 'local +http'
  pause 2

  say B "Step 2: read the agent card"
  run B "mkdir -p ~/a2a-lessons && cd ~/a2a-lessons"
  run B 'AGENT="$(a2a-serve status --dir agent --url=local)"'
  run B 'a2a card get "$AGENT"'
  pause 3

  clear_pane B
  say B "Step 3: send a message"
  run B 'a2a send -a "$AGENT" "In one sentence, what can you do?"'
  expect_status B completed
  pause 3

  clear_pane B
  say B "Step 4: stream the reply"
  run B 'a2a send -a "$AGENT" --stream "List the files in your workspace."'
  wait_text B '\[status\] completed' 10
  pause 3

  clear_pane B
  say B "Step 5: save the agent in .env"
  run B 'echo "A2ACLI_AGENT_CARD=$AGENT" > .env'
  run B 'a2a send "Thanks!"'
  expect_status B completed
  pause 3
}
