# Lesson 4 — messages and tasks

You can already talk to an agent. This lesson explores two richer areas. First, building a message from several **parts**. Second, managing the **tasks** an agent creates.

## What you'll learn

- The three kinds of message part: text, file, and data
- How to combine parts in one message
- How to stream a reply and how to send async
- How to inspect and manage tasks with `task get`, `task list`, `task subscribe`, and `task cancel`

## Prerequisites

The `a2a` CLI installed (see the [repo README](../../README.md)). This lesson reuses the Python script from lesson 1, whose short per-word delay makes long-running tasks easy to watch. You do not need to have finished the earlier lessons first.

## Start the agent

Run the agent in **terminal A**:

```bash
a2a server --exec "python3 -u ../exec-demo/a2a_unaware_agent.py" --name "Word Numberer" --port 8080
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

> This demo agent reads only the first text part. The other parts are still sent — add `-o json` or `--verbose` to see the full message the CLI builds.

### Streaming and async

```bash
# Stream the response as it is produced
a2a send --stream "one two three four five"

# Fire-and-forget — returns a task id immediately
a2a send --async "one two three four five"
```

## Tasks

Every `send` creates a **task**. The async send above prints its id; use it below:

```bash
# Inspect one task
a2a task get <task-id>
a2a task get <task-id> --history 10 -o json

# List recent tasks, optionally filtered
a2a task list
a2a task list --status completed --limit 20

# Follow a running task until it finishes
a2a task subscribe <task-id>

# Cancel a task
a2a task cancel <task-id>
```

## Run the whole lesson

`run.sh` starts the agent and walks through the message and task commands, then stops the agent:

```bash
bash run.sh
```

## Learn more

Read the [a2a-cli specification](../../specification/SPEC.md) for the full set of commands and flags.
