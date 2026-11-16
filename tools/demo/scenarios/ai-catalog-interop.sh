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
# summary: examples/ai-catalog-interop: a contact seen by ai-catalog, an ai-catalog catalog used by a2a

# Long replies: keep the pauses after them, so they can be read.
DEMO_GIF_SPEED=1.25
DEMO_GIF_IDLE=7

prepare() {
  ai_catalog_tool
  mkdir -p "$HOME/ai-catalog-interop"
  cp "$REPO_ROOT"/examples/ai-catalog-interop/{acme-catalog.json,start-agents.sh} "$HOME/ai-catalog-interop/"
}

layout() {
  panes B
  split_pane A B above 20
  title A "terminal A: agents"
  title B "terminal B"
  run A "cd ~/ai-catalog-interop && clear" 30
  run B "cd ~/ai-catalog-interop && clear" 30
}

scene() {
  start A "./start-agents.sh"
  wait_text A 'agents on' 30
  until_ok 30 curl -fsS http://127.0.0.1:9103/.well-known/agent-card.json
  pause 1

  say B "A contact, seen by ai-catalog"
  run B 'a2a contact add weather localhost:9101 --note "Forecasts for trip planning"'
  wait_text B 'Added contact weather' 5
  run B "ai-catalog catalog list"
  wait_text B 'a2a-contacts +file://' 5
  run B "ai-catalog search weather"
  wait_text B 'urn:ai-catalog:a2a-cli:weather +Weather' 5
  pause 8

  clear_pane B
  say B "A catalog, used by a2a"
  run B "ai-catalog catalog add acme ./acme-catalog.json"
  wait_text B '"acme" added' 5
  run B "a2a catalog list"
  run B "a2a catalog search travel"
  wait_text B 'urn:air:acme.example:agent:travel +Travel Desk' 5
  pause 8

  clear_pane B
  run B 'a2a send -a urn:air:acme.example:agent:travel "Lisbon, 3 nights from Dec 2"'
  expect_status B completed
  pause 8
  clear_pane B
  run B "a2a contact add travel urn:air:acme.example:agent:travel"
  run B 'a2a send -a travel "Porto, 1 night from Dec 5"'
  expect_status B completed
  run B "a2a contact list"
  wait_text B 'travel +Travel Desk' 5
  pause 8
}
