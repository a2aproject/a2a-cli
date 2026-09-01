---
name: a2a-cli
description: >-
  Use and learn the `a2a` CLI (github.com/a2aproject/a2a-cli), a command-line
  client for A2A (Agent2Agent) agents. Use when discovering an agent's card,
  sending a message or task to an A2A agent, streaming or polling for the
  result, or checking a task's status later. Because the CLI is stateless, this
  skill records each task started — agent URL, task ID, context ID — to a local
  log so it can be followed up afterwards.
compatibility: >-
  Requires the `a2a` binary on PATH (github.com/a2aproject/a2a-cli). No prebuilt
  releases exist yet, so install from source with Go — see Requirements. This
  skill installs nothing on its own.
license: Apache-2.0
---

# Using and learning the `a2a` CLI

`a2a` is a command-line client for the [A2A protocol](https://a2a-protocol.org/):
give it an agent and a message, and it negotiates the transport from the agent's
card, sends the message, and reports what the agent returned — over JSON-RPC,
REST, or gRPC, the same way each time.

## Requirements

This skill drives the `a2a` binary; it does not install it. Check for it, and
install from source if missing (no prebuilt releases yet; needs a Go toolchain
from https://go.dev/doc/install):

```bash
command -v a2a && a2a version || go install github.com/a2aproject/a2a-cli@latest
```

Re-run the `go install` command to update. The tool is under active development,
so the command surface can change between builds — always trust `a2a --help`
over memory.

## Learn the tool from the tool itself

The CLI is the source of truth for its own commands and flags — do not rely on a
memorized list, which goes stale as the tool evolves. Discover the current
surface at runtime:

```bash
a2a help                 # every command
a2a send --help          # flags for one command
a2a card get --help
a2a task --help          # get, list, cancel, subscribe, push-config
```

## Model — what to know before sending

- **Name the agent** with `-a <host|url|path>` (resolves its card and picks a
  transport) or `-e <url> --transport <rest|jsonrpc|grpc>` (connect to one
  interface directly). You choose the endpoint; the CLI talks only to what you
  give it.
- **Ask for JSON when scripting.** `-o json` prints one parseable object; add
  `--stream` to instead emit a stream of event objects as they arrive.
- **It blocks by default** until the task finishes — no sleep loops needed. Use
  `--async` only if you will follow up later with `a2a task get`.
- **Success is the task state, not the exit code.** A run the CLI completed exits
  `0` even when the agent's task ended `TASK_STATE_FAILED`/`REJECTED` or paused
  at `INPUT_REQUIRED`/`AUTH_REQUIRED`. In JSON read `status.state`; treat
  `TASK_STATE_COMPLETED` as success. (The `text` view shows the short name, e.g.
  `completed`.)
- **The CLI is stateless.** It never remembers the last task. To continue one you
  must pass `--task-id`; to group a new task in an existing conversation you pass
  `--context-id`. That is why every task you start must be **recorded** — below.

## Record every task you start

The CLI keeps no history, so the only way to check a task's status or continue it
later is to have saved its identifiers: the **agent**, the **taskId**, and the
**contextId**. After any `send` that starts a task, append a record to a local
log.

### 1. Pick the log directory (once per working directory)

Prefer a per-project local folder; reuse whichever already exists; only ask the
user when neither does.

```bash
a2a_log_dir() {
  if [ -d ./.a2a-cli-logs ]; then echo ./.a2a-cli-logs; return 0; fi
  if [ -d "$HOME/.a2a-cli-logs" ]; then echo "$HOME/.a2a-cli-logs"; return 0; fi
  return 1   # neither exists: ask the user A vs B, then create the chosen one
}
```

- If it returns a path, tell the user which log you are using and go on.
- If it returns non-zero, ask: **local `./.a2a-cli-logs/` (recommended)** or
  **global `~/.a2a-cli-logs/`**, then `mkdir -p` the chosen one. Add a local
  folder to `.gitignore` so logs are never committed.

### 2. Send and record in one step

`a2a send -o json` returns the protocol object. A created task is a `Task` with
`id`, `contextId`, and `status.state`; a direct `Message` reply has no `id` and
nothing to follow up on. Record a short message **summary**, never the full
content, so no secret or PII lands on disk.

```bash
a2a_record() {   # usage:  <send JSON on stdin> | a2a_record <agent> <summary> <logfile>
  python3 -c '
import json, os, sys, datetime
agent, summary, path = sys.argv[1], sys.argv[2], sys.argv[3]
task = json.load(sys.stdin)
task = task.get("task", task)          # tolerate a future SendMessageResponse wrapper
tid = task.get("id")
if not tid:                            # a direct Message reply: nothing to track
    sys.exit(0)
rec = {
    "ts": datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    "agent": agent, "taskId": tid, "contextId": task.get("contextId"),
    "state": task.get("status", {}).get("state"), "summary": summary[:80],
}
os.makedirs(os.path.dirname(path) or ".", exist_ok=True)
open(path, "a").write(json.dumps(rec) + "\n")
' "$1" "$2" "$3"
}

a2a_send_logged() {   # usage:  a2a_send_logged <agent> <message> [extra a2a flags...]
  local agent="$1" message="$2"; shift 2
  local dir; dir="$(a2a_log_dir)" || { echo "choose a log dir first" >&2; return 1; }
  local json; json="$(a2a send -a "$agent" -o json "$message" "$@")" || return 1
  printf '%s\n' "$json"                # still show the caller the real output
  printf '%s' "$json" | a2a_record "$agent" "$message" "$dir/tasks.jsonl"
}
```

Each record is one JSON line in `<log-dir>/tasks.jsonl`:

```json
{"ts":"2026-09-01T15:26:18Z","agent":"http://127.0.0.1:8080","taskId":"01a0…","contextId":"01a0…","state":"TASK_STATE_COMPLETED","summary":"Summarize this document"}
```

### 3. Follow up on a recorded task

Read the record for an agent + `taskId`, then use the tool to act on it:

```bash
dir="$(a2a_log_dir)"; tail -n 20 "$dir/tasks.jsonl"      # browse recent tasks

a2a task get -a <agent> <taskId>                         # current status + artifacts
a2a task subscribe -a <agent> <taskId>                   # re-attach to a live stream
a2a send -a <agent> --task-id <taskId> "<reply>"         # continue / answer the task
```

Replying with `send --task-id` resumes a task paused at `INPUT_REQUIRED` or
`AUTH_REQUIRED`; `send --context-id <contextId>` starts a new task in the same
conversation.

## A throwaway agent for testing

`a2a server --echo` runs a local A2A agent that echoes messages back — useful for
learning the tool or testing connectivity without a real agent:

```bash
a2a server --echo --port 8080 &
a2a card get http://127.0.0.1:8080
a2a send -a http://127.0.0.1:8080 "ping"
```
