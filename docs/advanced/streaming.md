# Streaming and Timeouts

`send` has three ways to wait for an agent, and `--timeout` means something different in each. This page explains the waiting modes, how the timeouts work, the polling fallback for agents that do not stream, how text output prints streamed chunks, and how `chat` cancels a turn. The [Streaming and Async guide](../guide/streaming-and-async.md) covers everyday use.

## Waiting modes

| Command | Call | Returns when | `--timeout` bounds |
| --- | --- | --- | --- |
| `send` | `SendMessage` | The agent replies: by default when the task finishes or needs input | The whole request |
| `send --stream` | `SendStreamingMessage`, or polling | The event stream ends | The time between two events |
| `send --async` | `SendMessage` with `returnImmediately` | The agent accepts the task | The whole request |
| `task get` | `GetTask` | The reply arrives | Client creation and the request together |
| `task get --wait` | `GetTask`, repeated | The task is terminal or needs input | Client creation and the whole wait together |
| `task subscribe` | `SubscribeToTask` | The event stream ends | The time between two events |

For `send` and `chat`, creating the client (resolving the card, running credential helpers, starting a transport plugin) has its own `--timeout` budget, separate from the call. `--stream` and `--async` together are a usage error.

A timeout exits with code 5:

```text
Error: failed to send message: failed to send HTTP request: Post "http://127.0.0.1:35261": context deadline exceeded
hint: increase --timeout, or start the task with --async and follow it later
```

`--timeout 0` disables the inactivity timeout, but a whole-request timeout of 0 expires at once. Client creation in every command except `task subscribe` has a whole-request timeout, so with `--timeout 0` those commands fail with exit code 5 before they reach the agent.

## Inactivity timeout

`--stream` and `task subscribe` use `utils.WithInactivityTimeout` (`internal/utils/timeout.go`). It returns a context that is canceled when nothing resets its timer for `--timeout`. Three things reset it:

1. Each event the CLI receives.
2. Each response body read through `utils.ActivityTransport`, which wraps the HTTP client of the REST and JSON-RPC transports. Any bytes count, including the SSE keep-alive comments a server sends while the agent works. The SSE parser drops those comments without producing an event, so without this an agent that thinks for longer than `--timeout` would time out.
3. Response headers arriving.

gRPC streams have no keep-alive bytes at this level, so only events reset the timer. When the timer expires, the error says how long the stream was idle:

```text
Error: streaming error: SSE stream error: no activity for 1s: context deadline exceeded
hint: increase --timeout, or start the task with --async and follow it later
```

A task that keeps sending events or keep-alives is followed for as long as it runs. For `task subscribe`, client creation counts against the first interval.

## Polling fallback

`send --stream` and `chat --stream` fall back to polling (`internal/polling`) when streaming fails with `a2a.ErrUnsupportedOperation`. That happens in two cases:

- The card's `capabilities.streaming` is false. The CLI checks this before calling, so it does not try to stream. With `--endpoint` there is no card, and this check is skipped.
- The agent answers the streaming call with an unsupported operation error.

`polling.Stream` then:

1. Sends the message again as a `SendMessage` with `returnImmediately` set, and prints the reply. A `Message` reply ends the turn.
2. Waits `--poll-interval` (default 2s) and calls `GetTask`.
3. If the task is terminal, `input-required` or `auth-required`, prints the whole task and stops.
4. Otherwise prints the changes since the last poll as events, built with a2a-go's `a2aevent.Recover`: an artifact event for each new or replaced artifact, an append event for parts added to an artifact, and a status event when the status changed.
5. Gives up after 3 `GetTask` failures in a row. A success resets the count.

`-v` logs the switch:

```text
# falling back to polling (2s): streaming not listed in agent capabilities: unsupported operation
```

The inactivity timeout keeps running while polling: each event and each poll response resets it. A `--poll-interval` longer than `--timeout` makes the command time out between polls.

## `task get --wait`

`polling.WaitForTask` calls `GetTask` at once, then every `--poll-interval`, until the task is terminal (`completed`, `failed`, `canceled`, `rejected`) or needs input (`input-required`, `auth-required`). It prints only the final task. Three failed polls in a row end the wait with an error. `--timeout` bounds the whole wait, so set it to the longest you are willing to wait:

```bash
a2a task get -a https://agent.example.com <task-id> --wait --poll-interval 5s --timeout 10m
```

## `task subscribe`

`task subscribe` attaches to a task that is already running and prints its events until the stream ends. The first event is usually the task as it stands, followed by new events:

```text
Task:     01a1d53b-9121-70a5-b0ab-4fa0a1e20c09
Context:  01a1d53b-9121-7186-a861-3c5f24703e44
Status:   working (2026-11-13T14:06:53Z)
Artifacts:
  [response] ......
History:
  [user] slow 2s
[artifact+] .................................Done.
[status] completed
```

Subscribing to a task that has finished is up to the agent; many reply with an error.

## Streamed artifacts in text output

Agents often stream an answer as many artifact chunks, each appending a few characters to one artifact. The text printer (`printArtifactChunk` in `internal/output/output.go`) writes them inline:

- A chunk that starts an artifact begins a line with `[artifact] `.
- A chunk with `append` set for the artifact that is open continues the same line with no separator.
- A chunk with `append` set for any other artifact, or after another event closed the line, begins a line with `[artifact+] `. The `+` marks text that continues an artifact whose start was printed earlier or not at all.
- A chunk with `lastChunk` set, any other event, and the end of the stream end the line.

```text
[status] working
[artifact] ...................Done.
[status] completed
```

JSON output prints every chunk as a separate `artifactUpdate` event. See [Output](../reference/output.md).

## Canceling a chat turn

`chat` (`internal/chat/session.go`) gives each turn its own interrupt scope:

| Event | Result |
| --- | --- |
| Ctrl-C during a turn | The turn stops, `Interrupted.` is printed, and the CLI sends `CancelTask` for the running task. |
| Ctrl-C during that `CancelTask` | The cancel request is abandoned. |
| Turn exceeds `--timeout` | The error is printed and the CLI sends `CancelTask` for the running task. |
| Other turn error | The error is printed and the session continues. |
| Ctrl-C, Ctrl-D, `/exit` or `/quit` at the prompt | The session ends and prints the context and task IDs to resume with. |

The CLI prints `Canceled task <id>.` or `Could not cancel task <id>: <error>`. `CancelTask` has its own `--timeout` budget. Either way the session returns to the prompt.

The CLI can cancel only a task whose ID it has seen. With `--stream`, the first event names the task. Without `--stream`, a new task is named only by the reply, so an interrupted first turn on a new task leaves it running on the agent. A turn that continues an `input-required` task knows the ID from the start.
