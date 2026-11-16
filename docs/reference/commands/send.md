# a2a send

<!-- Generated from the command definitions; do not edit. -->

Send a message to an agent and print the result.

The message is built from the positional text and the --text-part,
--file-part and --data-part flags, in the order they appear. Without
--stream or --async, send waits for the task to finish or to ask for input.
--stream prints each event as it arrives, and polls when the agent doesn't
stream. --async returns as soon as the agent accepts the task.

```
a2a send [message] [flags]
```

**Examples**

```bash
a2a send -a https://agent.example.com "What can you do?"
a2a send -a https://agent.example.com --stream "Summarize the report"
a2a send -a https://agent.example.com --async "Analyze the dataset"
a2a send -a https://agent.example.com --task-id <task-id> "yes, go ahead"
a2a send -a https://agent.example.com --context-id <context-id> "And the next one?"
a2a send -a https://agent.example.com --text-part "Review this" --file-part report.pdf
a2a send -a https://agent.example.com --data-part '{"priority":"high"}' "Triage"
```

**Flags**

| Flag | Default | Description |
|---|---|---|
| `--async` |  | Return immediately (fire-and-forget) instead of waiting for completion |
| `--context-id string` |  | Context ID to group this turn under |
| `--data-part path\|json` |  | Add a JSON data part from a file path or an inline JSON string (repeatable) |
| `--file-part path\|url` |  | Add a file part from a local path (inlined) or URL (by reference); repeatable |
| `--history int` |  | Request n history messages in the response |
| `--media-type string` |  | Media type for the immediately preceding part flag |
| `--metadata json` |  | Attach request metadata as a JSON object (repeatable) |
| `--poll-interval duration` | `2s` | Duration between GetTask requests in polling fallback mode. |
| `--request-payload string` |  | Full SendMessageRequest as a JSON file path or inline JSON string (mutually exclusive with the message-building flags) |
| `--save-fileparts string` |  | A directory for storing raw-bytes message and artifact parts as files. |
| `--stream` |  | Use streaming response. Falls back to polling and synthetic events if a server does not support streaming. |
| `--task-id string` |  | Task ID to continue an existing task |
| `--text-part string` |  | Add a text part (repeatable, order-preserving) |

The [global flags](a2a.md#global-flags) apply too.
