# A2A CLI Tool

> [!WARNING]
> This repository is in an alpha stage.

A command-line client for developers to send messages to A2A agents and services and receive task updates.

## Install

### From source (recommended)

The easiest way to install `a2a` and keep it up to date:

```bash
go install github.com/a2aproject/a2a-cli@latest
```

If you're new to Go, follow https://go.dev/doc/install to set up your `PATH` correctly.

### Prebuilt binaries

Prebuilt binaries are available at the [latest release](https://github.com/a2aproject/a2a-cli/releases/latest).

Extract the archive and put the `a2a` binary on your `PATH`.

Verify the download against the `checksums.txt` published with the release.

## Global Flags

These apply to every client-mode command. Each command selects the agent it talks to with either `--agent-card` (resolve a card) or `--url` (connect to an interface directly).

| Flag | Short | Description |
|---|---|---|
| `--agent-card <ref>` | `-a` | Agent Card reference: a host/origin (the well-known path is appended), a full card URL, or a local file path. The card is resolved and a transport negotiated. |
| `--url <ref>` | `-u` | Agent interface URL for a direct connection, skipping card resolution. Must be paired with exactly one `--transport`. Mutually exclusive with `--agent-card`. |
| `--transport <name>` | | Transport preference: `rest`, `jsonrpc`, `grpc`. Repeatable and ordered (highest preference first). With `--agent-card` it overrides the card's preference order; with `--url` exactly one is required. |
| `--output <fmt>` | `-o` | Output format: `text` (default), `json`. |
| `--svc-param <k=v>` | | Service parameter (repeatable). The chosen transport defines how it's passed. Split on the first `=`. |
| `--auth <creds>` | | Shorthand for `--svc-param "Authorization=<creds>"`. |
| `--tenant <id>` | | Tenant identifier. Passed on every request. |
| `--timeout <dur>` | | Request timeout. Default `30s`. |
| `--verbose` | `-v` | Verbose output to stderr. |

---

## Client Commands

### `card get` - Agent Card Discovery

Fetch and display an agent card from its base URL or complete agent card URL.

```bash
a2a card get <url>
a2a card get <url> -o json
```
To fetch the extended card (if supported):

```bash
a2a card get <url> --extended --auth "Bearer <token>"
```

The card reference may also be supplied via the global `-a, --agent-card` flag instead of the positional argument.

### `send` - Send a Message

Send a message to an agent and print the response. Message content is built from
one or more **part flags** (`--text`, `--file`, `--data`), which are repeatable
and preserve the order they appear on the command line. A trailing positional
argument is shorthand for a single `--text` part.

```bash
# Simple text message
a2a send -a <url> "Hello, what can you do?"

# Connect directly to an interface, skipping card resolution
a2a send -u <url> --transport rest "Hello, what can you do?"

# Multiple ordered parts: text, an inlined local file, and a referenced URL
a2a send -a <url> --text "Review this" --file report.pdf --media-type application/pdf --file https://example.com/spec.pdf

# A structured data part from a JSON file (or '-' to read stdin)
a2a send -a <url> --data payload.json

# Streaming response - events printed as they arrive
a2a send -a <url> --stream "Summarize this document"

# Fire-and-forget - get the task ID back immediately
a2a send -a <url> --async "Start a long analysis"

# Full structured message as JSON (the Message object)
a2a send -a <url> --json '{"role":"ROLE_USER","parts":[{"text":"analyze this"}]}'

# Continue a conversation (same task)
a2a send -a <url> --task-id <task-id> "Follow-up question"

# Group under a context (new task, shared context)
a2a send -a <url> --context-id <context-id> "Related question"
```

| Flag | Description |
|---|---|
| `--text <string>` | Add a text part. Repeatable, order-preserving. |
| `--file <path\|url>` | Add a file part. A local path is inlined as bytes; a URL is carried by reference (never fetched by the CLI). Repeatable. |
| `--data <path\|->` | Add a structured JSON data part, read from a file or stdin (`-`). Repeatable. |
| `--media-type <type>` | Media type for the part flag immediately preceding it. Usage error if it follows no part flag. |
| `--json <body>` | Raw JSON `Message` object. Mutually exclusive with the part flags and positional text. |
| `--stream` | Use `SendStreamingMessage`. Events are printed incrementally. Falls back to polling if the server does not support streaming. |
| `--async` | Return immediately (fire-and-forget) instead of waiting for completion. |
| `--task-id <id>` | Continue an existing task. |
| `--context-id <id>` | Group this turn under an existing context (new task). |
| `--history <n>` | Request `n` history messages in the response. |

### `task get` - Get Task Details

```bash
a2a task get -a <url> <id>
a2a task get -a <url> <id> --history 10
a2a task get -a <url> <id> -o json
```

| Flag | Description |
|---|---|
| `--history <n>` | Include up to `n` history messages. |

### `task list` - List Tasks

```bash
a2a task list -a <url>
a2a task list -a <url> --context <ctx-id>
a2a task list -a <url> --status working
a2a task list -a <url> --limit 50
```

| Flag | Description |
|---|---|
| `--context <id>` | Filter by context ID. |
| `--status <state>` | Filter by task state. Accepts short forms: `submitted`, `working`, `completed`, `failed`, `canceled`, `rejected`, `input-required`, `auth-required`. |
| `--limit <n>` | Page size. |
| `--page-token <t>` | Pagination token from a previous response. |
| `--history <n>` | Include up to `n` history messages per task. |
| `--since <time>` | Only tasks with status updates after this timestamp (RFC 3339). |
| `--with-artifacts` | Include artifacts in the response. |

### `task cancel` - Cancel a Task

```bash
a2a task cancel -a <url> <task-id>
```

Prints the updated task status.

### `task subscribe` - Subscribe to Task Events

```bash
a2a task subscribe -a <url> <task-id>
```
Streams events to stdout until the task reaches a terminal state. Output format matches `send --stream`.

---

## Server Mode

`a2a serve` starts an A2A-compliant server backed by one of three modes.

### Common Server Flags

| Flag | Description |
|---|---|
| `--port <n>` | Listen port. Default `8080`. |
| `--host <addr>` | Bind address. Default `127.0.0.1`. |
| `--name <name>` | Agent name for the auto-generated card. |
| `--description <desc>` | Agent description. |
| `--transport <proto>` | Transport to serve: `rest` (default), `jsonrpc`, `grpc`. |
| `--protocol <ver>` | Protocol version: `latest` (default), `0.3`. When set to `0.3`, the server uses the compat transport layer (`a2agrpc/v0` for gRPC, `a2acompat/a2av0` for REST/JSON-RPC) to accept v0.3 clients. |
| `--card <file>` | Serve a custom agent card JSON instead of auto-generating. |
| `--card-compat` | Serve the agent card in a dual v0.3/v1.0 format so both old and new clients can discover the agent. |
| `--quiet` | Suppress traffic logging to stderr. |

### `--echo` - Echo Mode

```bash
a2a serve --echo
a2a serve --echo --port 9090 --name "Echo Agent"
```
Returns the user's message text as the agent response. A "ping" for agents — useful for connectivity testing and client development.

### `--proxy` - Proxy Mode

Forward all requests to an upstream A2A agent. Logs traffic to stderr. Useful for debugging agent interactions, injecting service parameters, or acting as an authenticated gateway.

```bash
# Basic proxy with traffic logging
a2a serve --proxy https://upstream-agent.com

# Inject auth for an upstream that requires it
a2a serve --proxy https://upstream-agent.com --auth "<token>"

# Add tracing headers
a2a serve --proxy https://upstream-agent.com \
  --svc-param "X-Request-Source=a2a-proxy" \
  --svc-param "X-Trace-ID=debug-session-1"
```

The proxy creates an `a2aclient.Client` for the upstream agent and forwards each A2A operation. It injects any `--svc-param` service parameters into every forwarded request via `a2aclient.AttachServiceParams`, and derives its own agent card from the upstream card, substituting the local interface address.

### `--exec` - Exec Mode

Run any command as an A2A agent. Message text goes to stdin, stdout becomes the response artifact. The subprocess does not need to know anything about A2A.
```bash
a2a serve --exec "python -u a2a_unaware_agent.py"
a2a serve --exec "./content-generator.sh"
```

#### Subprocess Interface

**stdin:** The first text part of the incoming A2A message.
**stdout:** Response content. Interpretation depends on whether `--chunk` is set (see below).
**stderr:** Logged by the CLI at debug level. On non-zero exit, stderr content is included in the failure status message.

**Exit code:**
- `0` → `TaskStateCompleted`
- Non-zero → `TaskStateFailed`

#### Output Modes

**Default (no `--chunk`):** The CLI collects all stdout and emits it as a single text artifact when the process exits.
```
Status: working → [process runs] → Artifact (full output) → Status: completed
```

**With `--chunk=<delimiter>`:** The CLI reads stdout incrementally and splits it on the delimiter. It streams each piece as an artifact chunk event (`Append: true`) as soon as it's available, so the subprocess never needs to know A2A's event model.

```bash
# Emit 3 chunks with 500ms delay - useful for verifying client streaming
a2a serve --exec "for i in 1 2 3; do echo \$i; sleep 0.5; done" --chunk=$'\n'

# Space-delimited chunks
a2a serve --exec "echo 'alpha beta gamma'" --chunk=' '

# Paragraph-level chunks
a2a serve --exec "cat essay.txt" --chunk=$'\n\n'
```

The event sequence with `--chunk`:

```
StatusUpdate:          working
ArtifactUpdateEvent:   {id: new, append: false, parts: ["1"]}
ArtifactUpdateEvent:   {id: same, append: true,  parts: ["2"]}
ArtifactUpdateEvent:   {id: same, append: true,  parts: ["3"], lastChunk: true}
StatusUpdate:          completed
```

---

## Output Formatting

All commands support `-o json` for machine-readable output, emitting raw protocol objects.
Text mode is the default, meant for reading in a terminal.
