# Lesson 4 — messages and tasks

Last updated: 2026.09.08

You can already talk to an agent. This lesson explores two richer areas. First, building a message from several **parts**. Second, the **task** every send creates.

## What you'll learn

- The three kinds of message part: text, file, and data
- How to combine parts in one message
- How to stream a reply and how to send async
- How a send maps to a task, and the `task` commands for servers that keep tasks

## Prerequisites

The `a2a` CLI installed (see the [repo README](../../README.md)). This lesson reuses the Python script from [lesson 1](../01-exec-demo/), whose short per-word delay makes streaming easy to watch. You do not need to have finished the earlier lessons first.

## Start the agent

Run the agent in **terminal A**:

```bash
a2a server --exec "python3 -u ../01-exec-demo/a2a_unaware_agent.py" --name "Word Numberer" --port 8080
```

Set the card once in **terminal B** so the commands below stay short:

```bash
echo "A2ACLI_AGENT_CARD=http://localhost:8080" > .env
```

## Messages

A message is one or more **parts**, sent in the order you list them. There are three kinds:

```bash
# Text part (a trailing string is shorthand for a single text part)
a2a send --text-part "number these words"

# File part — a local path is inlined; a URL is sent by reference
a2a send --file-part note.txt --media-type text/plain

# Data part — structured JSON, from a file or inline
a2a send --data-part priority.json
a2a send --data-part '{"priority":"high"}'

# Combine parts in order
a2a send --text-part "with an attachment" --file-part note.txt --media-type text/plain
```

> The CLI flattens every part into text on the script's stdin, so this demo agent numbers all of them — including the file's name and metadata. Add `-o json` or `--verbose` to see the full message the CLI builds.

### Streaming and async

```bash
# Stream the response as it is produced
a2a send --stream "one two three four five"

# Fire-and-forget — returns a task id immediately
a2a send --async "one two three four five"
```

## Tasks

Every `send` creates a **task**. The `--async` send prints its id and state right away:

```text
Task:     01a08153-324e-7683-8551-320a70453e60
Context:  01a08153-324e-7729-9632-7ecc52f71689
Status:   submitted
History:
  [user] one two three four five
```

Once you have a task id, the `task` commands inspect and manage it:

```bash
a2a task get <task-id>                     # inspect one task
a2a task get <task-id> --history 10 -o json
a2a task list --status completed --limit 20  # list recent tasks
a2a task subscribe <task-id>               # follow it until it finishes
a2a task cancel <task-id>                  # cancel it
```

> These commands need a server that **keeps** its tasks. The demo `--exec` server runs each request synchronously and does not store them, so against it the send commands above work but `task get`/`list`/`subscribe`/`cancel` return an error. Point them at a task-backed A2A server to see them in action.

## Run the whole lesson

`run.sh` starts the agent and walks through the message, streaming, and async commands, then stops the agent:

```bash
bash run.sh
```

## Learn more

Read the [a2a-cli specification](../../specification/SPEC.md) for the full set of commands and flags.
