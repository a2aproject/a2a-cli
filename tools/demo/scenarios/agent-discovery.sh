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
  # The opencode TUI is still open when the recording ends.
  docker ps -q --filter "volume=$OPENCODE_VOLUME-data" | xargs -r docker kill >/dev/null 2>&1 || true
  docker volume rm -f "$OPENCODE_VOLUME-data" "$OPENCODE_VOLUME-cache" >/dev/null 2>&1 || true
}

layout() {
  panes B
  split_pane A B above 18
  title A "terminal A: docs.acme.test"
  title B "terminal B: opencode"
  run A "cd ~/a2a-cli/examples/agent-discovery && clear" 30
  run B "cd ~/a2a-cli/examples/agent-discovery && clear" 30
}

# ask types a prompt into the opencode TUI and sends it.
ask() {
  focus B
  type_keys B "$1"
  pause 1
  t send-keys -t "$(pane_id B)" Enter
}

# until_done <ERE> waits until the TUI shows the ERE and then stops changing,
# which means opencode finished its reply. It allows once any permission
# opencode asks for on the way.
until_done() {
  local start last="" now stable=0
  start="$(date +%s)"
  while true; do
    now="$(screen B)"
    if grep -q "Permission required" <<<"$now"; then
      pause 2
      t send-keys -t "$(pane_id B)" Enter
      stable=0
    elif grep -Eq -- "$1" <<<"$now" && [[ "$now" == "$last" ]]; then
      stable=$((stable + 1))
      ((stable >= 5)) && return 0
    else
      stable=0
    fi
    last="$now"
    (($(date +%s) - start < 300)) || die "pane B: no finished reply matching /$1/ after 300s"
    sleep 1
  done
}

scene() {
  start A "go run ./site --addr 127.0.0.1:8080 --url http://docs.acme.test:8080"
  wait_text A 'serving http://docs.acme.test:8080' 120
  pause 1

  run B 'export OPENCODE_DOCKER_ARGS="--add-host docs.acme.test:127.0.0.1"'
  start B "../opencode-contacts/run-opencode"
  skip_from
  wait_text B 'Ask anything' 120
  skip_to
  pause 2

  ask "Read http://docs.acme.test:8080/ and tell me how my client should handle 429s from the Acme Payments API."
  until_done 'Acme Docs Assistant'
  wait_text A 'agent card' 5
  pause 8

  ask "Yes, ask it."
  until_done 'Idempotency-Key'
  wait_text A 'A2A request' 5
  pause 10
}
