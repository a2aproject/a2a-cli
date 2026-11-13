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
# summary: lessons/03-messages-and-tasks: multi-part messages, --async, task get/subscribe/cancel/list

# answer_with_data answers a prompt sent with a data part. The scripted agent
# reads the prompt's text and the data part's JSON as one string.
answer_with_data() {
  answer_for "$1" "$3"
  cp "$DEMO_WORK/fake/answer-$DEMO_ANSWERS.txt" "$DEMO_WORK/fake/answer-$DEMO_ANSWERS.txt"$'\n'"$2"
}

fakes() {
  answer_with_data "Check this order against the notes, and say whether it can ship." \
    '{"destination":"Lisbon","id":1042,"items":3}' \
    "Yes: it has 3 items, so it ships from the Berlin warehouse to Lisbon."
  answer_with_data "How many items are in this order?" '{"id":1042,"items":3}' "Three."
  behave_as "Count to twenty, slowly." "slow 4s"
  behave_as "Count to ten, slowly." "slow 3s"
  behave_as "Count to a million, slowly." "stall"
}

prepare() {
  lesson_agent
}

layout() {
  panes B
  run B "cd ~/a2a-lessons && clear" 30
}

scene() {
  run B 'echo "Orders of more than 2 items ship from the Berlin warehouse." > notes.txt'
  run B "echo '{\"id\": 1042, \"items\": 3, \"destination\": \"Lisbon\"}' > order.json"
  clear_pane B
  say B "Step 1: a message with parts"
  run B 'a2a send \
  --text-part "Check this order against the notes, and say whether it can ship." \
  --file-part notes.txt \
  --data-part order.json'
  expect_status B completed
  pause 3
  run B "a2a send --data-part '{\"id\": 1042, \"items\": 3}' \"How many items are in this order?\""
  expect_status B completed
  pause 3

  clear_pane B
  say B "Step 2: start a task and come back later"
  run B 'a2a send --async "Count to twenty, slowly."'
  local task
  task="$(last_match B 'Task: +[^ ]+')"
  run B "TASK=${task##* }"
  run B 'a2a task get "$TASK"'
  run B 'a2a task get "$TASK" --wait --timeout 5m'
  expect_status B completed
  pause 3

  clear_pane B
  say B "Step 3: follow a running task"
  run B 'TASK="$(a2a send --async -o json "Count to ten, slowly." | jq -r .task.id)"'
  run B 'a2a task subscribe "$TASK"'
  wait_text B '\[status\] completed' 10
  pause 3

  clear_pane B
  say B "Step 4: cancel a task"
  run B 'TASK="$(a2a send --async -o json "Count to a million, slowly." | jq -r .task.id)"'
  run B 'a2a task cancel "$TASK"'
  expect_status B canceled
  pause 3

  clear_pane B
  say B "Step 5: list your tasks"
  run B "a2a task list"
  run B "a2a task list --status completed --limit 5"
  pause 4
}
