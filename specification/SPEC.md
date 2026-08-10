# a2a-cli Specification

**Version:** 0.1
**Status:** Draft — open for review.
**Last updated:** 2026-08-10
**Applies to:** A2A Protocol v1.0

## Abstract

This document specifies the behavior that a command-line interface (CLI) tool MUST, SHOULD, and MAY exhibit to be considered a conformant **`a2a-cli`** — a terminal client for the [Agent2Agent (A2A) Protocol](https://a2a-protocol.org/latest/specification/). It exists so that independently built CLIs, in any language, converge on one predictable command surface, output contract, and conversation model — measurable through a published compliance report.

This is an **implementer's specification**. Its audience is engineers building or improving an `a2a-cli`. It constrains CLI behavior only and never modifies A2A wire semantics.

## Why this matters

Three problems, all traceable to the same cause: there is no official A2A command-line tool.

### 1. Everyone builds their own, and they build the same thing

At least seven independent A2A CLIs exist today, across six languages — Go, Rust, Python, TypeScript, .NET, and Swift. Two already sit inside the A2A project's own GitHub organization ([A2A#1929](https://github.com/a2aproject/A2A/issues/1929)).

They largely re-implement the same short list of operations: send a task, continue a multi-turn conversation, check task status, read artifacts. Those are exactly the operations an official tool should cover.

The request keeps recurring rather than resolving. An earlier CLI contribution was closed as out of scope for its repository ([PR#1323](https://github.com/a2aproject/A2A/pull/1323)); its follow-up issue was closed as a duplicate ([#1325](https://github.com/a2aproject/A2A/issues/1325)); a working command grammar was designed separately inside the Go SDK ([a2a-go#306](https://github.com/a2aproject/a2a-go/discussions/306)); and the consolidation request itself ([A2A#1929](https://github.com/a2aproject/A2A/issues/1929)) is open for community vote.

### 2. AI coding agents have no standard way in

Developers increasingly work through AI coding agents, and those agents drive tools from the terminal. An agent learns to use a command-line tool from a skill descriptor — so with no canonical CLI there can be no canonical skill file, and therefore no standard way for a coding agent to work with an A2A agent at all ([A2A#1929](https://github.com/a2aproject/A2A/issues/1929)).

Agents also consume output directly. They need predictable, machine-readable results and stable exit codes rather than a screen formatted for humans, which is why machine-readable output (§9.3) and a skill descriptor (§12) are core requirements here rather than optional extras.

### 3. Testing a running agent is harder than it should be

A2A already provides SDKs in several languages, a Technology Compatibility Kit, an inspector, and a sample repository. What is missing is a quick way to exercise a *running* agent from outside any one SDK.

A conformant CLI fills that gap. Because it is scriptable and emits machine-readable output, it doubles as a lightweight test harness: a shell script or a Python test can drive real conversations against a live agent, and a coding agent can do the same unattended. That lowers the cost of checking an A2A service — by hand during development, or automatically in CI — without writing client code first.

### What this document does about it

It defines the core client behavior — the operations above — so that every implementation can agree on one definition. Specialised needs can be built on top; the goal here is to get the common path right.

This is not an attempt to replace the existing tools. Any tool, in any language, can implement this specification and report exactly what it supports (§13).

## Notational conventions

The key words **MUST**, **MUST NOT**, **REQUIRED**, **SHALL**, **SHALL NOT**, **SHOULD**, **SHOULD NOT**, **RECOMMENDED**, **MAY**, and **OPTIONAL** are to be interpreted as described in [RFC 2119](https://www.rfc-editor.org/rfc/rfc2119).

References of the form "A2A §x" point to the A2A Protocol Specification v1.0. Where an A2A rule is load-bearing for the CLI it is restated here so this document is self-contained; the A2A specification remains authoritative for protocol semantics.

---

## 1. Scope

1.1 An `a2a-cli` is an **A2A client**: it initiates requests to an A2A server (a remote agent) and renders the responses. Acting *as* a server — publishing an Agent Card, generating server-side identifiers, or serving inbound requests — is **outside the baseline** of this specification (see §8.5, optional).

1.2 The primary purpose of an `a2a-cli` is **interaction with an A2A server**. A2A interactions MAY be multi-turn and MAY be stateful and spanning multiple CLI invocations. A conformant tool MUST therefore allow a caller to **start, continue, inspect, and resume** a conversation (§6), and MUST provide a **polling** path for task status in addition to any streaming support (§7).

1.3 Conformance is **tiered and evidence-based** (§3). A tool asserts conformance by publishing a compliance report. Any number of **conformant** tools MAY coexist; conformance is open to any implementation, in any language, that passes the specification.

1.4 **Conformant vs. official.** These are distinct:
- A **conformant** tool is any implementation that satisfies a tier of this specification and publishes a compliance report (§13). Conformance is open to all.
- An **official** tool is one the A2A project has designated as a project-maintained reference implementation, hosted under the A2A project's GitHub organization, demonstrated against the A2A TCK, and listed in the compatibility matrix (§13). "Official" denotes governance and demonstrated conformance — not exclusivity, and not a higher standard of conformance. Designation is a project decision made outside this specification (§15.3).

---

## 2. Object model

Restated from A2A §4 so this document stands alone:

- **Message** — a single conversational turn. Has a `role` (`user` or `agent`) and one or more **Parts** (`text`, `file`, or `data`). Carries a client-assigned **`messageId`**.
- **Task** — the stateful unit of work a message may create. Identified by a server-assigned **`taskId`**; advances through a **TaskState** (§7.1); may emit **Artifacts**.
- **Artifact** — a task output (text, structured data, or file). Task outputs are delivered as Artifacts, not Messages; a conformant tool MUST render Artifacts.
- **`contextId`** — a server-assigned, opaque identifier that groups related tasks and messages into a single **conversation**.
- **AgentCard** — the server's machine-readable description of identity, capabilities, interfaces (transports), security schemes, and skills, obtained during discovery (§8.1).

---

## 3. Conformance model

3.1 A tool declares conformance **per tier**. A tier is satisfied only when **every MUST** in that tier is satisfied. Tiers are cumulative: Tier 2 requires Tier 1; Tier 3 requires Tier 2.

| Tier | Name | Requirements |
| --- | --- | --- |
| **Tier 1** | Core | §4.5 default behavior · §6 conversation/session handling · §7 polling · §8.1–8.4 commands (`discover`, `send`, `get`, `cancel`) · §9 output & exit codes · §10.1 auth · §11 transport & versioning · §12 SKILL.md |
| **Tier 2** | Standard | Tier 1 + `list`, `subscribe`, OAuth `auth login`, ≥2 transports, config profiles, interactive `chat`, `download`, wire debug, `conformance`, shell completions |
| **Tier 3** | Advanced | Tier 2 + push-notification config CRUD and a webhook receiver, gRPC transport, authenticated extended Agent Card, Agent Card signature verification, mTLS, OpenID Connect, `serve`/mock mode, catalog/registry, extensions |

3.2 Conformance MUST be demonstrated by a **compliance report** (§13) generated against the A2A Technology Compatibility Kit (TCK) and this specification. A tool MUST NOT advertise a tier it has not demonstrated.

### 3.3 Requirement identifiers

Each testable requirement carries a stable identifier of the form:

```
A2ACLI_<AREA>_<NNN>
```

where `<AREA>` names the command or cross-cutting concern and `<NNN>` is a zero-padded sequence number within that area — for example `A2ACLI_SEND_002`, `A2ACLI_CONV_001`, `A2ACLI_OUT_003`.

Defined areas:

| Area | Covers | Area | Covers |
| --- | --- | --- | --- |
| `DEFAULT` | Default behavior (§4.5) | `OUT` | Output & error contract (§9.1–9.4) |
| `DISCOVER` | Agent Card discovery (§8.1) | `EXIT` | Exit codes (§9.6) |
| `SEND` | Sending messages (§8.2) | `AUTH` | Authentication (§10) |
| `GET` | Task retrieval (§8.3) | `TX` | Transport selection (§11.1) |
| `CANCEL` | Task cancellation (§8.4) | `VER` | Protocol versioning (§11.2, §11.3) |
| `LIST` | Task listing (§8.5) | `SKILL` | Agent skill descriptor (§12) |
| `SUB` | Subscription / streaming (§7.4, §8.5) | `CHAT` | Interactive session (§8.5) |
| `CONV` | Conversation & session state (§6) | `CONFIG` | Profiles / environments (§8.5) |
| `POLL` | Task status polling (§7) | `DOWNLOAD` | Artifact retrieval (§8.5) |
| `PUSH` | Push notifications (Tier 3) | `CONFORM` | TCK conformance check (§8.5) |
| `SERVE` | Local agent mode (Tier 3) | | |

`ERR` is **reserved** and is never used as a requirement area: `A2ACLI_ERR_*` identifiers denote **error codes** (Appendix E), which carry a symbolic suffix rather than a number. Requirements about error handling live under `OUT`.

Stability rules — these make the identifiers safe to cite in tooling, test suites, and compliance reports:

- An identifier, once published, is **permanent**. It MUST NOT be renumbered, reused, or reassigned to a different requirement.
- **Tier membership is not encoded in the identifier.** A requirement may move between tiers across specification versions while keeping its identifier.
- New requirements take the next unused number in their area. Numbers need not be contiguous.
- A withdrawn requirement MUST be marked `Withdrawn` in the registry rather than deleted, and its number MUST NOT be reused.
- New areas MAY be added; existing area names MUST NOT be repurposed.

The authoritative list of requirement identifiers is the compliance-report template published alongside this specification.

---

## 4. Design principles

4.1 **Agent-first core, dual-mode.** The default behavioral contract MUST be safe for non-interactive and programmatic use (structured output, no interactive prompts required, deterministic exit codes). A rich interactive experience for humans MAY be layered on top and MUST be gated by terminal (TTY) detection. A conformant tool is a single dual-mode tool, not two separate tools.

4.2 **Stable, versioned contract.** The machine-readable output shapes (`json` and `jsonl` — §9.3, Appendix B) and the exit-code scheme (§9.6) are a stable contract; breaking changes require a specification version bump.

4.3 **Explicit, recoverable state.** Every identifier needed to resume a conversation MUST be observable in output (§6.3). Conversation state MUST NOT exist only in process memory.

4.4 **Transport- and language-agnostic.** Observable behavior MUST be identical across the JSON-RPC, HTTP+JSON, and gRPC bindings and across implementation languages.

4.5 **Opinionated defaults, always overridable.** Good defaults are essential to a seamless experience: common tasks MUST work with minimal flags. A conformant tool MUST ship the baseline defaults below, and MUST make **every** default overridable — by an explicit flag at all times, and MAY additionally be settable via a config profile or environment variable. An explicit flag MUST take precedence over a configured default, which MUST take precedence over the built-in default. A tool SHOULD expose its effective defaults (e.g. via `--help`) so a user can see what will happen before overriding.

| Behavior | Default | Override |
| --- | --- | --- |
| Transport | **HTTP+JSON**, when the Agent Card offers a choice or expresses no preference (subject to card-driven selection, §11.1) | `--transport <http-json\|jsonrpc\|grpc>` |
| Task completion | **Wait** (block) until the task reaches a terminal or interrupted state | `--async` / `--return-immediately` / `--no-wait` (return identifiers immediately) |
| Output presentation | **Human-readable, minimal yet consistently structured** text (labeled fields; not raw JSON, not a verbose dashboard) | `--output <text\|tui\|json\|jsonl>`, `-n` (machine-readable `json`) |
| Detail level | **Concise** | `-v, --verbose` for detailed output |
| Protocol version | The **latest** A2A version the tool supports, signaled explicitly (§11.2) | `--a2a-version <version>` |
| Transport security | **TLS verification enabled** | `--insecure` (development only; MUST warn) |

---

## 5. Command surface & global options

5.1 The command surface is `a2a-cli <command> [arguments] [options]`.

| Command | Tier | Purpose |
| --- | --- | --- |
| `discover` | 1 | Fetch and inspect an Agent Card |
| `send` | 1 | Send a message to start or continue a conversation |
| `get` | 1 | Retrieve a task's status and artifacts |
| `cancel` | 1 | Cancel an active task |
| `list` | 2 | List tasks |
| `subscribe` | 2 | (Re)subscribe to a task's event stream |
| `chat` | 2 | Interactive multi-turn session |
| `auth` | 2 | Interactive credential acquisition (OAuth) |
| `config` | 2 | Manage named environments/profiles |
| `download` | 2 | Save task artifacts |
| `conformance` | 2 | Smoke-check a live agent against the A2A TCK |
| `push-config` | 3 | Manage push-notification configurations |
| `serve` | 3 | Run a local mock agent (out of client baseline) |

5.2 Global options (Tier 1 MUST unless noted):

| Option | Meaning |
| --- | --- |
| `-u, --service-url <url>` | Target agent base URL (or `--env`). |
| `--context-id <id>` | Continue an existing conversation (§6.2). |
| `--task-id <id>` | Continue an existing task (§6.2). |
| `--continue` / `--last` | Resume the stored last conversation (§6.4). |
| `-o, --output <text\|tui\|json\|jsonl>` | Output mode. Default: minimal, structured `text` (§4.5). `tui` is an opt-in interactive mode; `json` emits one complete document; `jsonl` emits one JSON object per line for streaming (§9.3). |
| `-n` | Alias for `--output json`, non-interactive. |
| `--transport <http-json\|jsonrpc\|grpc>` | Override the transport binding (default HTTP+JSON, §4.5), subject to the Agent Card (§11.1). |
| `--async` / `--return-immediately` / `--no-wait` | Do not wait; return the task identifiers immediately for later polling (default is to wait, §4.5 / §7.3). |
| `--wait` / `--watch` | Block until the task reaches a terminal or interrupted state. This is the default for `send` (§4.5); stating it explicitly overrides a configured default. On `get` it turns the one-shot read into a poll loop (§7.3). |
| `--poll-interval <duration>` / `--timeout <duration>` | Polling controls (§7.3). |
| `--bearer <token>` / `--api-key <key>` / `-H, --header <k:v>` | Credentials (§10.1). |
| `--a2a-version <version>` | Protocol version to signal (§11). |
| `--env <name>` | Named profile (Tier 2). |
| `-v, --verbose` | Detailed output; additional diagnostics to stderr. `--dump-wire` (Tier 2) emits raw protocol JSON. |
| `--insecure` | Disable TLS verification (development only; MUST emit a warning). |

---

## 6. Conversation & session state

A2A conversations span multiple invocations; the CLI is the bridge that carries state between them.

### 6.1 Identifiers

| Identifier | Assigned by | Role | Constraints |
| --- | --- | --- | --- |
| `messageId` | Client | Turn identity / idempotency | SHOULD be reused when retrying a turn, because Send is not guaranteed idempotent; reuse avoids duplicated work. |
| `taskId` | Server | Unit of work | A tool MUST NOT invent a `taskId` for a new task. A client-supplied `taskId` MUST reference an existing task; otherwise the server returns a not-found error. |
| `contextId` | Server | Conversation grouping | Opaque; a tool SHOULD NOT fabricate one. A `contextId` and `taskId` that do not correspond MUST be rejected by the server; a tool MUST NOT attempt to reconcile them. |

A conformant tool never *creates* server identifiers; it **captures** them from responses and **replays** them on later turns.

### 6.2 Continuing a conversation (MUST)

A conformant tool MUST allow continuation via explicit options:

- **`--context-id <id>`** attaches this turn to an existing conversation (a new task within the same context).
- **`--task-id <id>`** continues an existing task — for example, to respond to a task waiting in `INPUT_REQUIRED` (§7.1).

Rules:
- When `--task-id` is supplied, the tool MUST send the message against that task and MUST surface any server error (e.g. not-found or state conflict) rather than silently starting a new task.
- When only `--context-id` is supplied, the tool sends a message under that context which MAY return a message or Task
- When both are supplied, the tool MUST pass them through unchanged.
- Interactive `chat` (Tier 2) MUST carry the `contextId` — and the active `taskId` while a task is interrupted — across turns automatically.

### 6.3 Reporting identifiers back (MUST)

Because the next invocation depends on them, every command that touches a task MUST expose, on completion and on interruption:

- the **`taskId`**, the **`contextId`**, and the current **task state**;
- in `--output json`, these MUST appear as stable top-level fields (`taskId`, `contextId`, `state`) per Appendix B;
- in human-facing modes, these MUST be printed in a copy-pasteable form, and the tool SHOULD print the exact command required to resume (for example, `a2a-cli send --task-id <id> "<reply>"`).

### 6.4 Local session state (SHOULD)

- A tool SHOULD persist the most recent conversation (`contextId`, latest `taskId`, service URL) so a caller can resume without re-supplying identifiers (e.g. `--continue`).
- A tool SHOULD support named profiles/environments (service URL, credentials, defaults) selected with `--env`.
- Persisted data MUST reside under a conventional configuration path, MUST NOT store secrets in world-readable files (secret files MUST be mode `0600` or the platform equivalent), and MUST be inspectable and clearable by the user.
- Explicit options MUST override stored state.

---

## 7. Task status & polling

### 7.1 Task states

Task states: `SUBMITTED`, `WORKING`, `INPUT_REQUIRED`, `AUTH_REQUIRED`, `COMPLETED`, `FAILED`, `CANCELED`, `REJECTED` (A2A §4.1.3). The canonical wire values are the A2A `TaskState` protobuf enum names (`TASK_STATE_*`, e.g. `TASK_STATE_COMPLETED`); this document uses the short forms for readability.

- **Terminal:** `COMPLETED`, `FAILED`, `CANCELED`, `REJECTED`. Streams close; no further messages are accepted.
- **Interrupted (caller action required):** `INPUT_REQUIRED`, `AUTH_REQUIRED`. A tool MUST stop waiting and return/prompt so the caller can act (§6.2).

### 7.2 Update-delivery mechanisms

A2A provides three ways to observe task progress (A2A §3.5). A conformant tool MUST implement **polling**, SHOULD implement **streaming**, and MAY implement **push notifications** (Tier 3):

1. **Streaming (SSE)** — live status/artifact events; the first event MUST be the `Task`. Available only when the Agent Card advertises the streaming capability.
2. **Polling** — repeated `get` until a terminal or interrupted state. Always available; the REQUIRED fallback when streaming is unsupported or a connection drops.
3. **Push notifications** — server-initiated webhook callbacks (Tier 3); require the tool to host a receiver, which is beyond the client baseline.

### 7.3 Polling behavior (MUST)

A conformant tool MUST provide a polling path:

- **`get <taskId>`** — one-shot retrieval of task state (with artifacts via `--include-artifacts`, history via `--history <n>`).
- **A blocking/watch mode** that repeatedly polls until a **terminal** state and **stops immediately on an interrupted** state, returning so the caller can act. This is the default behavior of `send` (§4.5) and is available on `get` via `--wait` / `--watch`.
- Polling controls: `--poll-interval` (RECOMMENDED default 2 seconds) and `--timeout` (on expiry the tool MUST exit non-zero with the timeout code, §9). A tool SHOULD apply bounded backoff, MUST NOT busy-loop, and MUST remain interruptible without losing the already-printed `taskId`.
- When both streaming and polling are available, a blocking wait MAY prefer streaming and MUST fall back to polling on stream failure. Because messages are not a reliable delivery mechanism, after any stream reconnect the tool MUST reconcile final state with a `get`.

### 7.4 Stream resumption (SHOULD)

For long-running tasks, a tool SHOULD support reconnection via `subscribe` (whose first event is the `Task`, closing the gap between a poll and a subscribe) and, where the server supports it, resumption from the last received event. After any reconnect, the tool MUST reconcile with a `get`.

---

## 8. Command specifications

### 8.1 `discover` (Tier 1, MUST)
Fetch the Agent Card from the well-known location (`/.well-known/agent-card.json`) or an explicit `--card-url`, then parse and present: identity, advertised capabilities (streaming, push notifications, extended card), declared interfaces/transports, security schemes, and skills. The tool MUST use the card to select a transport (§11). It SHOULD offer `--validate` to check the card against the A2A schema and SHOULD cache the card honoring HTTP caching semantics.

### 8.2 `send` (Tier 1, MUST)
Send a message to **start or continue** a conversation.
- Blocking by default (the operation waits until the task reaches a terminal or interrupted state, §4.5); `--async` / `--return-immediately` returns the `taskId` immediately instead.
- Accepts `--context-id` / `--task-id` (§6.2); message parts via `--text`, `--file`, `--data`; `--stream` (§7.2); polling controls (§7.3).
- With `--stream`, the tool consumes the event stream when the streaming capability is present; the first event MUST be the `Task`. If streaming is unsupported the tool MUST fall back or error clearly and MUST NOT hang.
- On `INPUT_REQUIRED` or `AUTH_REQUIRED`, the tool MUST stop and report the `taskId`, `contextId`, and state with a resume hint (§6.3), and MUST NOT deadlock.
- The tool MUST render produced artifacts.

### 8.3 `get` (Tier 1, MUST)
Retrieve a task by identifier: state, artifacts (`--include-artifacts`), and optionally history (`--history <n>`). One-shot by default; `--wait` / `--watch` polls until a terminal or interrupted state (§7.3). MUST report `taskId`, `contextId`, and state.

### 8.4 `cancel` (Tier 1, MUST)
Cancel an active task by identifier. The operation is idempotent and MAY return a not-cancelable error if the task has already reached a terminal state. MUST report the resulting state.

### 8.5 Higher-tier commands (outline)
- **Tier 2:** `list` (cursor-paginated, filterable by status and context); `subscribe` (stream reconnect); `auth login` (OAuth 2.1 device-code and client-credentials flows with a secure token store); multi-transport selection; `config` (named profiles); `chat` (interactive multi-turn); `download` (save artifacts); `--dump-wire`; `conformance` (TCK smoke check); shell completions.
- **Tier 3:** `push-config` create/get/list/delete plus a webhook receiver; gRPC transport; authenticated extended Agent Card; Agent Card signature verification; mTLS; OpenID Connect; `serve`/mock agent; catalog/registry integration; batch/stdin input; protocol extensions.

---

## 9. Output & exit codes

9.1 In a machine-readable mode (`json` or `jsonl`), a tool MUST emit only the structured payload on **stdout**; all diagnostics, prompts, progress indicators, and logs MUST go to **stderr**. The two streams MUST NOT be mixed.

9.2 When `tui` is in effect (selected explicitly or by configuration), a tool MUST auto-degrade to `text` if stdout is not a terminal, producing no terminal control sequences and never blocking on interactive input. The default output mode is already non-interactive `text` (§4.5).

### 9.3 Machine-readable modes: `json` and `jsonl`

A conformant tool MUST support **both** machine-readable modes. They serve different consumers and MUST NOT be conflated.

| Mode | Shape | Use it when |
| --- | --- | --- |
| **`json`** | Exactly **one** complete JSON document written once, when the result is known | The agent responds quickly, or the caller wants the whole result in a single parse — the common scripting case |
| **`jsonl`** | **One JSON object per line** ([JSON Lines](https://jsonlines.org/)), flushed as each event occurs | The caller consumes progress incrementally — streaming agents, and agentic apps/harnesses that render or act on partial output |

- **`json` MUST buffer**: even when the underlying interaction streams, the tool MUST emit a single final document, never a concatenation of objects. A `json` consumer can always `JSON.parse` stdout in one shot.
- **`jsonl` MUST stream**: each line MUST be a complete, independently parseable JSON object terminated by a newline, flushed as it is produced so a reader can consume the stream incrementally. Lines MUST NOT be pretty-printed across multiple physical lines.
- If a tool cannot stream a given interaction (streaming unsupported by the agent, or a one-shot command such as `cancel`), `jsonl` MUST still be honored by emitting the applicable object(s), one per line — a single-line result is valid JSONL.
- Both modes MUST use the envelope in Appendix B and MUST include the stable fields defined in §6.3.

9.4 Errors MUST be machine-readable in both modes (the error envelope in Appendix B) and MUST be normalized across transports so that the same A2A error yields the same tool-level result regardless of binding. In `jsonl`, an error terminating the stream MUST be emitted as a final error object on its own line.

The `code` field MUST carry a stable, symbolic error identifier of the form `A2ACLI_ERR_<SYMBOL>`, drawn from the registry in Appendix E. Symbolic codes — rather than bare numbers — let a caller match on meaning, keep working as the registry grows, and read clearly in logs.

Only the **eight core codes** in Appendix E.1 are required. The extended codes in E.2 are optional refinements for tools that can tell those cases apart; a tool that cannot classify a condition MUST fall back to the applicable core code (`A2ACLI_ERR_INTERNAL` if none is closer). Tools MUST NOT invent codes in the `A2ACLI_ERR_` namespace; vendor-specific codes MUST use a distinct prefix.

A tool SHOULD also populate the envelope's `hint` field with an actionable next step (Appendix B). A precise code tells a program what happened; a good hint tells a person what to do about it, and costs far less to implement than the rest of this section.

9.5 When the caller does not wait for completion (`--async` / `--return-immediately` / `--no-wait`), the tool MUST still emit a result object carrying the identifiers required to resume or poll later — at minimum `taskId` and `contextId` (§6.3) — so the caller can query status with `get` at a later time.

9.6 Exit codes. Every error code in Appendix E maps to exactly one of these: the exit code is the coarse signal for shells and CI, the error code the precise one for programmatic callers.

| Code | Meaning |
| --- | --- |
| 0 | Success / task completed |
| 1 | Generic failure |
| 2 | Usage error |
| 3 | Agent or transport unreachable |
| 4 | Authentication required or failed |
| 5 | Task failed or rejected |
| 6 | Input required (non-interactive) |
| 7 | Timeout |

---

## 10. Authentication

10.1 **Tier 1 (MUST):** scriptable, caller-supplied credentials — `--bearer`, `--api-key`, and `-H/--header`, with environment-variable equivalents. Credentials are attached per request in transport headers or metadata (A2A conveys identity at the transport layer, not in the payload).

10.2 **Tier 2 (SHOULD):** interactive OAuth 2.1 via `auth login`, supporting the device-code flow (designed for CLIs) and the client-credentials flow, with secure token storage and automatic attachment on subsequent calls.

10.3 **Tier 3 (MAY):** mutual TLS and OpenID Connect. A tool at this tier SHOULD also handle the in-task `AUTH_REQUIRED` state, a second authentication path that can occur mid-task.

10.4 Fetching the authenticated extended Agent Card MUST use a security scheme advertised on the public Agent Card.

---

## 11. Transport & version negotiation

11.1 **Transport selection (MUST):** a tool MUST select a binding from the Agent Card's declared interfaces (honoring the declared preference order) and MUST NOT assume a single transport. When the card offers multiple bindings without a decisive preference, the tool defaults to **HTTP+JSON** (§4.5), overridable with `--transport`. When an interface declares a routing identifier, the tool MUST echo it on every request.

11.2 **Protocol version (MUST):** a tool MUST signal the A2A protocol version on every request. This is a per-binding service parameter conveyed as an HTTP header, a query parameter, or gRPC metadata depending on the transport; an empty value causes the server to assume a legacy version, so the tool MUST set it explicitly. The tool SHOULD expose `--a2a-version` and MUST surface a version-unsupported error clearly rather than silently downgrading.

11.3 **Capability validation (SHOULD):** before invoking a capability-gated operation (streaming, push notifications, extended card), a tool SHOULD verify the capability on the Agent Card, and MUST declare any server-required extensions it supports.

---

## 12. Agent integration (SKILL.md)

12.1 A conformant tool MUST ship a machine-readable agent skill descriptor (`SKILL.md`) that instructs an AI coding agent how to drive the tool: use `--output json` for a single parseable result, or `--output jsonl` to consume progress incrementally (§9.3); rely on blocking completion (the default, or `--wait` on `get`) rather than ad-hoc sleeps; determine success from the reported task state; **capture and replay `taskId` and `contextId` to sustain a multi-turn conversation**; and use scriptable credentials rather than interactive login.

12.2 **One lightweight, generic skill.** A conformant tool MUST ship exactly one skill descriptor — not one per tier, per command, or per capability. It MUST be generic and token-efficient: it MUST NOT enumerate the full command surface or embed all capabilities inline, and instead MUST direct the agent to discover capabilities at runtime (for example, beginning with `a2a-cli help` / `a2a-cli <command> --help`), keeping the always-loaded context footprint minimal.

12.3 **Distinct layers; bundled and self-installable.** The specification (the behavioral contract) and the skill (agent-facing usage guidance) are DISTINCT layers and MUST be maintained separately; the skill MUST NOT restate normative requirements. The tool and its skill SHOULD be distributed as a single bundle that an AI coding agent can discover and install itself, following an established agent-plugin convention.

---

## 13. Compliance report & compatibility matrix

13.1 A tool asserting conformance MUST publish a **compliance report** stating: the tier claimed; per-command pass/fail; the A2A TCK version(s) exercised; transports covered; and conformance to the conversation/session (§6) and polling (§7) requirements.

13.2 The A2A project publishes a **compatibility matrix** (tools × features/tiers) so users can compare implementations. Entries are backed by compliance reports rather than being self-asserted alone. This evidence-based matrix — not self-declaration — is what substantiates a tool's advertised tier.

---

## 14. Non-goals

This specification does not: define server/agent behavior (`serve` mode is optional and out of the client baseline); alter A2A wire semantics; mandate an implementation language or framework; or designate which implementations are official — that is a project decision made outside this document (§15.3).

---

## 15. Governance & official status

15.1 **Ownership & ratification.** This specification is maintained under the A2A project's public GitHub organization and is ratified through the A2A project's governance process (its Technical Steering Committee). Status advances along the document axis **Draft → Proposed → Ratified**; only a Ratified version is "official" as a specification.

15.2 **Two independent status axes.** Specification status (§15.1) is independent of *implementation* maturity (**alpha → beta → GA**). An implementation MAY be released as an early alpha, with no stability guarantees, while the specification is still a Draft or Proposed.

15.3 **Reference implementations.** The A2A project MAY designate one or more reference implementations. Such a designation is a project decision recorded outside this specification; which implementations hold it, and in which languages, is published alongside the compatibility matrix (§13.2). This specification is language- and implementation-neutral: designation confers no additional normative authority, and it neither restricts nor privileges conformance (§1.4), which remains open to any implementation in any language.

15.4 **Change control.** While the specification is a **Draft**, it is still being assembled: normative requirements MAY change without a version bump, and each notable revision is recorded by date in the revision history (Appendix D) and reflected in the **Last updated** date in the header. From the first **Proposed** version onward, any change to a normative requirement MUST go through the ratification process and MUST bump the specification version. Implementers SHOULD therefore treat a Draft as a moving target and pin to a ratified version for conformance claims.

---

## Appendix A — Command to A2A operation mapping (informative)

| Command | A2A operation | A2A reference | Tier |
| --- | --- | --- | --- |
| `discover` | Get Agent Card / Get Extended Agent Card | §8 / §3.1.11 | 1 (extended: 3) |
| `send` | Send Message / Send Streaming Message | §3.1.1 / §3.1.2 | 1 |
| `get` | Get Task | §3.1.3 | 1 |
| `cancel` | Cancel Task | §3.1.5 | 1 |
| `list` | List Tasks | §3.1.4 | 2 |
| `subscribe` | Subscribe to Task | §3.1.6 | 2 |
| `push-config` | Create / Get / List / Delete Push Notification Config | §3.1.7–§3.1.10 | 3 |

## Appendix B — Minimal JSON output envelope (normative)

In a machine-readable mode (`json` or `jsonl`), task-affecting commands (`send`, `get`, `cancel`, and per-item `list`/`subscribe` output) MUST emit at least the following fields. Tools MAY add fields; consumers MUST ignore unknown fields.

**Task-operation object:**
```json
{
  "taskId":    "string | null",
  "contextId": "string | null",
  "state":     "TASK_STATE_*",
  "artifacts": [],
  "message":   null
}
```
- `taskId`, `contextId`, `state` — REQUIRED (may be `null` only when no task was created, e.g. a direct message response).
- `artifacts` — REQUIRED when artifacts were requested/available; otherwise MAY be omitted or empty.
- `message` — the direct message response when the server returned a Message rather than a Task; otherwise `null`.

**Error object** (mutually exclusive with a successful result):
```json
{
  "error": {
    "code":    "string",
    "message": "string",
    "hint":    "string | null",
    "a2aCode": "string | number | null"
  }
}
```
- `code` — REQUIRED, a symbolic `A2ACLI_ERR_<SYMBOL>` identifier from Appendix E (stable across transports).
- `message` — REQUIRED, human-readable.
- `hint` — RECOMMENDED. A short, actionable next step, ideally a copy-pasteable command. Derive it from context where possible — for example, reading the Agent Card's security schemes to name the exact login command for that agent. Omit it, or set `null`, when there is nothing useful to say; never pad it.
- `a2aCode` — the underlying A2A/transport error code when one exists, else `null`.

**Streaming (`--output jsonl`):** one JSON object per line, flushed as produced. Each line MUST carry a `type` field identifying the event (for example `status`, `artifact`, `result`, `error`), and the final/terminal line MUST include the task-operation fields above so a reader that keeps only the last line still obtains `taskId`, `contextId`, and `state`.

**Single document (`--output json`):** exactly one task-operation object (or one error object) for the whole invocation.

## Appendix C — References

- A2A Protocol Specification v1.0 — https://a2a-protocol.org/latest/specification/
- A2A Agent Discovery — https://a2a-protocol.org/latest/topics/agent-discovery/
- A2A Streaming & Asynchronous Operations — https://a2a-protocol.org/latest/topics/streaming-and-async/
- RFC 2119 — Key words for requirement levels
- RFC 8628 — OAuth 2.0 Device Authorization Grant

## Appendix D — Revision history

While the specification is in Draft, notable revisions are recorded by date; the version number changes only at ratification milestones (§15.4). Newest first.

| Version | Date | Notes |
| --- | --- | --- |
| 0.1 (Draft) | 2026-08-10 | Added a RECOMMENDED `hint` field to the error envelope (Appendix B, §9.4) — an actionable next step alongside the machine-readable code. Restructured "Why this matters" into three sourced problems, adding testing a running agent as a distinct motivation. Split the error registry into eight required core codes (E.1) and optional extended refinements (E.2) so conformance stays cheap. Added the requirement-identifier convention (§3.3) and a symbolic error vocabulary. |
| 0.1 (Draft) | 2026-08-06 | Defined two machine-readable output modes — `json` (exactly one buffered document) and `jsonl` (one object per line, flushed as produced) — and required both (§9.3). Renamed NDJSON to JSONL throughout. Added `--no-wait` as an alias for `--async` / `--return-immediately`, and required that not waiting still returns `taskId` and `contextId` for later polling (§9.5). |
| 0.1 (Draft) | 2026-08-04 | Made governance implementation-neutral: removed the designation of a specific reference implementation and language from §15.3, and aligned §1.4, §14 and §15.2. |
| 0.1 (Draft) | 2026-08-04 | Initial draft. Tier 1 normative; Tiers 2–3 outlined. Client-only baseline; conversation/session state (§6) and task polling (§7) as first-class; opinionated defaults (§4.5); conformant-vs-official + governance (§1.4, §15); normative output envelope (Appendix B). |

## Appendix E — Error code registry (normative)

Values for the `code` field of the error envelope (Appendix B).

The registry is split so that conformance stays cheap: a tool needs only the **core** codes below. The **extended** codes exist for tools that can tell failures apart more precisely — using them is encouraged but never required, and a tool that cannot distinguish a case simply reports the core code instead.

Identifiers are permanent: once published, a code MUST NOT be reused or redefined. Codes MAY be added later; a consumer MUST tolerate an unrecognized `A2ACLI_ERR_*` value and SHOULD fall back to the exit code.

### E.1 Core codes (required)

A conformant tool MUST be able to emit these. Together they cover every exit code, so a caller can always act on the result.

| Code | Meaning | Exit |
| --- | --- | --- |
| `A2ACLI_ERR_USAGE` | Invalid arguments, flags, or flag combination | 2 |
| `A2ACLI_ERR_UNREACHABLE` | Agent could not be reached — DNS, connection, TLS, or no Agent Card | 3 |
| `A2ACLI_ERR_AUTH_REQUIRED` | Credentials required but not supplied | 4 |
| `A2ACLI_ERR_AUTH_FAILED` | Credentials supplied but rejected | 4 |
| `A2ACLI_ERR_TASK_FAILED` | Task ended unsuccessfully (`FAILED`, or `REJECTED` if not distinguished) | 5 |
| `A2ACLI_ERR_INPUT_REQUIRED` | Task needs caller input in a non-interactive run | 6 |
| `A2ACLI_ERR_TIMEOUT` | `--timeout` expired before a terminal state | 7 |
| `A2ACLI_ERR_INTERNAL` | Unexpected tool-side failure, or any condition with no better code | 1 |

### E.2 Extended codes (optional)

Use when the tool can distinguish the case. Each refines a core code; if unsupported, report the core code shown in brackets.

| Code | Meaning | Refines | Exit |
| --- | --- | --- | --- |
| `A2ACLI_ERR_CARD_NOT_FOUND` | No Agent Card at the well-known location or given URL | [`UNREACHABLE`] | 3 |
| `A2ACLI_ERR_CARD_INVALID` | Agent Card fetched but malformed or schema-invalid | [`INTERNAL`] | 1 |
| `A2ACLI_ERR_TASK_NOT_FOUND` | Referenced `taskId` does not exist | [`INTERNAL`] | 1 |
| `A2ACLI_ERR_TASK_REJECTED` | Task reached `REJECTED` rather than `FAILED` | [`TASK_FAILED`] | 5 |
| `A2ACLI_ERR_TASK_NOT_CANCELABLE` | Task cannot be canceled in its current state | [`INTERNAL`] | 1 |
| `A2ACLI_ERR_CONTEXT_MISMATCH` | `contextId` and `taskId` do not correspond | [`INTERNAL`] | 1 |
| `A2ACLI_ERR_CAPABILITY_UNSUPPORTED` | Agent does not advertise a required capability (e.g. streaming) | [`INTERNAL`] | 1 |
| `A2ACLI_ERR_TRANSPORT_UNSUPPORTED` | No transport in common between tool and Agent Card | [`INTERNAL`] | 1 |
| `A2ACLI_ERR_VERSION_UNSUPPORTED` | Agent rejected the signaled protocol version | [`INTERNAL`] | 1 |
| `A2ACLI_ERR_EXTENSION_REQUIRED` | Agent requires an extension the tool does not support | [`INTERNAL`] | 1 |
| `A2ACLI_ERR_STREAM_INTERRUPTED` | Stream ended before a terminal state and could not be resumed | [`INTERNAL`] | 1 |
