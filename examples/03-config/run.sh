#!/usr/bin/env bash
# Lesson 3, end to end: the three ways to set a value, and `config show`.
# Requires the `a2a` CLI on your PATH (go install github.com/a2aproject/a2a-cli@latest).
set -uo pipefail
cd "$(dirname "$0")"

# Use the a2a CLI; if it is installed as a2a-cli, alias it.
shopt -s expand_aliases
type a2a >/dev/null 2>&1 || alias a2a=a2a-cli

PORT=8090
URL="http://localhost:$PORT"

# Start a throwaway echo server in the background; stop it on exit.
a2a server --echo --port "$PORT" --quiet &
SERVER_PID=$!
trap 'kill "$SERVER_PID" 2>/dev/null; rm -f .env' EXIT

# Wait for it to accept requests.
for _ in $(seq 1 20); do
  a2a card get "$URL" >/dev/null 2>&1 && break
  sleep 0.25
done

echo "== (a) pass it on the command line =="
a2a send -a "$URL" "hello from a flag"

echo; echo "== (b) session environment variable =="
export A2ACLI_AGENT_CARD="$URL"
a2a send "hello from an env var"

echo; echo "== (c) .env file (unset the env var first so the file is used) =="
unset A2ACLI_AGENT_CARD
echo "A2ACLI_AGENT_CARD=$URL" > .env
a2a send "hello from .env"

echo; echo "== config show: effective values and their sources =="
a2a config show
