# Errors and Exit Codes

When a command fails, the CLI prints an error with a symbolic code, a message and often a hint, and exits with a status that says what kind of failure it was. A task that ends in a failed state is not a CLI failure. This page lists the error format, the exit codes and how scripts should check task results. The codes come from `internal/clierr`.

## Format

In text mode the error goes to stderr:

```text
Error: failed to resolve agent card: card request failed: Get "http://127.0.0.1:1/.well-known/agent-card.json": dial tcp 127.0.0.1:1: connect: connection refused
hint: check the agent is running and the --agent-card reference is correct
```

With `-o json` or `-o jsonl` the error goes to stdout as a JSON object, indented with `json` and on one line with `jsonl`:

```json
{
  "error": {
    "code": "A2ACLI_ERR_IO",
    "message": "failed to resolve agent card: card request failed: Get \"http://127.0.0.1:1/.well-known/agent-card.json\": dial tcp 127.0.0.1:1: connect: connection refused",
    "hint": "check the agent is running and the --agent-card reference is correct"
  }
}
```

| Field | Content |
| --- | --- |
| `code` | One of the codes below |
| `message` | One-line description. URLs from failed HTTP requests have their password masked and query string removed. |
| `hint` | What to try next. Omitted when there is none. |
| `a2aCode` | For `A2ACLI_ERR_PROTOCOL`, the A2A error reason the agent returned, such as `TASK_NOT_FOUND`. Omitted otherwise. |

The JSON form is used whenever `--output` or `A2ACLI_OUTPUT` asks for JSON, which includes most flag and configuration errors. Errors that occur before the output setting is read, such as a `.env` that fails to parse, are printed as text. In streaming commands, events printed before the failure stay on stdout, so the error object is the last record. Warnings and verbose logs always go to stderr.

## Exit codes

| Exit | Code | When |
| --- | --- | --- |
| 0 | | The command succeeded |
| 1 | `A2ACLI_ERR_PROTOCOL` | The agent returned an A2A error |
| 1 | `A2ACLI_ERR_INTERNAL` | Any failure with no better code, such as no compatible transport, a missing transport plugin, a credential helper that failed, or an invalid `.env` |
| 2 | `A2ACLI_ERR_USAGE` | Invalid flags, arguments, flag combinations or configuration values |
| 3 | `A2ACLI_ERR_IO` | The agent could not be reached or the connection broke |
| 4 | `A2ACLI_ERR_CARD_INVALID` | The card was reached but cannot be used |
| 5 | `A2ACLI_ERR_TIMEOUT` | `--timeout` expired |

### 1: protocol

The agent answered with an A2A error, such as an unknown task, a task that cannot be canceled, an unsupported operation or a rejected credential. `a2aCode` holds the reason:

```json
{
  "error": {
    "code": "A2ACLI_ERR_PROTOCOL",
    "message": "failed to get task nonexistent: task not found",
    "a2aCode": "TASK_NOT_FOUND"
  }
}
```

The reasons are `PARSE_ERROR`, `INVALID_REQUEST`, `METHOD_NOT_FOUND`, `INVALID_PARAMS`, `INTERNAL_ERROR`, `SERVER_ERROR`, `TASK_NOT_FOUND`, `TASK_NOT_CANCELABLE`, `PUSH_NOTIFICATION_NOT_SUPPORTED`, `UNSUPPORTED_OPERATION`, `CONTENT_TYPE_NOT_SUPPORTED`, `INVALID_AGENT_RESPONSE`, `EXTENDED_AGENT_CARD_NOT_CONFIGURED`, `EXTENSION_SUPPORT_REQUIRED`, `VERSION_NOT_SUPPORTED`, `UNAUTHENTICATED` and `UNAUTHORIZED`. An error the SDK cannot map is `INTERNAL_ERROR`. There is no hint.

### 1: internal

Everything the other codes don't cover, with no hint. Examples:

```text
Error: failed to create a client: no compatible transports found: available transports - [JSONRPC_1.0,HTTP+JSON_1.0]
Error: failed to create a client: no transport plugin "nosuch" found on PATH (expected a "a2a-transport-nosuch" binary); available plugins: [echo]
```

### 2: usage

The command line or configuration is wrong: an unknown flag or command, a missing `--agent-card` and `--endpoint`, `--endpoint` without exactly one `--transport`, `--stream` with `--async`, a card file that does not exist, an invalid `--output`, an unreadable `--config` or `--ca-cert`, or an invalid value from an `A2ACLI_*` variable or a file. `chat` outside a terminal is also a usage error.

```text
Error: either '--agent-card <ref>' or '--endpoint <url> --transport <t>' must be provided
hint: if you are an agent, your a2a skill may be out of date — run `a2a skill` for the current commands and update your local copy
```

Every usage error has that hint. It is written for coding agents that learned the commands from an older copy of the skill that `a2a skill` prints.

### 3: I/O

A network failure: DNS, connection refused or reset, TLS, broken pipe, unexpected end of a response.

| Where | Hint |
| --- | --- |
| Card request | `check the agent is running and the --agent-card reference is correct` |
| gRPC to a server that does not use TLS | `the agent may not use TLS; pass --insecure to allow plaintext gRPC connections` |
| Elsewhere | `check connectivity or whether the agent is running` |

### 4: card invalid

The card request got a response, but it was not `200 OK` or the body was not a card.

| Cause | Hint |
| --- | --- |
| 401 or 403 | `pass or check credentials (--auth, --svc-param or --credential-helper)` |
| 5xx | `the agent failed to serve its card, check its logs or retry` |
| Other status, or a body that does not parse | `check the --agent-card reference (host, full card URL, or local file path)` |

```text
Error: failed to resolve agent card: card request failed, status: 404 Not Found
hint: check the --agent-card reference (host, full card URL, or local file path)
```

A card file that exists but cannot be read or parsed is also exit 4. A card file that does not exist is a usage error.

### 5: timeout

`--timeout` expired: during the card request (hint `increase --timeout`), a request, a credential helper run, a `task get --wait`, or between two events of a stream.

```text
Error: streaming error: SSE stream error: no activity for 1s: context deadline exceeded
hint: increase --timeout, or start the task with --async and follow it later
```

See [Streaming and Timeouts](../advanced/streaming.md) for what `--timeout` bounds in each command.

## Task states and the exit code

The exit code reports whether the CLI got an answer, not what the answer was. A task that ends `failed`, `canceled` or `rejected`, or stops in `input-required` or `auth-required`, still exits 0. The same applies to `send --stream`, `task get --wait` and `task subscribe`.

Scripts that need the outcome should read the state from JSON output:

```bash
out=$(a2a send -a https://agent.example.com -o json "Summarize the report") || exit
state=$(jq -r '.task.status.state // empty' <<<"$out")
case "$state" in
  TASK_STATE_COMPLETED) jq -r '.task.artifacts[]?.parts[]?.text // empty' <<<"$out" ;;
  TASK_STATE_INPUT_REQUIRED) echo "agent needs input: $(jq -r '.task.id' <<<"$out")" >&2; exit 10 ;;
  "") jq -r '.message.parts[]?.text // empty' <<<"$out" ;;
  *) echo "task ended in $state" >&2; exit 11 ;;
esac
```

An agent may reply with a `message` in place of a task; the `""` case above handles it. For a stream, read the state from the last `task` or `statusUpdate` record:

```bash
a2a send -a https://agent.example.com --stream -o jsonl "Summarize the report" |
  jq -rs 'map(.task.status.state // .statusUpdate.status.state // empty) | last'
```

The [Scripting guide](../guide/scripting.md) has more patterns.
