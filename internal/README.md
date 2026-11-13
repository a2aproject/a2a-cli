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
| `--agent-card <ref>` | `-a` | Agent Card reference: a host/origin (the well-known path is appended), a full card URL, a local file path, a [contact](#contacts) name, or the identifier of an agent in an [AI Catalog](#ai-catalogs-experimental). The card is resolved and a transport negotiated. |
| `--catalog <name\|url>` | | AI Catalog to look the `--agent-card` identifier up in: a registered catalog name, or a catalog URL or path. Experimental. |
| `--endpoint <ref>` | `-e` | Agent interface URL for a direct connection, skipping card resolution. Must be paired with exactly one `--transport`. Mutually exclusive with `--agent-card`. |
| `--transport <name>` | | Transport preference: `rest`, `jsonrpc`, `grpc`. Repeatable and ordered (highest preference first). With `--agent-card` it overrides the card's preference order; with `--endpoint` exactly one is required. |
| `--insecure` | | Use plaintext for the built-in gRPC client. By default it uses TLS and verifies the server certificate against the system trust store, or the certificates in `--ca-cert`. |
| `--ca-cert <file>` | | Trust only the CA certificates in the PEM file `<file>` for TLS. Applies to the card request and the REST, JSON-RPC and gRPC transports. Not read from the local `.env`. |
| `--output <fmt>` | `-o` | Output format: `text` (default), `json` (indented), or `jsonl` (one compact JSON object per line). |
| `--svc-param <k=v>` | | Service parameter (repeatable). The chosen transport defines how it's passed. Split on the first `=`. |
| `--auth <creds>` | | Shorthand for `--svc-param "Authorization=<creds>"`. |
| `--credential-helper <spec>` | | Credential helper returning the headers for an agent: `[scope=]path`. Repeatable. Not read from the local `.env`. See [Credentials](#credentials). |
| `--unix-socket <path>` | | Send every HTTP request and gRPC connection through the Unix socket at `<path>`, whatever the URL's host. Applies to the card request and the REST, JSON-RPC and gRPC transports. Not read from the local `.env`. |
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

The card reference may also be supplied via the global `-a, --agent-card` flag instead of the positional argument. Like `--agent-card`, it may be the identifier of an agent in an [AI Catalog](#ai-catalogs-experimental).

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

## AI Catalogs (experimental)

An [AI Catalog](https://ai-catalog.io/) lists AI artifacts, A2A agents among them. `a2a` can register catalogs and resolve `--agent-card` from the agents they list, so you can refer to an agent by its catalog identifier instead of a card URL.

Registered catalogs live in the store shared with [ai-catalog-cli](https://github.com/Agent-Card/ai-catalog-cli): `~/.ai-catalog`, or the directory in `AI_CATALOG_CACHE_DIR`. A catalog registered with either tool is visible to both. The store layout is not part of the AI Catalog specification, so these commands are experimental, and `a2a` warns about and refuses to modify a store whose format it does not recognize.

```bash
a2a catalog add corp https://acme.com            # fetches https://acme.com/.well-known/ai-catalog.json
a2a catalog add local ./ai-catalog.json
a2a catalog list
a2a catalog search finance                       # A2A agents whose identifier, name, description or tags match
a2a catalog search --catalog https://acme.com/ai-catalog.json   # search a catalog without registering it
a2a catalog update [name]                        # fetch one or every registered catalog again
a2a catalog remove corp
```

`search` lists only A2A agents (entries of type `application/a2a-agent-card+json`). Catalogs nested in a catalog are followed up to four levels deep: any catalog may reference `https` URLs, and only a local catalog may reference local files.

Pass an agent's identifier to `--agent-card`:

```bash
a2a send -a urn:air:acme.com:agent:finance "..."                                              # across all registered catalogs
a2a send --catalog corp -a urn:air:acme.com:agent:finance "..."                               # in a registered catalog
a2a send --catalog https://acme.com/ai-catalog.json -a urn:air:acme.com:agent:finance "..."   # in an unregistered catalog
a2a send -a https://acme.com/finance-catalog.json "..."                                       # a catalog with a single A2A agent
```

- A `urn:` value of `--agent-card` is an identifier, unless a file with that name exists. With `--catalog`, any value is an identifier.
- An identifier of a [contact](#contacts) resolves to the contact before the registered catalogs are searched.
- Lookups in registered catalogs read only the local store; run `a2a catalog update` to pick up changes.
- An identifier listed by more than one catalog is an error; pick one with `--catalog`.
- When a catalog lists several versions of an agent, the latest is used.
- A URL passed to `--agent-card` that serves a catalog instead of a card resolves only when the catalog lists exactly one A2A agent; otherwise the error lists the agents.

---

## Contacts

The contact book stores named agents you trust. It is an AI Catalog at `~/.config/a2a-cli/catalog.json` that only `a2a` writes, registered in the shared [AI Catalog store](#ai-catalogs-experimental) as `a2a-contacts`, so `ai-catalog search` and `a2a catalog search` see your contacts too. Other tools keep their own curated catalogs and register them the same way.

```bash
a2a contact add weather https://weather.example.com          # any --agent-card reference
a2a contact add fin urn:air:acme.com:agent:finance --note "Expense reports"
a2a contact add travel --catalog https://acme.com/ai-catalog.json urn:air:acme.com:agent:travel
a2a contact list                 # no requests are made
a2a contact get fin              # contact details and its card (--refresh to fetch it again)
a2a send -a fin "Where is report R-2041?"
a2a contact remove fin
```

- A contact name can be passed to `--agent-card` (`-a`) or `card get` in place of a card reference. Names use lowercase letters, digits, `-` and `_`, so they never look like a URL, path or identifier. A name can match a single-label host such as `agentbox`; the contact takes precedence, and `-a https://agentbox` reaches the host.
- If the contact book can't be read, `-a` prints a warning and resolves the reference as if there were no contacts.
- An agent from a catalog is copied with its identifier, publisher and trust manifest, so its signature can still be verified. A relative card URL is made absolute, unless the trust manifest signs it; then it is kept as listed and resolved against the catalog it was added from. An agent added by its card gets the identifier `urn:ai-catalog:a2a-cli:<name>`.
- A contact's identifier takes precedence over the same identifier in registered catalogs, so `-a <identifier>` resolves to the contact.
- Two contacts can't share an identifier, even with different versions. Pass `--replace` to overwrite a contact of the same name.
- `contact add` fetches the card first and fails if it can't, using the same credentials, `--svc-param` headers and `--ca-cert` as `card get`. Fetched cards are cached under the user cache directory (`~/.cache/a2a-cli` on Linux) for 24 hours. An older card is fetched again by `contact get` and `-a <contact>`; until then the hook describes the contact from its catalog entry.
- If the AI Catalog store can't be updated, the contact is still saved and `a2a` prints a warning.

### Agent harness hooks

`a2a hook` is a hidden command for agent harness hooks. It reads a hook event as JSON on stdin and prints the context to add in the Claude Code hook output format. The [Agent Plugin](../agent-plugin/) registers it for `SessionStart`, `UserPromptSubmit` and `PostToolBatch`.

- On `SessionStart` it lists your contacts as trusted agents. After a compaction it also lists the agents discovered earlier in the session.
- When the contact book changes during a session, the next event lists the contacts again.
- It probes the origins of URLs in prompts and in `WebFetch` and `WebSearch` calls (`discovery-scan-tools`) for an agent card. Agents it finds are listed as unverified, and the model is told to ask the user before delegating to them.
- Agents in registered AI Catalogs are not listed, since a catalog can list hundreds. The model finds them with `a2a catalog search`.

The hook itself makes no network requests. Origins without a cached card are probed by a detached `a2a hook probe` process, and agents it finds are reported by a later event. Each agent is reported once per session. Found cards are cached for 24 hours, and origins without a card for 6 hours. A connection failure, a timeout, a server error or a rate limit isn't cached, so the origin is probed again when it's next mentioned. A document at the well-known path without an agent name and interface doesn't count as a card, and neither does one larger than 512 KiB.

The hook never fails the harness event: an unreadable configuration file, contact book or event makes it print nothing, and `-v` shows why.

| Setting | Default | Description |
|---|---|---|
| `discovery` | `true` | Probe origins of URLs in the conversation. Set `false` (or `A2ACLI_DISCOVERY=false`) to keep only the contacts. |
| `discovery-allow-host` | | Hosts discovery may probe whatever address they resolve to, and reach by any redirect. |
| `discovery-scan-tools` | `WebFetch,WebSearch` | Tools whose calls are scanned for URLs. |

Discovery fetches URLs that only appeared in a conversation, so it limits where a probe can connect:

- It probes internal hosts (loopback, private and carrier-grade NAT addresses) as well as public ones.
- It never connects to link-local addresses, which covers the IPv4 cloud metadata endpoint `169.254.169.254`, or to unspecified or multicast addresses. Metadata endpoints at other addresses, such as AWS's IPv6 `fd00:ec2::254`, count as internal: they are probed when a URL names them directly, but a redirect from a public site can't reach them.
- It follows at most 3 redirects, and refuses a redirect from a public address to an internal one, so a public site can't point a probe into your network.

The checks run on the addresses a probe's host resolves to when it connects, so DNS rebinding can't reach a link-local address or turn a redirect from a public host into an internal one; a host with both public and internal addresses counts as public. Probes don't reuse connections, so every redirect hop is checked. Probes use the proxy from `HTTPS_PROXY`, `HTTP_PROXY` and `NO_PROXY`. A proxied target is checked against its address as resolved locally, and a name that doesn't resolve locally is treated as public. The proxy resolves the name again on its own, so for proxied requests the checks are best effort. A refused probe isn't cached, so a change to `discovery-allow-host` applies at once. `discovery-allow-host` is not read from a `.env` found by walking up from the working directory.

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

### `auth get` - Inspect credentials

```bash
a2a auth get https://agent.example.com            # header names, values redacted
a2a auth get https://agent.example.com --reveal   # header values
```

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