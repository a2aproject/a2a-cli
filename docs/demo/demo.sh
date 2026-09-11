#!/usr/bin/env bash
#
# Drives the terminal recording for the README demo GIF.
#
# It stands up two throwaway A2A servers from ordinary scripts (--exec), then
# discovers and talks to them with the a2a CLI. Nothing here is A2A-specific
# code; that is the point of the demo.
#
# Record and render:
#   asciinema rec --command "bash docs/demo/demo.sh" docs/demo/a2a-demo.cast
#   agg docs/demo/a2a-demo.cast docs/demo/a2a-demo.gif
#
# Requires: a2a on PATH, python3, bash. Run from the repo root.

set -euo pipefail

EX="examples/01-exec-demo"
PROMPT="$ "

# Print a command as if typed, then run it.
run() {
  printf '%s' "$PROMPT"
  printf '%s' "$*" | while IFS= read -r -n1 ch; do
    printf '%s' "$ch"
    sleep 0.02
  done
  printf '\n'
  sleep 0.4
  eval "$*"
  sleep 1.0
}

# Print a comment line (no execution).
note() {
  printf '# %s\n' "$*"
  sleep 0.7
}

cleanup() {
  [[ -n "${SRV_A:-}" ]] && kill "$SRV_A" 2>/dev/null || true
  [[ -n "${SRV_B:-}" ]] && kill "$SRV_B" 2>/dev/null || true
}
trap cleanup EXIT

clear
note "The a2a CLI: one command surface for any A2A agent."
run "a2a version"

note "Turn an ordinary script into an A2A server — no A2A code."
a2a server --exec "bash $EX/content-generator.sh" --port 8080 >/tmp/a2a-demo-a.log 2>&1 &
SRV_A=$!
a2a server --exec "python3 -u $EX/a2a_unaware_agent.py" --chunk=$'\n' --port 8081 >/tmp/a2a-demo-b.log 2>&1 &
SRV_B=$!
sleep 1.5

note "Discover what an agent can do — read its card."
run "a2a card get -a http://localhost:8080"

note "Send a message and wait for the result."
run 'a2a send -a http://localhost:8080 "hello world from A2A"'

note "Stream a reply piece by piece as it arrives."
run 'a2a send -a http://localhost:8081 --stream "one two three four"'

note "That is a full A2A round trip. Learn more: examples/"
sleep 1.5
