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
# summary: lessons/02-configuration: one setting from a flag, a variable, .env and config.yaml

prepare() {
  lesson_agent
}

layout() {
  panes B
  run B "cd ~/a2a-lessons && clear" 30
}

scene() {
  say B "Step 1: see the current configuration"
  run B "a2a config show"
  pause 4

  clear_pane B
  say B "Step 2: one setting, four places"
  run B 'echo "timeout: 45s" >> ~/.config/a2a-cli/config.yaml'
  run B "a2a config show | grep timeout"
  wait_text B 'timeout +45s +user-file' 5
  run B 'echo "A2ACLI_TIMEOUT=1m" >> .env'
  run B "a2a config show | grep timeout"
  wait_text B 'timeout +1m +local-file' 5
  run B "export A2ACLI_TIMEOUT=2m"
  run B "a2a config show | grep timeout"
  wait_text B 'timeout +2m +env' 5
  run B "a2a config show --timeout 3m | grep timeout"
  wait_text B 'timeout +3m0s +flag' 5
  pause 3
  run B "unset A2ACLI_TIMEOUT"
  run B "sed -i.bak '/^timeout:/d' ~/.config/a2a-cli/config.yaml"
  run B "sed -i.bak '/^A2ACLI_TIMEOUT=/d' .env"
  run B "rm ~/.config/a2a-cli/config.yaml.bak .env.bak"

  clear_pane B
  say B "Step 3: pick a file with --config"
  run B "cat > verbose.yaml <<'EOF'
verbose: true
output: json
EOF"
  paste_run B "a2a --config verbose.yaml config show | jq -r '.[] | select(.source == \"local-file\") | \"\\(.name)=\\(.value) from \\(.path)\"'"
  wait_text B 'output=json from verbose.yaml' 5
  pause 3

  clear_pane B
  say B "Step 4: settings a project can't set"
  run B 'echo "A2ACLI_CA_CERT=/tmp/evil.pem" >> .env'
  run B "a2a config show | grep ca-cert"
  wait_text B 'warning: ignoring ca-cert' 5
  run B "sed -i.bak '/^A2ACLI_CA_CERT=/d' .env && rm .env.bak"
  pause 3
}
