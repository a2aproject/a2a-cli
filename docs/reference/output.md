# Output

Commands print results to stdout in one of three formats, chosen with `-o`: `text` (default), `json` or `jsonl`. Text is for reading. JSON prints the A2A protocol objects as the SDK serializes them. Progress, warnings and verbose logs go to stderr in every format. Errors go to stderr in text mode and to stdout in JSON modes; see [Errors and Exit Codes](errors.md). The [Scripting guide](../guide/scripting.md) shows how to use the JSON output.

## Formats

| `-o` | Output |
| --- | --- |
| `text` | The layouts below |
| `json` | Each value as JSON indented by two spaces. A command that prints several events prints one indented document per event, one after another, with no enclosing array. |
| `jsonl` | Each value as one compact JSON object on its own line |

Any other value is a usage error. `version` prints JSON only with `-o json`; with `-o jsonl` it prints text.

## Agent card

`card get` prints the card.

```text
Name:         demo
Description:  demo is an acp coding agent reached through `a2a-serve`.
Version:      1.0.0
Interfaces:
  JSONRPC (A2A v1.0)     http://127.0.0.1:35261
  HTTP+JSON (A2A v1.0)   http://127.0.0.1:35261
Streaming:    true
Skills:
  coding               Coding agent
```

| Line | Shown |
| --- | --- |
| `Description:` | When the card has one |
| `Interfaces:` | When the card lists any. One line per interface: binding and protocol version padded to 22 characters, the URL, and `  (tenant: <t>)` when the interface has a tenant |
| `Skills:` | When the card lists any. One line per skill: ID padded to 20 characters, then the name |

With `-o json` it prints the `AgentCard` object, abridged here. A v0.3 card is printed in the v1.0 form the CLI converted it to.

```json
{
  "supportedInterfaces": [
    {
      "url": "http://127.0.0.1:35261",
      "protocolBinding": "JSONRPC",
      "protocolVersion": "1.0"
    }
  ],
  "capabilities": {
    "streaming": true
  },
  "name": "demo",
  "version": "1.0.0"
}
```

## Events

`send`, `chat`, `task get`, `task subscribe` and `task cancel` print the protocol events they receive. `send` without `--stream` prints one event, the task or message the agent replied with. `send --stream` and `task subscribe` print each event as it arrives.

### Text

A task:

```text
Task:     01a1d53b-2106-7b83-827c-421754d88e35
Context:  01a1d53b-2106-7ce2-9a4f-70e2172430e6
Status:   completed (2026-11-13T14:06:24Z)
Artifacts:
  [response] Hello from fakeacp.
```

| Line | Shown |
| --- | --- |
| `Context:` | When the task has a context ID |
| `Status:` | Always: the state, then the status timestamp in RFC 3339 when present |
| `  <text>` | The status message, when present, indented two spaces |
| `Artifacts:` | When the task has artifacts. One line each: `[<name>]`, or `[<artifact ID>]` without a name, then the artifact's content |
| `History:` | When the task carries history. One line per message: `[user]` or `[agent]`, then the content |

A task in `input-required` or `auth-required` is followed by a command to reply with, after two blank lines:

```text
Task:     01a1d53b-390a-762e-bf38-be8aa98a2e7e
Context:  01a1d53b-390a-76d6-879b-6505620790be
Status:   input-required (2026-11-13T14:06:30Z)
  Permission required: git push (git push origin main).


Resume:     a2a send --task-id 01a1d53b-390a-762e-bf38-be8aa98a2e7e "<reply>"
```

A message is one line: `[agent] <content>` or `[user] <content>`.

A status update is `[status] <state>`, with `: <message content>` when the update carries a message:

```text
[status] working
[status] working: Running list files {"toolCall":{"id":"t1","kind":"read","status":"in_progress","title":"list files"}}
[status] completed
```

Artifact updates are printed inline, so a streamed answer reads as one line:

```text
[artifact] ...................Done.
```

A chunk that starts an artifact begins a `[artifact] ` line. Chunks that append to the open artifact continue the line. An appending chunk for an artifact that is not open begins a `[artifact+] ` line. The line ends at the chunk marked `lastChunk`, at the next other event, or at the end of the stream. See [Streaming and Timeouts](../advanced/streaming.md#streamed-artifacts-in-text-output).

States are printed in short form: `submitted`, `working`, `completed`, `failed`, `canceled`, `rejected`, `input-required`, `auth-required`. An unknown state is printed as received.

### Content

Parts are rendered as follows and separated by one space. In artifacts, adjacent text parts are joined without a space.

| Part | Rendering |
| --- | --- |
| Text | The text |
| Data | Compact JSON |
| File | `file:`, the file name if any, and in parentheses the media type, the size of inline bytes (`512 B`, `1.5 KiB`) and the URL, each when present |

```text
  [report] file: report.pdf (application/pdf, 1.2 MiB)
```

### JSON

Each event is a `StreamResponse` object with one key naming the event type:

| Key | Event |
| --- | --- |
| `task` | `Task` |
| `message` | `Message` |
| `statusUpdate` | `TaskStatusUpdateEvent` |
| `artifactUpdate` | `TaskArtifactUpdateEvent` |

`send -o json` prints one object:

```json
{
  "task": {
    "id": "01a1d53b-33fe-76cb-90e6-e3cc5550efc3",
    "artifacts": [
      {
        "artifactId": "01a1d53b-3404-7002-ba70-31706d3e6788",
        "name": "response",
        "parts": [
          {
            "text": "Hello from fakeacp."
          }
        ]
      }
    ],
    "contextId": "01a1d53b-33fe-7a05-b4e1-9ac1c2950b7f",
    "status": {
      "state": "TASK_STATE_COMPLETED",
      "timestamp": "2026-11-13T14:06:29.380Z"
    }
  }
}
```

`send --stream -o jsonl` prints one object per line (some artifact chunks removed here). Each artifact chunk is its own `artifactUpdate`; chunks after the first have `"append": true`.

```text
{"task":{"id":"01a1d53b-4c96-7952-899f-058437d311db","contextId":"01a1d53b-4c96-7a87-af05-9decff36d846","history":[{"messageId":"01a1d53b-4c5d-718c-b561-6af7fc590b50","parts":[{"text":"slow 1s"}],"role":"ROLE_USER"}],"status":{"state":"TASK_STATE_SUBMITTED"}}}
{"statusUpdate":{"contextId":"01a1d53b-4c96-7a87-af05-9decff36d846","status":{"state":"TASK_STATE_WORKING","timestamp":"2026-11-13T14:06:35.674Z"},"taskId":"01a1d53b-4c96-7952-899f-058437d311db"}}
{"artifactUpdate":{"artifact":{"artifactId":"01a1d53b-4ccf-7379-8b8e-98ec6eef7857","name":"response","parts":[{"text":"."}]},"contextId":"01a1d53b-4c96-7a87-af05-9decff36d846","taskId":"01a1d53b-4c96-7952-899f-058437d311db"}}
{"artifactUpdate":{"append":true,"artifact":{"artifactId":"01a1d53b-4ccf-7379-8b8e-98ec6eef7857","parts":[{"text":".Done."}]},"contextId":"01a1d53b-4c96-7a87-af05-9decff36d846","taskId":"01a1d53b-4c96-7952-899f-058437d311db"}}
{"statusUpdate":{"contextId":"01a1d53b-4c96-7a87-af05-9decff36d846","status":{"state":"TASK_STATE_COMPLETED","timestamp":"2026-11-13T14:06:36.678Z"},"taskId":"01a1d53b-4c96-7952-899f-058437d311db"}}
```

JSON uses the protocol names: states are `TASK_STATE_COMPLETED`, `TASK_STATE_INPUT_REQUIRED` and so on, and roles are `ROLE_USER` and `ROLE_AGENT`. Some string values escape `<`, `>` and `&` as `\u003c`, `\u003e` and `\u0026`; JSON parsers decode them.

## Task list

`task list` prints a table, and the token for the next page when there is one:

```text
ID                                    STATUS          CONTEXT
01a1d53b-4c96-7952-899f-058437d311db  completed       01a1d53b-4c96-7a87-af05-9decff36d846
01a1d53b-390a-762e-bf38-be8aa98a2e7e  input-required  01a1d53b-390a-76d6-879b-6505620790be
01a1d53b-38ac-7ddf-a3be-a6233258b315  completed       01a1d53b-38ac-7e79-b42a-d5a614d94163

Next page token: MTc5NDU3ODc5MDU4Mzc2NjIxM3wwMWExZDUzYi0zOGFjLTdkZGYtYTNiZS1hNjIzMzI1OGIzMTU
```

With `-o json` it prints the `ListTasksResponse`:

```json
{
  "tasks": [
    {
      "id": "01a1d53b-4c96-7952-899f-058437d311db",
      "contextId": "01a1d53b-4c96-7a87-af05-9decff36d846",
      "status": {
        "state": "TASK_STATE_COMPLETED",
        "timestamp": "2026-11-13T14:06:36.678Z"
      }
    }
  ],
  "totalSize": 6,
  "pageSize": 1,
  "nextPageToken": "MTc5NDU3ODc5NjY4MDg1MDYzNHwwMWExZDUzYi00Yzk2LTc5NTItODk5Zi0wNTg0MzdkMzExZGI"
}
```

## Other commands

| Command | JSON |
| --- | --- |
| `task push-config create`, `task push-config get` | The `PushConfig` object |
| `task push-config list` | Array of `PushConfig` objects |
| `task push-config delete` | `{"deleted": true, "taskId": ..., "id": ...}` |
| `config show` | Array of `{"name", "envVar", "value", "source", "path"}`; `path` only for file sources |
| `auth get` | `{"url", "helper", "headers", "expires"}`; header values are `<redacted>` without `--reveal` |
| `plugin list` | `{"enabled": bool, "plugins": [{"name", "version", "description", "path", "error"}]}` |
| `plugin set-enabled` | `{"plugins-enabled": bool, "path": ...}` |
| `transport list` | Array of `{"name", "version", "protocol", "description", "path", "error"}` |
| `version` | `{"version", "commit", "date", "goVersion", "platform"}`, with `-o json` only |

Empty optional fields are omitted. Text output of push configs shows `--auth-credentials` as `<redacted>`; JSON output includes it.

`chat` prints events like `send`, to stdout, and its prompt and session messages to stderr.
