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

These apply to every client-mode command. Each command selects the agent it talks to with either `--agent-card` (resolve a card) or `--endpoint` (connect to an interface directly).

| Flag | Short | Description |
|---|---|---|
| `--agent-card <ref>` | `-a` | Agent Card reference: a host/origin (the well-known path is appended), a full card URL, or a local file path. The card is resolved and a transport negotiated. |
| `--endpoint <ref>` | `-e` | Agent interface URL for a direct connection, skipping card resolution. Must be paired with exactly one `--transport`. Mutually exclusive with `--agent-card`. |
| `--transport <name>` | | Transport preference: `rest`, `jsonrpc`, `grpc`. Repeatable and ordered (highest preference first). With `--agent-card` it overrides the card's preference order; with `--endpoint` exactly one is required. |
| `--insecure` | | Use plaintext for the built-in gRPC client. By default it uses TLS and verifies the server certificate against the system trust store, or the certificates in `--ca-cert`. |
| `--ca-cert <file>` | | Trust only the CA certificates in the PEM file `<file>` for TLS. Applies to the card request and the REST, JSON-RPC and gRPC transports. Not read from the local `.env`. |
| `--output <fmt>` | `-o` | Output format: `text` (default), `json` (indented), or `jsonl` (one compact JSON object per line). |
| `--svc-param <k=v>` | | Service parameter (repeatable). The chosen transport defines how it's passed. Split on the first `=`. |
| `--auth <creds>` | | Shorthand for `--svc-param "Authorization=<creds>"`. |
| `--credential-helper <spec>` | | Credential helper returning the headers for an agent: `[scope=]path`. Repeatable. Not read from the local `.env`. See [Credentials](#credentials). |
| `--unix-socket <path>` | | Send every HTTP request and gRPC connection through the Unix socket at `<path>`, whatever the URL's host. Applies to the card request and the REST, JSON-RPC and gRPC transports. Not read from the local `.env`. |
| `--oauth <mode>` | | OAuth sign-in for agents that require it: `auto` (default), `cached`, or `off`. See [OAuth](#oauth). |
| `--oauth-clients <json>` | | OAuth client registration per authorization server. Repeatable. Not read from the local `.env`. |
| `--oauth-default-client-id <id>` | | OAuth client ID for authorization servers that support Client ID Metadata Documents when no registration matches; empty disables it. Not read from the local `.env`. |
| `--oauth-store <store>` | | Where OAuth grants are kept: `file` (default) or `keychain`. Not read from the local `.env`. |
| `--tenant <id>` | | Tenant identifier. Passed on every request. |
| `--timeout <dur>` | | Request timeout. Default `30s`. |
| `--verbose` | `-v` | Verbose output to stderr. |
| `--config <path>` | | Load configuration from an explicit file (`.yaml`, `.json`, or `.env`) in place of the local `.env`. |

---

## Configuration

Every global default can be set from the environment, an explicit configuration file (`.yaml`, `.json`), a `.env` file, or a persistent user-level `config.yaml`. This keeps repeated invocations short.

### YAML and JSON Configuration

Pass `--config <path>` pointing to a `.yaml`, `.yml`, or `.json` file to set global flags using standard kebab-case flag names and native data types (strings, lists, booleans):

**YAML (`a2a.yaml`):**
```yaml
agent-card: https://agent.example.com
transport:
  - rest
  - jsonrpc
timeout: 60s
verbose: false
insecure: false
tenant: my-team
svc-param:
  - "Authorization=Bearer <token>"
  - "X-Client-ID=cli-app"
```

**JSON (`a2a.json`):**
```json
{
  "agent-card": "https://agent.example.com",
  "transport": ["rest", "jsonrpc"],
  "timeout": "60s",
  "verbose": false,
  "insecure": false,
  "tenant": "my-team",
  "svc-param": [
    "Authorization=Bearer <token>",
    "X-Client-ID=cli-app"
  ]
}
```

Additional or unknown properties in YAML and JSON files are safely ignored.

### Environment Variables and `.env` Files

When using environment variables or `.env` files, settings are named `A2ACLI_` followed by the long flag name upper-snake-cased: `--agent-card` $\rightarrow$ `A2ACLI_AGENT_CARD`, `--timeout` $\rightarrow$ `A2ACLI_TIMEOUT`, `--transport` $\rightarrow$ `A2ACLI_TRANSPORT` (a comma-separated list).

`.env` files use `KEY=value`, one per line. Blank lines and `#` comments are ignored, a leading `export ` is tolerated, and surrounding quotes are stripped:

```dotenv
A2ACLI_AGENT_CARD=https://agent.example.com
A2ACLI_TRANSPORT=rest,jsonrpc
A2ACLI_TIMEOUT=60s
# credentials
A2ACLI_AUTH="Bearer <token>"
```

### User-level config file

A persistent user-level config lives at `~/.config/a2a-cli/config.yaml`. It uses the same kebab-case flag names and native YAML types as a `--config` file, but applies to every invocation without being passed explicitly. It is also where CLI-managed settings are stored — for example `plugins-enabled`, written by `a2a plugin set-enabled` (see [Command Plugins](#command-plugins)).

```yaml
plugins-enabled: true
tenant: my-team
```

### Precedence

When the same setting is defined in multiple places, the first match wins:

1. an explicit command-line flag,
2. a session environment variable (`A2ACLI_*`),
3. a local configuration file (the file named by `--config`, or the nearest `.env` found by walking up from the working directory),
4. the user-level `config.yaml` (default `~/.config/a2a-cli/config.yaml`),
5. the global `.env` at `$XDG_CONFIG_HOME/a2a-cli/.env` (default `~/.config/a2a-cli/.env`),
6. the built-in flag default.

`--stream`, `--help`, `--version`, and `--config` are never read from configuration files or the environment and must be passed explicitly.

Store secrets only in files you keep private.

### `config show` - Inspect configuration

Read-only: print each effective setting and the source it resolved from. Credential values are redacted.

```bash
a2a config show
a2a config show -o json
a2a --config ./a2a.yaml config show
a2a --config ./prod.env config show
```

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
one or more **part flags** (`--text-part`, `--file-part`, `--data-part`), which
are repeatable and preserve the order they appear on the command line. A trailing
positional argument is shorthand for a single `--text-part` part.

```bash
# Simple text message
a2a send -a <url> "Hello, what can you do?"

# Connect directly to an interface, skipping card resolution
a2a send -e <url> --transport rest "Hello, what can you do?"

# Multiple ordered parts: text, an inlined local file, and a referenced URL
a2a send -a <url> --text-part "Review this" --file-part report.pdf --media-type application/pdf --file-part https://example.com/spec.pdf

# A structured data part from a JSON file, or an inline JSON string
a2a send -a <url> --data-part payload.json
a2a send -a <url> --data-part '{"priority":"high"}'

# Streaming response - events printed as they arrive
a2a send -a <url> --stream "Summarize this document"

# Fire-and-forget - get the task ID back immediately
a2a send -a <url> --async "Start a long analysis"

# Full SendMessageRequest as JSON (inline string or a file path)
a2a send -a <url> --request-payload '{"message":{"role":"ROLE_USER","parts":[{"text":"analyze this"}]}}'
a2a send -a <url> --request-payload request.json

# Continue a conversation (same task)
a2a send -a <url> --task-id <task-id> "Follow-up question"

# Group under a context (new task, shared context)
a2a send -a <url> --context-id <context-id> "Related question"
```

| Flag | Description |
|---|---|
| `--text-part <string>` | Add a text part. Repeatable, order-preserving. |
| `--file-part <path\|url>` | Add a file part. A local path is inlined as bytes; a URL is carried by reference (never fetched by the CLI). Repeatable. |
| `--data-part <path\|json>` | Add a structured JSON data part, read from a file path or parsed from an inline JSON string. Repeatable. |
| `--media-type <type>` | Media type for the part flag immediately preceding it. Usage error if it follows no part flag. |
| `--request-payload <path\|json>` | Full `SendMessageRequest` as a JSON file path or inline JSON string. Mutually exclusive with the message-building flags (part flags, positional text, `--task-id`, `--context-id`, `--metadata`, `--history`, `--async`). |
| `--stream` | Use `SendStreamingMessage`. Events are printed incrementally. Falls back to polling if the server does not support streaming. |
| `--async` | Return immediately (fire-and-forget) instead of waiting for completion. |
| `--task-id <id>` | Continue an existing task. |
| `--context-id <id>` | Group this turn under an existing context (new task). |
| `--history <n>` | Request `n` history messages in the response. |

### `chat` - Chat with an Agent

Hold a back-and-forth conversation with one agent. Each message continues the
same conversation: the context id is carried across turns, and a task waiting on
`INPUT_REQUIRED` is continued with your next message. A failed turn prints the
error and the session stays open. An `AUTH_REQUIRED` task ends the session with an
explanation. Ctrl-C cancels a running turn, asks the agent to cancel the task it
started, and returns to the prompt; a second Ctrl-C abandons that cancel request.
A turn that exceeds `--timeout` also cancels its task. Type `/exit` or `/quit`,
press Ctrl-D, or press Ctrl-C at the prompt to exit; the context and task ids are printed so you can resume with
`send` or `chat`.

```bash
a2a chat -a <url>
a2a chat -a <url> "Hello, what can you do?"

# Stream each reply as it arrives
a2a chat -a <url> --stream

# Resume a conversation (same task)
a2a chat -a <url> --context-id <context-id> --task-id <task-id>
```

| Flag | Description |
|---|---|
| `--stream` | Use `SendStreamingMessage` for each turn. Events are printed incrementally. Falls back to polling if the server does not support streaming. |
| `--task-id <id>` | Continue an existing task with the first message. |
| `--context-id <id>` | Start the conversation in an existing context. |

`chat` requires a terminal on standard input and output; otherwise it is a usage
error. Use `send` to send a single message from a script.

### `task get` - Get Task Details

```bash
a2a task get -a <url> <id>
a2a task get -a <url> <id> --history 10
a2a task get -a <url> <id> -o json

# Follow a task started with `send --async` until it finishes
a2a task get -a <url> <id> --wait
a2a task get -a <url> <id> --wait --poll-interval 2s --timeout 60s
```

| Flag | Description |
|---|---|
| `--history <n>` | Include up to `n` history messages. |
| `--wait` | Poll until the task reaches a terminal (`completed`/`failed`/`canceled`/`rejected`) state, returning early on an interrupted (`input-required`/`auth-required`) state so the caller can act. |
| `--poll-interval <duration>` | Delay between polls while waiting (default `2s`); only used with `--wait`. |

The overall wait budget is the global `--timeout` (default `30s`); when it expires before the task settles, the command reports a timeout error and exits non-zero.

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

## Credentials

A credential helper is a program that returns the HTTP headers to send to an agent. a2a implements the [Bazel credential helper protocol](https://github.com/EngFlow/credential-helper-spec), so helpers written for Bazel work unchanged: a2a runs `<command> get`, writes `{"uri": "<agent URL>"}` to its stdin, and reads `{"headers": {"Authorization": ["Bearer …"]}, "expires": "<RFC 3339>"}` from its stdout.

List helpers under `credential-helper` in a config file:

```yaml
# ~/.config/a2a-cli/config.yaml
credential-helper:
  - command: [a2a-serve, credential]     # every host
  - scope: "*.corp.example.com"
    command: [/usr/local/bin/corp-helper]
```

`--credential-helper` and `A2ACLI_CREDENTIAL_HELPER` take `[scope=]path`, like Bazel's `--credential_helper`: a path with no arguments. A scope is a host without a port (`example.com`), or `*.example.com` for the domain and its subdomains. Helpers are chosen per request: the card request uses the helper for the card reference as given (`--agent-card` or the positional URL), and A2A calls use the helper for the interface URL the client connects to. That URL is also the `uri` the helper receives. Unlike Bazel, which sends the full URL of each request, a2a does not append the `/.well-known/agent-card.json` path or the A2A method path. The helper with the most specific matching scope runs; among equal scopes the last one wins. Its headers are sent unless `--auth` or `--svc-param` set the same header. a2a runs a helper once per URL and reuses its headers until they expire. `--timeout` bounds each run. A helper that exits non-zero fails the command; a helper with no credentials for the URL prints `{}`.

a2a executes the configured helper command, so `credential-helper`, `--unix-socket` and `--ca-cert` are not read from the local `.env` found by walking up from the current directory: a `.env` in a cloned repository could otherwise run a command, redirect requests or choose the certificates a2a trusts. a2a ignores them there with a warning. Set them with the flag, the environment, `~/.config/a2a-cli/config.yaml`, or a file passed with `--config`.

### OAuth

When an agent requires OAuth, a2a signs you in, stores the grant and attaches it to later calls, so you don't fetch and paste tokens.

```bash
a2a send -a https://agent.example.com "hello"                     # prompts once, saves the grant, sends
a2a send -a https://agent.example.com "again"                     # reuses the grant, refreshing it when needed
a2a auth login https://agent.example.com --device                 # sign in ahead of time, no browser needed
a2a send -a https://agent.example.com --oauth cached "from CI"    # never prompts; fails if no grant is stored
```

| Flag | Values | Description |
|---|---|---|
| `--oauth` | `auto` (default), `cached`, `off` | `auto` signs in when an agent requires it and stderr is a terminal; without a terminal it fails like `cached`. `cached` only uses stored grants and otherwise fails with `A2ACLI_ERR_CREDENTIALS_MISSING` (exit 4). `off` disables OAuth. |
| `--oauth-clients` | JSON object or array | Client registrations, one per authorization server: `{"authority":"<issuer>","client_id":"<client id or CIMD URL>","scopes":[...]}` on the command line and in `A2ACLI_OAUTH_CLIENTS`; a list of such objects in config files. Not read from the local `.env`. See [Client registrations](#client-registrations). |
| `--oauth-default-client-id` | Client ID or CIMD URL, empty to disable | Client ID used with servers that support Client ID Metadata Documents when no registration matches. Default `https://a2a-protocol.org/oauth/a2a-cli.json`. Not read from the local `.env`. |
| `--oauth-store` | `file` (default), `keychain` | `file` keeps grants, tokens included, in `~/.config/a2a-cli/oauth/grants.json` (mode 0600). `keychain` keeps the tokens in the macOS Keychain, Windows Credential Manager or Linux Secret Service, one entry per token, and the rest of each grant in `keychain-grants.json`; the two stores keep separate grants. Without a keychain, or when a token is too large for it, saving a grant fails rather than writing the tokens to disk. Not read from the local `.env`. |

#### Where a2a learns how to sign in

| Source | When | What a2a reads |
|---|---|---|
| Agent card | Before the first request | An `oauth2` scheme: its `oauth2MetadataUrl` ([RFC 8414](https://www.rfc-editor.org/rfc/rfc8414) metadata), or inline `authorizationCode` / `deviceCode` endpoints. An `openIdConnect` scheme: its discovery document. The first `securityRequirements` entry made of one such scheme is used, with its scopes. |
| `401` with a `Bearer` challenge | `--endpoint`, a protected card, or a card that declares no OAuth | `resource_metadata`, or the well-known location, for the Protected Resource Metadata ([RFC 9728](https://www.rfc-editor.org/rfc/rfc9728)); redirects stay on the same origin. Then the first listed authorization server whose metadata validates and that has a usable client. |

OAuth is enforced only when the agent card requires it. A card that also accepts other credentials, such as an API key, or an agent that declares nothing, gets a stored grant when one applies, and the request is sent either way.

#### Sign-in

| Flow | Used when | What you see |
|---|---|---|
| Authorization code with PKCE ([RFC 7636](https://www.rfc-editor.org/rfc/rfc7636), [RFC 8252](https://www.rfc-editor.org/rfc/rfc8252)) | Default | A URL to open; a2a waits for the redirect on `http://127.0.0.1:<port>/oauth/callback`. |
| Device authorization ([RFC 8628](https://www.rfc-editor.org/rfc/rfc8628)) | `--device`, or the server has no authorization endpoint | A URL and a code to enter on any device. |

Tokens are only sent over `https`, or `http` to a loopback host. Every request to the authorization server carries the agent URL as `resource` ([RFC 8707](https://www.rfc-editor.org/rfc/rfc8707)), so the token only works for that agent. These requests go through the same transport as agent requests, so `--unix-socket` and `--ca-cert` apply to them. Signing in has its own 5-minute limit; `--timeout` covers the agent requests, which start after it. If signing in fails or the agent still rejects the token, the command fails with `A2ACLI_ERR_AUTH_FAILED` (exit 4).

Grants are stored per issuer, client ID, agent URL and scopes in `~/.config/a2a-cli/oauth/grants.json` (mode 0600), or in the OS keychain with an index in `keychain-grants.json` under `--oauth-store keychain`. A grant also serves agents below its URL, and one obtained with more scopes than an agent asks for is reused. a2a refreshes a token shortly before it expires, or once when the agent rejects it with `error="invalid_token"`, and retries the request once.

#### Client registrations

The authorization server only talks to clients it knows. Configure one registration per authorization server, not per agent:

```yaml
# ~/.config/a2a-cli/config.yaml
oauth-clients:
  - authority: https://login.example.com
    client_id: 0f1e2d3c
    scopes: [a2a.invoke]          # optional; used when the agent names none
```

When no registration matches and the server advertises `client_id_metadata_document_supported`, a2a uses the [Client ID Metadata Document](https://datatracker.ietf.org/doc/draft-ietf-oauth-client-id-metadata-document/) `https://a2a-protocol.org/oauth/a2a-cli.json` as its client ID, so no registration is needed. Set `--oauth-default-client-id` to use another document, or to an empty value to turn this off.

#### Precedence

`--auth` and `--svc-param Authorization=…` win over a credential helper, which wins over OAuth. OAuth leaves alone any request that already carries an `Authorization` header.

### `auth` - Manage credentials

| Command | Flags | Description |
|---|---|---|
| `auth login <agent>` | `--device`, `--scope <scope>` (repeatable) | Sign in and store the grant without sending a message. |
| `auth list` | | Stored grants: resource, issuer, client, scopes and expiry. |
| `auth get <agent>` | `--reveal` | Credential helper headers and the stored grants that apply to the agent, redacted unless `--reveal`. |
| `auth logout <agent>` | `--all` | Revoke the grants that apply to the agent at the authorization server when it supports [RFC 7009](https://www.rfc-editor.org/rfc/rfc7009), then delete them locally. Other agents on the same host keep theirs. |

```bash
a2a auth login https://agent.example.com --scope a2a.invoke
a2a auth list
a2a auth get https://agent.example.com --reveal
a2a auth logout --all
```

A script or a sandboxed agent can sign in once with `a2a auth login` and then run with `--oauth cached`. Both have to use the same `~/.config/a2a-cli` and `--oauth-store`, so a sandbox with its own home directory doesn't see a grant stored outside it.

---

## Server Mode

`a2a server` starts an A2A-compliant server backed by one of three modes.

### Common Server Flags

| Flag | Description |
|---|---|
| `--port <n>` | Listen port. Default `8080`. |
| `--host <addr>` | Bind address. Default `127.0.0.1`. |
| `--name <name>` | Agent name for the auto-generated card. |
| `--description <desc>` | Agent description. |
| `--serve-transport <proto>` | Transport to serve: `rest` (default), `jsonrpc`, `grpc`. |
| `--advertise-address <addr>` | Endpoint advertised in the served agent card. Defaults to the listener address. |
| `--insecure` | Use insecure (plaintext) gRPC transport credentials (proxy upstream). |
| `--protocol <ver>` | Protocol version: `latest` (default), `0.3`. When set to `0.3`, the server uses the compat transport layer (`a2agrpc/v0` for gRPC, `a2acompat/a2av0` for REST/JSON-RPC) to accept v0.3 clients. |
| `--card <file>` | Serve a custom agent card JSON instead of auto-generating. |
| `--card-compat` | Serve the agent card in a dual v0.3/v1.0 format so both old and new clients can discover the agent. |
| `--quiet` | Suppress traffic logging to stderr. |

### `--echo` - Echo Mode

```bash
a2a server --echo
a2a server --echo --port 9090 --name "Echo Agent"
```
Returns the user's message text as the agent response. A "ping" for agents — useful for connectivity testing and client development.

### `--proxy` - Proxy Mode

Forward all requests to an upstream A2A agent. Logs traffic to stderr. Useful for debugging agent interactions, injecting service parameters, or acting as an authenticated gateway. `--proxy` is a boolean flag; the upstream agent is selected with the global `-a, --agent-card` or `-e, --endpoint` flags.

```bash
# Basic proxy with traffic logging
a2a server --proxy -a https://upstream-agent.com

# Inject auth for an upstream that requires it
a2a server --proxy -a https://upstream-agent.com --auth "<token>"

# Add tracing headers
a2a server --proxy -a https://upstream-agent.com \
  --svc-param "X-Request-Source=a2a-proxy" \
  --svc-param "X-Trace-ID=debug-session-1"
```

The proxy creates an `a2aclient.Client` for the upstream agent and forwards each A2A operation. It injects any `--svc-param` service parameters into every forwarded request via `a2aclient.AttachServiceParams`, and derives its own agent card from the upstream card, substituting the local interface address.

### `--exec` - Exec Mode

Run any command as an A2A agent. Message text goes to stdin, stdout becomes the response artifact. The subprocess does not need to know anything about A2A.
```bash
a2a server --exec "python -u a2a_unaware_agent.py"
a2a server --exec "./content-generator.sh"
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
a2a server --exec "for i in 1 2 3; do echo \$i; sleep 0.5; done" --chunk=$'\n'

# Space-delimited chunks
a2a server --exec "echo 'alpha beta gamma'" --chunk=' '

# Paragraph-level chunks
a2a server --exec "cat essay.txt" --chunk=$'\n\n'
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

All commands support machine-readable output, emitting raw protocol objects:

- `-o json` — indented JSON: a single indented document, or one indented record per event under `--stream`.
- `-o jsonl` — [JSON Lines](https://jsonlines.org/): one compact JSON object per line, ideal for piping and incremental consumption under `--stream`.

Text mode is the default, meant for reading in a terminal. The output format controls
only presentation (indentation); `--stream` independently controls whether the command
follows the agent's live events or waits for the terminal result.

## Command Plugins

The CLI can be extended with new top-level commands by installing an
`a2a-<name>` binary on your `PATH`. Discovery is **opt-in** and disabled by
default — plugins are only scanned and registered when command plugins are
enabled, so an untrusted binary on your `PATH` is never exec'd implicitly.

```console
# Enable discovery (writes plugins-enabled: true to ~/.config/a2a-cli/config.yaml)
$ a2a plugin set-enabled true

# List discovered plugins along with the current enabled/disabled state
$ a2a plugin list

# Disable again
$ a2a plugin set-enabled false
```

The `plugins-enabled` flag resolves through the same configuration chain as every
other setting, so it can also be set through the environment `A2ACLI_PLUGINS_ENABLED=true`. 
See the **[command plugin guide](./docs/command-plugins.md)** for the plugin contract and
authoring details.

## Custom Transport Plugins

The CLI speaks JSON-RPC, REST and gRPC out of the box. Additional transport
bindings can be added **without recompiling** by dropping an `a2a-transport-<name>` 
binary on your `PATH`. The CLI launches the plugin as a local proxy that speaks a 
standard A2A binding and forwards to the custom protocol.

```console
$ a2a transport list
$ a2a send --transport slimrpc --endpoint slim://agents.example/agent "hello"
```

Authoring a plugin in Go is a few lines with the
[`devkit/clitransport`](./devkit/clitransport) package — you provide an
`a2aclient.Transport`, it produces a CLI-compatible plugin. See the
**[transport plugin guide](./docs/transport-plugins.md)** and the runnable
**[echo plugin example](./examples/a2a-transport-echo)**.