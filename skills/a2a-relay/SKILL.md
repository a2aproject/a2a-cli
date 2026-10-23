---
name: a2a-relay
description: Receive and answer A2A tasks as a webhook node hosted by an `a2a server` agent. Use when a webhook wakes you with JSON that has kind, taskId and replyUrl fields, when asked to pair as a relay node using a code, or to reply to, check or list A2A tasks.
---

# A2A webhook node

The agent at {{BASE}} hosts you as a webhook node. It gives you an A2A
address ({{BASE}}/n/<node-id>) while you are asleep: it holds each task and
wakes you by POSTing a JSON envelope to your webhook. You answer later with
the `a2a` CLI or plain HTTPS. The agent keeps all task state, so you don't
need to remember anything between wakes: every wake carries its task ID.

## Install

    curl -fsSL -o a2a {{BASE}}/download/a2a && chmod +x a2a

If you can't run it, every command below has an HTTP equivalent (see
"Without the CLI").

## Pair (once)

The agent's owner adds your webhook and gives you a pairing code such as
K7QF-M2XD. It is valid for 15 minutes and works once:

    ./a2a pair add {{BASE}} <code>

This prints your node token and saves it to ~/.config/a2a-cli/credentials.json.
If you run in a fresh environment on every wake, store the token as a secret
named A2A_RELAY_TOKEN. The token also contains the agent's URL, so it is the
only setting you need. Never share it.

    ./a2a relay inbox

## Handle a wake

Every wake has the same JSON shape, signed with the header
`X-A2A-Serve-Signature: t=<unix time>,v1=<hex HMAC-SHA256 of "<t>.<body>">`:

    {"id": "wake_…", "kind": "task", "node": "…", "taskId": "…", "contextId": "…",
     "sender": {"kind": "agent", "name": "assistant", "onBehalfOf": "user:owner"},
     "text": "…", "message": {…}, "state": "submitted", "replyUrl": "…", "instructions": "…"}

| kind | Meaning | What to do |
| --- | --- | --- |
| task | A new task. | Do the work, then reply. |
| message | The caller answered your input-required question. | Continue the same task and reply. |
| canceled | The caller canceled the task. | Stop. Replies are no longer accepted. |
| ping | Test wake. | Nothing. |

Use contextId to recognize follow-ups in the same conversation. Fetch the full
history with `./a2a relay task <taskId>`.

## Reply

    ./a2a relay reply <taskId> --status working --text "Looking into it"
    ./a2a relay reply <taskId> --input-required "Which branch?"
    ./a2a relay reply <taskId> --final "Done: …" --artifact report.pdf
    ./a2a relay reply <taskId> --fail "Could not reach the repo"

- Finish every task with --final, --input-required or --fail. A task with no
  reply for 10 minutes fails, and each reply resets that timer, so post
  --status working during long jobs.
- --input-required pauses the task. The caller's answer arrives as a message
  wake with the same taskId.
- --artifact attaches a file (repeatable). The --final text becomes the
  "response" artifact.
- Retrying a reply is safe: the CLI derives an idempotency key from the
  content, and the agent applies each key once.

## Catch up

    ./a2a relay inbox          # wakes not delivered, or tasks still waiting for you
    ./a2a relay inbox --all    # the last 200 wakes

## Call the agent

To send tasks to the agent itself, pair again with a caller code from its
owner (`a2a server pair --name <you>` on their side):

    ./a2a pair add {{BASE}} <code>
    ./a2a send -a {{BASE}} "Summarize what changed today"

## Without the CLI

Send the header Authorization: Bearer $A2A_RELAY_TOKEN and JSON bodies.

    POST {{BASE}}/pair                          {"code": "K7QF-M2XD"}   (no token; returns it)
    POST {{BASE}}/relay/api/tasks/<id>/events   {"state": "completed", "text": "…", "idempotencyKey": "<unique>"}
    GET  {{BASE}}/relay/api/tasks/<id>
    GET  {{BASE}}/relay/api/inbox[?all=1]
    GET  {{BASE}}/relay/api/node

Event states: working, input-required, completed, failed, rejected. Events can
also carry "parts" (A2A parts for the status message) and "artifacts".
