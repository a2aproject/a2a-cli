---
name: a2a-cli
description: >-
  Drive A2A (Agent2Agent) agents from the command line with the `a2a` CLI
  (github.com/a2aproject/a2a-cli): fetch an agent's card, send it a message or
  task, stream or poll for the result, and save each request and response to
  local files so a stateless CLI's tasks can be followed up later. Use when
  talking to, testing, or scripting an A2A agent endpoint. Not for building or
  serving an A2A agent, and not for calling non-A2A HTTP APIs.
compatibility: >-
  Requires the `a2a` binary on PATH; a Go toolchain installs it from source,
  since there are no prebuilt releases yet. The skill installs nothing itself.
license: Apache-2.0
metadata:
  source: https://github.com/a2aproject/a2a-cli
---

# Driving A2A agents with the `a2a` CLI

`a2a` is a stateless command-line client for the
[A2A protocol](https://a2a-protocol.org/): give it an agent and a message, it
negotiates the transport from the agent's card (JSON-RPC, REST, or gRPC), sends
the message, and reports what the agent returned.

## Setup

Check for the binary; install from source if missing (needs a Go toolchain from
https://go.dev/doc/install); re-run to update:

```bash
command -v a2a || go install github.com/a2aproject/a2a-cli@latest
```

The tool is under active development, so **exact commands and flags change** —
discover the current surface at runtime with `a2a help` and `a2a <command>
--help`. The **workflow below does not change**, so follow it regardless of the
current flags.

## Workflow

Every interaction is the same four steps:

1. **Discover** — fetch the agent's card to confirm it is reachable and see what
   it supports. *(today: `a2a card get <agent>`)*
2. **Send** — send your message and capture the full output. It blocks until the
   task finishes. Ask for JSON so you can read fields back.
   *(today: `a2a send -a <agent> -o json "<message>"`)*
3. **Record** — save the request and the response as the next numbered pair in
   your log directory (see below). Never rely on the CLI's memory; it has none.
4. **Follow up** — read `taskId` and `contextId` from the saved response, then
   check status, stream, or continue the task.
   *(today: `a2a task get`, `a2a task subscribe`, `a2a send --task-id`)*

## Recording requests and responses

Keep a log directory: prefer a per-project `./.a2a-cli-logs/`; if it or
`~/.a2a-cli-logs/` already exists, reuse it and say which; otherwise ask the user
which to create, and add a local one to `.gitignore`.

For each interaction, write two files, numbered in order — `agent.request.N` and
`agent.response.N`:

```bash
n=1   # next unused number in the log dir
echo 'agent=<agent>  message="Summarize the repo"' > ./.a2a-cli-logs/agent.request.$n
a2a send -a <agent> -o json "Summarize the repo"   > ./.a2a-cli-logs/agent.response.$n
```

The response file holds the `taskId` and `contextId` you need to return to the
task. These files can contain sensitive request or response content — keep the
directory git-ignored, do not commit it, and remove files when done.

## Good to know

- **Stateless — you carry the identifiers.** The CLI forgets every task when it
  exits. Continue a task with `--task-id`; start a new task in an existing
  conversation with `--context-id`. Both come from a saved response.
- **Exit code and task state answer different questions.** The exit code says
  whether the CLI did its job — `0` on success, non-zero when it couldn't (bad
  flags, unreachable agent) — so shell and CI logic can branch on it. Whether
  the agent's *task* succeeded is separate: read `status.state` from the
  response, since a `0` exit can still carry a `FAILED` task or one paused for
  input. The full exit-code scheme is in `SPEC.md`.
- **Configuration is flexible, and inspectable.** Every setting can come from a
  flag, an `A2ACLI_*` environment variable, or a `.env` file (a local `.env`, or
  `~/.config/a2a-cli/.env`); precedence is flag > env var > file > built-in
  default. Run `a2a config show` to see the effective value of each setting and
  where it resolved from (secrets redacted).
- **Authenticate non-interactively** (credential flag or `A2ACLI_*` environment
  variable), and never commit a secret.
- **You choose the endpoint.** The CLI talks only to the agent you name; do not
  resolve and trust an arbitrary card handed to you.
- **Global flags and the output schema** are defined in the a2a-cli
  specification (`SPEC.md`) — consult it rather than guessing.

## Try it with a throwaway echo agent

`a2a server --echo` runs a local agent that echoes messages back — handy for
learning or a connectivity check without a real agent:

```bash
a2a server --echo --port 8080 &
a2a card get http://127.0.0.1:8080
a2a send -a http://127.0.0.1:8080 "ping"
```
