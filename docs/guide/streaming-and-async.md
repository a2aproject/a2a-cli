# Streaming and Async

By default `a2a send` waits for the task to finish, or to ask you something, and prints it once. Two flags change that.

| Command | What it does |
|---|---|
| `a2a send --stream "..."` | Prints each event as the agent sends it. |
| `a2a send --async "..."` | Prints the task as soon as the agent accepts it, and returns. |
| `a2a task get <id> --wait` | Polls until the task finishes or asks for input. |
| `a2a task subscribe <id>` | Streams the events of a running task. |

## Stream a reply

```bash
a2a send -a localhost:8080 --stream "List the files in your workspace"
```

```text
Task:     01a1d530-5355-76ef-bac5-dbeda9bdf0d8
Context:  01a1d530-5355-778d-9be0-4f020a4b0e84
Status:   submitted
History:
  [user] List the files in your workspace
[status] working
[status] working: Running list files {"toolCall":{"id":"t1","kind":"read","status":"in_progress",...}}
[status] working: Finished list files {"toolCall":{"id":"t1","kind":"read","status":"completed",...}}
[artifact] Listed the files.
[status] completed
```

The first event is the task, then status updates and artifacts as they happen. A status message prints its text parts, then its data parts as JSON; here the agent attaches the tool call it runs. Chunks the agent appends to an artifact continue the same `[artifact]` line.

If the agent doesn't stream, `--stream` sends the message, polls the task every `--poll-interval` (default `2s`) and prints the same kinds of events.

## Send asynchronously

```bash
a2a send -a localhost:8080 --async "Run the full test suite"
```

```text
Task:     01a1d530-53cf-74b7-a36c-af9ea1df605e
Context:  01a1d530-53cf-75ae-86d7-a936d61bd45e
Status:   submitted
```

Then wait for it, or follow it:

```bash
a2a task get -a localhost:8080 01a1d530-53cf-74b7-a36c-af9ea1df605e --wait --timeout 10m
a2a task subscribe -a localhost:8080 01a1d530-53cf-74b7-a36c-af9ea1df605e
```

`--stream` and `--async` can't be combined.

## Timeouts

`--timeout` (default `30s`) bounds a plain `send` and `task get --wait` from start to end. With `--stream` and `task subscribe` it bounds the silence between events, so a long task that keeps reporting progress is followed for as long as it runs. Raise it for slow agents:

```bash
a2a send -a localhost:8080 --timeout 5m "Refactor the parser"
```

A timeout exits with code 5. [Streaming and timeouts](../advanced/streaming.md) explains the details.
