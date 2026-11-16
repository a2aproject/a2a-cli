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
# summary: examples/opencode-contacts: opencode delegates to a contact (needs Docker and GEMINI_API_KEY; not in CI)

prepare() {
  [[ -n "${GEMINI_API_KEY:-}" ]] || die "set GEMINI_API_KEY, for example from gcloud services api-keys get-key-string"
  [[ -n "$DEMO_FAKE" ]] && log "opencode needs a real model; --fake only affects a2a-serve agents"
  ln -sfn "$REPO_ROOT" "$HOME/a2a-cli"
  export OPENCODE_VOLUME="a2a-demo-opencode-$$"
  # Build the container's a2a and pull the image off camera.
  (cd "$HOME/a2a-cli/examples/opencode-contacts" && ./run-opencode --version >/dev/null)
}

teardown() {
  docker volume rm -f "$OPENCODE_VOLUME-data" "$OPENCODE_VOLUME-cache" >/dev/null 2>&1 || true
}

layout() {
  panes B
  split_pane A B above 15
  title A "terminal A: agent"
  title B "terminal B"
  run A "cd ~/a2a-cli/examples/opencode-contacts && clear" 30
  run B "cd ~/a2a-cli/examples/opencode-contacts && clear" 30
}

scene() {
  start A "./start-agent.sh"
  until_ok 30 curl -fsS http://127.0.0.1:9201/.well-known/agent-card.json
  pause 1

  say B "Add the contact"
  run B 'a2a contact add release-notes localhost:9201 --note "Use for any release notes or changelog text"'
  wait_text B 'Added contact release-notes' 5
  pause 2

  clear_pane B
  say B "Ask opencode"
  run B './run-opencode run "Which A2A agents can you delegate to?"' 300
  wait_text B 'release-notes' 5
  pause 4

  clear_pane B
  run B "cat commits.txt"
  run B './run-opencode run "Write release notes for the commits in commits.txt."' 300
  wait_text B 'a2a send -a release-notes' 5
  wait_text B 'Save trusted agents under a short name' 5
  pause 5
}
