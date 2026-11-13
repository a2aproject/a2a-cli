# Lesson 3: Messages and Tasks

So far every message was one line of text, and every task finished while you waited. In this lesson you send messages made of several parts, start tasks without waiting for them, and use the `task` commands to check on them, follow them and cancel them.

![Sending a multi-part message, starting a task with --async, waiting for it, following it and canceling one](demo.gif)

## What you'll learn

- The three kinds of message part: text, file and data
- How `--async` returns as soon as the agent accepts a task
- How to wait for a task, follow its events and cancel it
- How to list the tasks an agent keeps for you

## Prerequisites

The agent from [lesson 1](../01-first-message/README.md), and the `.env` in `~/a2a-lessons` that points at it. Create two files to send, next to your `.env`:

```bash
cd ~/a2a-lessons
echo "Orders of more than 2 items ship from the Berlin warehouse." > notes.txt
echo '{"id": 1042, "items": 3, "destination": "Lisbon"}' > order.json
```

## Step 1: A message with parts

An A2A message is a list of **parts**. A text part carries text, a file part carries a file's bytes or a link to it, and a data part carries JSON. `a2a send` builds the message from its flags, in the order you write them:

```bash
a2a send \
  --text-part "Check this order against the notes, and say whether it can ship." \
  --file-part notes.txt \
  --data-part order.json
```

`--file-part notes.txt` reads the file and sends its bytes, with `notes.txt` as its name and `text/plain` as its media type, guessed from the extension. A URL in its place, such as `--file-part https://example.com/spec.pdf`, is sent as a link for the agent to fetch.

`--data-part order.json` sends the file's JSON as structured data. It also takes the JSON itself:

```bash
a2a send --data-part '{"id": 1042, "items": 3}' "How many items are in this order?"
```

The text you write without a flag becomes a text part at the start. Whether an agent understands a part depends on the agent: its card lists the media types it accepts under `defaultInputModes`.

## Step 2: Start a task and come back later

Some tasks take minutes. `--async` returns as soon as the agent accepts the task, with its ID:

```bash
a2a send --async "Count to twenty, slowly."
```

```text
Task:     01a1d530-53cf-74b7-a36c-af9ea1df605e
Context:  01a1d530-53cf-75ae-86d7-a936d61bd45e
Status:   submitted
```

Keep the ID. `task get` shows where the task is now, and `--wait` polls until it finishes:

```bash
TASK=01a1d530-53cf-74b7-a36c-af9ea1df605e
a2a task get "$TASK"
a2a task get "$TASK" --wait --timeout 5m
```

`--timeout` bounds the whole wait; without it, `a2a` gives up after 30 seconds and exits with code 5. The task keeps running on the agent either way.

## Step 3: Follow a running task

`task subscribe` streams a task's events, like `send --stream` does for a new one. Start another task and follow it:

```bash
TASK="$(a2a send --async -o json "Count to ten, slowly." | jq -r .task.id)"
a2a task subscribe "$TASK"
```

`-o json` prints the task as JSON, and `jq` takes its ID. Lesson 1's `[status]` and `[artifact]` lines appear as the agent works, and `subscribe` returns when the task finishes.

## Step 4: Cancel a task

```bash
TASK="$(a2a send --async -o json "Count to a million, slowly." | jq -r .task.id)"
a2a task cancel "$TASK"
```

```text
Task:     01a1d530-8953-7482-912d-ec575378dd06
Context:  01a1d530-8953-752c-a092-eb5533a521c3
Status:   canceled (2026-11-13T13:54:50Z)
```

A task that already finished can't be canceled; the agent returns an error.

## Step 5: List your tasks

```bash
a2a task list
a2a task list --status completed --limit 5
```

```text
ID                                    STATUS     CONTEXT
01a1d530-8953-7482-912d-ec575378dd06  canceled   01a1d530-8953-752c-a092-eb5533a521c3
01a1d530-847f-7708-a0c7-8efe4d69738e  completed  01a1d530-847f-77a5-8362-992ffda09ddc
01a1d530-53cf-74b7-a36c-af9ea1df605e  completed  01a1d530-53cf-75ae-86d7-a936d61bd45e
```

Which tasks an agent keeps, and which of them it shows you, is up to the agent.

## Recap

- Messages are lists of text, file and data parts, sent in the order of the flags.
- `--async` returns at once; `task get --wait` and `task subscribe` pick the task up later.
- `task cancel` stops a running task, and `task list` finds the ones you started.

## Next

[Lesson 4](../04-conversations/README.md) keeps a conversation going across tasks, and answers an agent that asks you something.
