#!/usr/bin/env bash
# Lesson 4, end to end: multi-part messages, streaming, and async.
# Requires the `a2a` CLI on your PATH (go install github.com/a2aproject/a2a-cli@latest).
set -uo pipefail
cd "$(dirname "$0")"

# Use the a2a CLI; if it is installed as a2a-cli, alias it.
shopt -s expand_aliases
type a2a >/dev/null 2>&1 || alias a2a=a2a-cli

PORT=8080
URL="http://localhost:$PORT"
export A2ACLI_AGENT_CARD="$URL"   # so the client commands can skip -a

# Start the lesson 1 script as a server in the background; stop it on exit.
a2a server --exec "python3 -u ../01-exec-demo/a2a_unaware_agent.py" --name "Word Numberer" --port "$PORT" --quiet &
SERVER_PID=$!
trap 'kill "$SERVER_PID" 2>/dev/null' EXIT

for _ in $(seq 1 20); do
  a2a card get >/dev/null 2>&1 && break
  sleep 0.25
done

echo "== text part (shorthand) =="
a2a send "number these words"

echo; echo "== file part =="
a2a send --file-part note.txt --media-type text/plain "with an attachment"

echo; echo "== data part (inline) =="
a2a send --data-part '{"priority":"high"}' "with data"

echo; echo "== streaming =="
a2a send --stream "one two three four five"

echo; echo "== async: returns a task id =="
a2a send --async "one two three four five"

# The `task get`/`list`/`subscribe`/`cancel` commands need a server that keeps
# its tasks. This demo `--exec` server runs synchronously and does not store
# them, so those commands are covered in the README rather than run here.
