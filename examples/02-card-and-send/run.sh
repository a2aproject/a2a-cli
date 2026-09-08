#!/usr/bin/env bash
# Lesson 2, end to end: start the agent, read its card, set config, send a message.
# Requires the `a2a` CLI on your PATH (go install github.com/a2aproject/a2a-cli@latest).
set -uo pipefail
cd "$(dirname "$0")"

# Use the a2a CLI; if it is installed as a2a-cli, alias it.
shopt -s expand_aliases
type a2a >/dev/null 2>&1 || alias a2a=a2a-cli

PORT=8090
URL="http://localhost:$PORT"

# Start the built-in echo server in the background; stop it on exit.
a2a server --echo --name "Echo Agent" --port "$PORT" --quiet &
SERVER_PID=$!
trap 'kill "$SERVER_PID" 2>/dev/null' EXIT

# Wait for it to accept requests.
for _ in $(seq 1 20); do
  a2a card get "$URL" >/dev/null 2>&1 && break
  sleep 0.25
done

echo "== card get =="
a2a card get "$URL"

echo; echo "== card get -o json =="
a2a card get "$URL" -o json

echo; echo "== export the card to agent-card.json =="
a2a card get "$URL" -o json > agent-card.json
echo "wrote agent-card.json"

echo; echo "== set the card via .env, then drop -a =="
echo "A2ACLI_AGENT_CARD=$URL" > .env
a2a config show

echo; echo "== send a text message =="
a2a send "hello world from A2A"
