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
# summary: examples/agent-discovery: opencode discovers a site's agent and delegates after asking (needs Docker and GEMINI_API_KEY; not in CI)

# Long replies: keep the pauses after them, so they can be read.
DEMO_GIF_SPEED=1.25
DEMO_GIF_IDLE=7

prepare() {
  [[ -n "${GEMINI_API_KEY:-}" ]] || die "set GEMINI_API_KEY, for example from gcloud services api-keys get-key-string"
  ln -sfn "$REPO_ROOT" "$HOME/a2a-cli"
  export OPENCODE_VOLUME="a2a-demo-opencode-$$"
  (cd "$HOME/a2a-cli/examples/agent-discovery" && go build -o /dev/null ./site && ../opencode-contacts/run-opencode --version >/dev/null)
}

teardown() {
  docker volume rm -f "$OPENCODE_VOLUME-data" "$OPENCODE_VOLUME-cache" >/dev/null 2>&1 || true
}

layout() {
  panes B
  split_pane A B above 22
  title A "terminal A: docs.acme.test"
  title B "terminal B"
  run A "cd ~/a2a-cli/examples/agent-discovery && clear" 30
  run B "cd ~/a2a-cli/examples/agent-discovery && clear" 30
}

scene() {
  start A "go run ./site --addr 127.0.0.1:8080 --url http://docs.acme.test:8080"
  wait_text A 'serving http://docs.acme.test:8080' 120
  pause 1

  say B "Point a name at the site, for opencode's container"
  run B 'export OPENCODE_DOCKER_ARGS="--add-host docs.acme.test:127.0.0.1"'
  pause 1

  clear_pane B
  say B "Ask opencode"
  run B '../opencode-contacts/run-opencode run "Read http://docs.acme.test:8080/ and tell me how my client should handle 429s from the Acme Payments API."' 300
  wait_text A 'agent card' 5
  wait_text B 'Acme Docs Assistant' 5
  pause 8

  clear_pane B
  run B '../opencode-contacts/run-opencode run --continue "Yes, ask it."' 300
  wait_text B 'a2a send -a http://docs.acme.test:8080' 5
  wait_text A 'A2A request' 5
  wait_text B 'Retry-After' 5
  pause 8
}
