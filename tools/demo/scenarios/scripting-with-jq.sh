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
# summary: examples/scripting-with-jq: fan questions out with --async, collect them with task get --wait

fakes() {
  answer_for "Which Go version does this project use?" "Go 1.26, from go.mod."
  answer_for "Name the largest file in the workspace." "go.sum, at 31 KB."
  answer_for "Is there a test that takes longer than a second?" "No; the slowest, TestStream, takes 0.4s."
}

prepare() {
  lesson_agent
  mkdir -p "$HOME/scripting-with-jq"
  cp "$REPO_ROOT/examples/scripting-with-jq/ask-all.sh" "$REPO_ROOT/examples/scripting-with-jq/questions.txt" "$HOME/scripting-with-jq/"
}

layout() {
  panes B
  run B "cd ~/scripting-with-jq && clear" 30
}

scene() {
  run B 'export A2ACLI_AGENT_CARD="$(a2a-serve status --dir ~/a2a-lessons/agent --url=local)"'
  run B "cat questions.txt"
  pause 2
  run B "./ask-all.sh questions.txt"
  wait_text B 'completed +Is there a test' 10
  run B 'echo "exit code $?"'
  pause 4
}
