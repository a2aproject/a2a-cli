#!/usr/bin/env bash
# Progressive discovery demo, non-interactive walkthrough.
#
# Builds the `a2a` CLI and the Nimbus demo server, starts the server (one origin
# serving BOTH a website and an A2A agent), then reproduces what a Claude Code
# hook does: it discovers the agent from a URL and delegates the task to it.
#
# Default: runs on http://localhost:8080 (no root, no hosts entry needed).
# Pretty domain: run ./setup-hosts.sh first, then `DOMAIN=nimbus.coffee ./run.sh`
# (uses sudo to bind :80 so the URL has no port).
set -euo pipefail

cd "$(dirname "$0")/../.."   # repo root
DOMAIN="${DOMAIN:-}"

if [[ -n "$DOMAIN" ]]; then
  PORT="${PORT:-80}"
  if [[ "$PORT" == "80" ]]; then ORIGIN="http://${DOMAIN}"; else ORIGIN="http://${DOMAIN}:${PORT}"; fi
  BIND=":${PORT}"
  ALLOW=(--allow "$DOMAIN")
  SUDO=""; [[ "$PORT" == "80" ]] && SUDO="sudo"
  if ! getent hosts "$DOMAIN" >/dev/null 2>&1 && ! ping -c1 -W1 "$DOMAIN" >/dev/null 2>&1; then
    echo "!! ${DOMAIN} does not resolve. Run ./examples/05-progressive-discovery/setup-hosts.sh first." >&2
    exit 1
  fi
else
  PORT="${PORT:-8080}"
  ORIGIN="http://localhost:${PORT}"
  BIND="localhost:${PORT}"
  ALLOW=(--allow-private)
  SUDO=""
fi

echo "== build =="
go build -o /tmp/a2a-demo .
go build -o /tmp/nimbus-demo ./examples/05-progressive-discovery/server
A2A=/tmp/a2a-demo

echo "== start server on ${ORIGIN} =="
${SUDO} /tmp/nimbus-demo -addr "${BIND}" -public-url "${ORIGIN}" >/tmp/nimbus-demo.log 2>&1 &
SERVER_PID=$!
trap 'kill "${SERVER_PID}" 2>/dev/null || true' EXIT
sleep 1

echo
echo "== 1. The website a user (or agent) fetches =="
curl -s "${ORIGIN}/" | grep -E '<title>|Track your order' | sed 's/^ *//'

echo
echo "== 2. A hook fires on the URL and discovers an agent at the SAME origin =="
echo "   (simulating a Claude Code UserPromptSubmit event on stdin)"
printf '{"hook_event_name":"UserPromptSubmit","session_id":"demo","prompt":"where is my nimbus order NR-2041? site: %s"}' "${ORIGIN}" \
  | "${A2A}" discover "${ALLOW[@]}" --no-cache \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["hookSpecificOutput"]["additionalContext"])'

echo
echo "== 3. Now the model knows to delegate — send the task to the discovered agent =="
"${A2A}" send -a "${ORIGIN}" "Where is my order NR-2041?"

echo
echo "Done. To try it live in Claude Code, see this folder's README.md."
