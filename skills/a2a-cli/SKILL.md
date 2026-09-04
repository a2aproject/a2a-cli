---
name: a2a-cli
description: >-
  Delegate work to A2A (Agent2Agent) agents from the command line with the `a2a`
  CLI (github.com/a2aproject/a2a-cli). Use when you need to hand a task to a
  specialized or remote agent, fetch an agent's card, send a message to an agent
  at a URL, stream or poll a running task, or check, list, resume, or cancel an
  A2A task. Not for building or serving an A2A agent, or for calling non-A2A
  HTTP APIs.
compatibility: >-
  Requires the `a2a` binary on PATH; a Go toolchain installs it from source,
  since there are no prebuilt releases yet. The skill installs nothing itself.
license: Apache-2.0
metadata:
  source: https://github.com/a2aproject/a2a-cli
  version: "2026.09.04"
---

# Driving A2A agents with the `a2a` CLI

`a2a` is a stateless client for the [A2A protocol](https://a2a-protocol.org/):
give it an agent and a message, it negotiates the transport from the agent's
card (JSON-RPC, REST, or gRPC), sends the message, and reports what the agent
returned.

## Core concepts

- An interaction starts with a **Message** you send to an agent.
- The agent replies with either a **Message** (a direct answer) or a **Task**
  (tracked, potentially long-running work).
- A **Task** has a server-assigned `taskId` and a `contextId`, and moves through
  states. It can complete, fail, be cancelled, or become **interrupted** when it
  needs your input or authentication to continue.
- Resume an interrupted task by sending a Message with its `--task-id`.
- Group related work with `--context-id`: a Task created in reply joins that
  context. Many tasks can share one context, and a `--task-id` and `--context-id`
  passed together must agree.
- The server keeps tasks (and the messages tied to them); a plain Message not
  tied to a task is not stored — so **you** hold the `taskId`/`contextId` to
  return to work later.

## Setup

Check for the binary; install from source if missing (needs a Go toolchain from
https://go.dev/doc/install); re-run to update:

```bash
go install github.com/a2aproject/a2a-cli@latest
```

The tool is under active development. Treat `a2a help` and `a2a <command>
--help` as the source of truth for the current commands and flags.

## Task lifecycle

Point at an agent with `-a <host|url|path>` (resolves its card and picks a
transport) or `-e <url> --transport <rest|jsonrpc|grpc>` (connect directly).

1. **Inspect the agent** — confirm it is reachable and see what it supports:

   ```bash
   a2a card get https://agent.example.com
   ```

2. **Send a message.** `send` blocks until the task reaches a terminal or
   interrupted state. Add `-o json` for machine-readable output, and note the
   `taskId` and `contextId` in the response — you need them to continue:

   ```bash
   a2a send -a https://agent.example.com "Summarize this repo"
   a2a send -a https://agent.example.com -o json "Summarize this repo"
   ```

3. **Follow a long task live** instead of blocking, or re-attach to one later:

   ```bash
   a2a send -a https://agent.example.com --stream "Run a long analysis"
   a2a task subscribe -a https://agent.example.com <task-id>
   ```

4. **Check status and fetch results** at any time:

   ```bash
   a2a task get -a https://agent.example.com <task-id>
   ```

5. **Answer an interrupted task** (it reached `INPUT_REQUIRED` / `AUTH_REQUIRED`)
   by replying on the same task:

   ```bash
   a2a send -a https://agent.example.com --task-id <task-id> "Yes, proceed"
   ```

6. **Continue the conversation** as a new task in the same context:

   ```bash
   a2a send -a https://agent.example.com --context-id <context-id> "Follow-up question"
   ```

7. **List or cancel tasks:**

   ```bash
   a2a task list -a https://agent.example.com
   a2a task cancel -a https://agent.example.com <task-id>
   ```

## Key flags

Run `a2a <command> --help` for the full, current set. The load-bearing ones:

| Flag | Use |
|---|---|
| `-a, --agent-card <host\|url\|path>` | Resolve the agent's card (picks the transport). |
| `-e, --endpoint <url>` + `--transport <rest\|jsonrpc\|grpc>` | Connect to one interface directly, skipping card resolution. |
| `-o, --output json` | Machine-readable output; add `--stream` for a live event stream. |
| `--async` | Return immediately with the identifiers instead of blocking; poll later with `task get`. |
| `--task-id <id>` / `--context-id <id>` | Continue a task / group a new task under a context. |
| `--auth "<creds>"` / `--svc-param <k=v>` | Attach credentials or transport parameters (or set `A2ACLI_*` env vars). Never commit a secret. |

## Configuration

Every setting can come from a flag, an `A2ACLI_*` environment variable, or a
`.env` file (a local `.env`, or `~/.config/a2a-cli/.env`); precedence is
flag > env var > file. Inspect the effective values and where each resolved from
with `a2a config show` (secrets redacted).
