#!/usr/bin/env bash
# Runs the whole relay flow on localhost, with a stand-in for a webhook agent
# such as Grok Bot:
#
#   1. starts a relay,
#   2. starts a webhook "agent" that answers every task with
#      `a2a server relay reply <task> --final "echo: <text>"`,
#   3. attaches the agent's webhook, pairs it and grants a caller,
#   4. sends the agent a message as that caller and prints the result.
#
#   deploy/relay/local-demo.sh
#
# Needs go, python3 and curl. Ports: RELAY_PORT (default 8090) and HOOK_PORT
# (default 8091).
set -euo pipefail

root=$(cd "$(dirname "$0")/../.." && pwd)
relay_port=${RELAY_PORT:-8090}
hook_port=${HOOK_PORT:-8091}
relay_url="http://127.0.0.1:$relay_port"
work=$(mktemp -d -t a2a-relay-demo-XXXX)
pids=()
trap 'kill "${pids[@]}" 2>/dev/null || true; rm -rf "$work"' EXIT

go build -C "$root" -o "$work/a2a" .
a2a="$work/a2a"
# Keep relay.json out of the real home directory.
export HOME="$work/home"
mkdir -p "$HOME"

export A2A_RELAY_ADMIN_TOKEN=demo-admin-token
"$a2a" server relay serve --addr "127.0.0.1:$relay_port" --state-dir "$work/state" >"$work/relay.log" 2>&1 &
pids+=($!)

# The stand-in agent: reads each wake and replies through the relay with the
# CLI, like a webhook-triggered agent following the a2a-relay skill would.
cat >"$work/hook.py" <<'EOF'
import json, os, subprocess, sys
from http.server import BaseHTTPRequestHandler, HTTPServer

class Hook(BaseHTTPRequestHandler):
    def do_POST(self):
        wake = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        self.send_response(200)
        self.end_headers()
        print(f"agent: woke with kind={wake['kind']} sender={wake['sender']} text={wake.get('text', '')!r}", flush=True)
        if wake["kind"] in ("task", "message"):
            subprocess.run([os.environ["A2A"], "server", "relay", "reply", wake["taskId"],
                            "--final", "echo: " + wake.get("text", "")], check=False)

    def log_message(self, *args):
        pass

HTTPServer(("127.0.0.1", int(sys.argv[1])), Hook).serve_forever()
EOF

for _ in $(seq 1 50); do curl -sf "$relay_url/healthz" >/dev/null && break; sleep 0.2; done

echo "== Owner attaches the agent's webhook"
attach=$("$a2a" server relay attach "http://127.0.0.1:$hook_port/" --name demo-agent \
  --webhook-header "Authorization: Bearer demo-routine-secret" --relay "$relay_url" -o json)
code=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["pairingCode"])' <<<"$attach")
node_url=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["node"]["url"])' <<<"$attach")
echo "A2A URL: $node_url, pairing code: $code"

echo "== Agent pairs with the code"
"$a2a" server relay pair "$code" --relay "$relay_url"
A2A="$a2a" python3 "$work/hook.py" "$hook_port" &
pids+=($!)
for _ in $(seq 1 50); do curl -s -o /dev/null "http://127.0.0.1:$hook_port/" && break; sleep 0.2; done
"$a2a" server relay ping

echo "== Agent grants a caller"
token=$("$a2a" server relay grant alice -o json | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])')

echo "== Alice sends the agent a message"
task=$("$a2a" send -a "$node_url" --auth "Bearer $token" --async "hello through the relay" -o json |
  python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("id") or d["task"]["id"])')
"$a2a" task get -a "$node_url" --auth "Bearer $token" "$task" --wait --poll-interval 200ms
