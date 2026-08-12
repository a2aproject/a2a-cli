# a2a-cli Specification

**Version:** 0.1
**Status:** Draft — open for review.
**Last updated:** 2026-08-12
**Applies to:** A2A Protocol v1.0

## Abstract

This document specifies the behavior that a command-line interface (CLI) tool MUST, SHOULD, and MAY exhibit to be considered a conformant **`a2a-cli`** — a terminal client for the [Agent2Agent (A2A) Protocol](https://a2a-protocol.org/latest/specification/). It exists so that independently built CLIs, in any language, converge on one predictable command surface, output contract, and interaction model — measurable through a published compliance report.

This is an **implementer's specification**. Its audience is engineers building or improving an `a2a-cli`. It constrains CLI behavior only and never modifies A2A wire semantics.

## Why this matters

Two problems, both traceable to the same cause: there is no official A2A command-line tool.

### 1. Everyone builds their own, and they build the same thing

At least eight independent A2A CLIs exist today, across six languages — Go, Rust, Python, TypeScript, .NET, and Swift. Two already sit inside the A2A project's own GitHub organization ([A2A#1929](https://github.com/a2aproject/A2A/issues/1929)).

They largely re-implement the same short list of operations: send a task, continue a multi-turn interaction, check task status, read artifacts. Those are exactly the operations an official tool should cover.

The request keeps recurring rather than resolving. An earlier CLI contribution was closed as out of scope for its repository ([PR#1323](https://github.com/a2aproject/A2A/pull/1323)); its follow-up issue was closed as a duplicate ([#1325](https://github.com/a2aproject/A2A/issues/1325)); a working command grammar was designed separately inside the Go SDK ([a2a-go#306](https://github.com/a2aproject/a2a-go/discussions/306)); and the consolidation request itself ([A2A#1929](https://github.com/a2aproject/A2A/issues/1929)) is open for community vote.

### 2. Driving and testing a running agent has no standard path

A2A already provides SDKs in several languages, a Technology Compatibility Kit, an inspector, and a samples repository. What none of them gives is a quick way to exercise a *running* agent from outside whichever SDK you happen to be using — so checking that a deployed service actually behaves means writing client code first.

The same gap blocks AI coding agents, for a related reason. Developers increasingly work through them, and an agent learns a command-line tool from a skill descriptor. With no canonical CLI there can be no canonical skill file, and therefore no standard way for a coding agent to work with an A2A agent at all ([A2A#1929](https://github.com/a2aproject/A2A/issues/1929)).

A conformant CLI closes both at once. Because it is scriptable, emits machine-readable output and returns meaningful exit codes, it doubles as a lightweight testing framework: a bash script or a Python test can drive real interactions against a live agent, and an agentic harness can do the same unattended. Testing an A2A service — by hand while developing, or automatically in CI — stops being a project and becomes a command.

That is also why machine-readable output (§9.3) is a core requirement here rather than an optional extra: the tool's consumers are as often programs as people.

### What this document does about it

It defines the core client behavior — the operations above — so that every implementation can agree on one definition. Specialised needs can be built on top; the goal here is to get the common path right.

This is not an attempt to replace the existing tools. Any tool, in any language, can implement this specification and report exactly what it supports (§13).

## Notational conventions

The key words **MUST**, **MUST NOT**, **REQUIRED**, **SHALL**, **SHALL NOT**, **SHOULD**, **SHOULD NOT**, **RECOMMENDED**, **MAY**, and **OPTIONAL** are to be interpreted as described in [RFC 2119](https://www.rfc-editor.org/rfc/rfc2119).

References of the form "A2A §x" point to the A2A Protocol Specification v1.0. Where an A2A rule is load-bearing for the CLI it is restated here so this document is self-contained; the A2A specification remains authoritative for protocol semantics.

---

## 1. Scope

1.1 An `a2a-cli` is an **A2A client**: it initiates requests to an A2A server (a remote agent) and renders the responses. Acting *as* a server — publishing an Agent Card, generating server-side identifiers, or serving inbound requests — is **outside the baseline** of this specification (§14; `serve` is Tier 3, §5.1).

1.2 The primary purpose of an `a2a-cli` is **interaction with an A2A server**. A2A interactions MAY be multi-turn and MAY be stateful, and MAY span multiple CLI invocations. A conformant tool MUST therefore allow a caller to **inspect an agent card** and to **start, continue, and resume** an interaction (§6), and MUST provide a **polling** path for task status in addition to any streaming support (§7).

1.3 Conformance is **tiered and evidence-based** (§3). A tool asserts conformance by publishing a compliance report. Any number of **conformant** tools MAY coexist; conformance is open to any implementation, in any language, that passes the specification.

1.4 **Conformant vs. official.** These are distinct:
- A **conformant** tool is any implementation that satisfies a tier of this specification and publishes a compliance report (§13). Conformance is open to all.
- An **official** tool is one the A2A project has designated as a project-maintained reference implementation, hosted under the A2A project's GitHub organization, demonstrated against the A2A TCK, and listed in the compatibility matrix (§13). "Official" denotes governance and demonstrated conformance — not exclusivity, and not a higher standard of conformance. Designation is a project decision made outside this specification (§15.3).

---

## 2. Object model

Restated from A2A §4 so this document stands alone:

- **Message** — a single turn. Has a `role` (`user` or `agent`) and one or more **Parts** (`text`, `file`, or `data`). Carries a client-assigned **`messageId`**.
- **Task** — the stateful unit of work a message may create. Identified by a server-assigned **`taskId`**; advances through a **TaskState** (§7.1); may emit **Artifacts**.
- **Artifact** — a task output (text, structured data, or file). Task outputs are delivered as Artifacts, not Messages; a conformant tool MUST render Artifacts.
- **`contextId`** — a server-assigned, opaque identifier that groups related tasks and messages. A2A does not define what the grouping *means*; that is the agent author's decision, and a conformant tool MUST NOT assume it denotes a chat session.
- **AgentCard** — the server's machine-readable description of identity, capabilities, interfaces (transports), security schemes, and skills, obtained during discovery (§8.1).

---

## 3. Conformance model

3.1 A tool declares conformance **per tier**. A tier is satisfied only when **every applicable requirement listed for that tier** is satisfied: claiming a tier holds the tool to all of the tier's requirements, including any expressed as SHOULD in the prose, which the claim promotes to required for that tier. A requirement that is genuinely inapplicable (for example the Agent Skill requirements when the tool ships no skill) does not count against the tier; one that applies but could not be exercised is not thereby satisfied (§13.1). Tiers are cumulative: Tier 2 requires Tier 1; Tier 3 requires Tier 2.

| Tier | Name | Requirements |
| --- | --- | --- |
| **Tier 1** | Core | §4.5 default behavior · §6 interaction/session handling · §7 polling · §8.1–8.4 commands (`inspect`, `send`, `task get`, `task cancel`) · §9 output & exit codes · §10.1 auth · §11 transport & versioning · §12 SKILL.md |
| **Tier 2** | Standard | Tier 1 + `task list`, `task subscribe`, OAuth `auth login`, ≥2 transports, configuration scoping, push-notification config CRUD, `download`, wire debug, `conformance`, shell completions |
| **Tier 3** | Advanced | Tier 2 + a push-notification webhook receiver, interactive `chat`, gRPC transport, authenticated extended Agent Card, Agent Card signature verification, mTLS, OpenID Connect, `serve`/mock mode, catalog/registry, extensions |

3.2 Conformance MUST be demonstrated by a **compliance report** (§13): the tool exercised against a live A2A agent, with an outcome recorded for every requirement identifier in the tier claimed. A tool MUST NOT advertise a tier it has not demonstrated.

The A2A Technology Compatibility Kit validates *agents*, not clients, so it cannot grade an `a2a-cli`. Its role here is upstream: a reporter SHOULD state that the agent they tested against is itself TCK-conformant, so that a failure can be attributed to the CLI rather than to the agent. Reporting against a non-conformant agent measures two unknowns at once.

### 3.3 Requirement identifiers

Each testable requirement carries a stable identifier of the form:

```
A2ACLI_<AREA>_<NNN>
```

where `<AREA>` names the command or cross-cutting concern and `<NNN>` is a zero-padded sequence number within that area — for example `A2ACLI_SEND_002`, `A2ACLI_INTERACT_001`, `A2ACLI_OUT_003`.

Defined areas:

| Area | Covers | Area | Covers |
| --- | --- | --- | --- |
| `DEFAULT` | Default behavior (§4.5) | `OUT` | Output & error contract (§9.1–9.4) |
| `INSPECT` | Agent Card inspection (§8.1) | `EXIT` | Exit codes (§9.6) |
| `SEND` | Sending messages (§8.2) | `AUTH` | Authentication (§10) |
| `TASK_GET` | Task retrieval (§8.3) | `TX` | Transport selection (§11.1) |
| `TASK_CANCEL` | Task cancellation (§8.4) | `VER` | Protocol versioning (§11.2, §11.3) |
| `TASK_LIST` | Task listing (§5.1, App. A) | `SKILL` | Agent skill descriptor (§12) |
| `TASK_SUBSCRIBE` | Subscription / streaming (§7.2, §7.4) | `CHAT` | Interactive session (§6.2) |
| `INTERACT` | Interaction state (§6) | `CONFIG` | Configuration (§6.4) |
| `TASK_POLL` | Task status polling (§7.1, §7.3) | `DOWNLOAD` | Artifact retrieval (§5.1) |
| `PUSH` | Push notifications (§7.2, App. A) | `CONFORM` | TCK conformance check (§5.1) |
| `SERVE` | Local demo agent for CLI practice (§5.1, §14) | | |

`ERR` is **reserved** and is never used as a requirement area: `A2ACLI_ERR_*` identifiers denote **error codes** (Appendix E), which carry a symbolic suffix rather than a number. Requirements about error handling live under `OUT`.

Stability rules — these make the identifiers safe to cite in tooling, test suites, and compliance reports:

- An identifier MUST NOT be **reused**: a retired number never returns meaning something else. This holds at every status.
- **Renumbering** is permitted while this specification is a **Draft**, and is frozen from the first **Proposed** version onward.
- **Tier membership is not encoded in the identifier.** A requirement may move between tiers across specification versions while keeping its identifier.
- New requirements take the next unused number in their area. Numbers need not be contiguous.
- From **Proposed** onward, a withdrawn requirement MUST be marked `Withdrawn` in the registry rather than deleted, and its number MUST NOT be reused.
- New areas MAY be added; existing area names MUST NOT be repurposed.

This specification defines the identifier **scheme**, not the list of identifiers assigned under it. For the current checklist, see `COMPLIANCE.md`, published alongside this specification.

---

## 4. Design principles

4.1 **Agent-first core.** The default behavioral contract MUST be safe for non-interactive and programmatic use: no interactive prompts, no terminal control sequences, deterministic exit codes, and output whose shape does not vary with the environment. This constrains **predictability, not format** — the default output is human-readable text (§4.5, §9.2); a machine consumer selects a machine format explicitly with `--output`. Any interactive mode a tool adds (for example `chat`) MUST be gated by terminal detection and MUST NOT be the default.

4.2 **Stable, versioned contract.** The machine-readable output format (`json`, line-delimited as `jsonl` under `--stream` — §9.3) and the exit-code scheme (§9.6) are a stable contract; breaking changes require a specification version bump. The *shapes* themselves are the A2A protocol's, not this document's (Appendix B).

4.3 **Explicit, recoverable state.** Every identifier needed to resume an interaction MUST be observable in output (§6.3). Interaction state MUST NOT exist only in process memory.

4.4 **Transport- and language-agnostic.** Observable behavior MUST be identical across the JSON-RPC, HTTP+JSON, and gRPC bindings and across implementation languages.

4.5 **Opinionated defaults, always overridable.** Good defaults are essential to a seamless experience: common tasks MUST work with minimal flags. A conformant tool MUST ship the baseline defaults below, and MUST make **every** default overridable — by an explicit flag at all times, and MAY additionally be settable via an environment variable or a configuration file. An explicit flag MUST take precedence over a configured default, which MUST take precedence over the built-in default. A tool SHOULD expose its effective defaults (e.g. via `--help`) so a user can see what will happen before overriding.

| Behavior | Default | Override |
| --- | --- | --- |
| Transport | The **first interface in the Agent Card's `supported_interfaces`** — the list is always in server preference order (§11.1) | `--transport <binding>`, repeatable, highest preference first |
| Task completion | **Wait** (block) until the task reaches a terminal or interrupted state | `--async` / `--return-immediately` / `--no-wait` (return identifiers immediately) |
| Output presentation | **Human-readable `text`** — labeled fields, one field per line, no control sequences (§9.2) | `--output <text\|json>`; `--stream` renders live (text) or emits JSONL (`json`) (§9.3) |
| Detail level | **Concise** | `--verbose` for a full, human-readable view of the data exchanged with the agent |
| Protocol version | The **highest version supported by both** tool and agent, signaled explicitly, never below 1.0 (§11.2) | `--a2a-version <version>` |
| Transport security | **TLS verification enabled** | `--insecure` (development only; MUST warn) |

---

## 5. Command surface & global options

5.1 The command surface is `a2a-cli <command> [arguments] [options]`.

| Command | Tier | Purpose |
| --- | --- | --- |
| `inspect` | 1 | Fetch and inspect an Agent Card |
| `send` | 1 | Send a message to start or continue an interaction |
| `task cancel` | 1 | Cancel an active task |
| `task get` | 1 | Retrieve a task's status and artifacts |
| `help` | 1 | Show usage for the tool or a specific command |
| `auth` | 2 | Interactive credential acquisition (OAuth) |
| `show-config` | 2 | Inspect configuration (read-only): show the effective settings and the source each value resolved from (§6.4) |
| `conformance` | 2 | Smoke-check a live agent against the A2A TCK |
| `download` | 2 | Save task artifacts |
| `push-config` | 2 | Manage push-notification configurations |
| `task subscribe` | 2 | (Re)subscribe to a task's event stream |
| `task list` | 2 | List tasks |
| `chat` | 3 | Interactive multi-turn session |
| `serve` | 3 | Run a local demo agent to practise CLI commands (out of client baseline) |

5.2 Global options. Unless noted, an option is available from Tier 1; where an option controls a higher-tier feature (for example `--metadata` or `--stream`), its availability follows that feature's tier (§3.1).

| Option | Meaning |
| --- | --- |
| `-a, --agent-card <ref>` | The agent to talk to, given as an Agent Card reference: a host (the well-known path is appended), an explicit card URL (used as-is), or a `file://` path to a local card. |
| `--context-id <id>` | Group this turn with an existing interaction: the message starts a new task under the given server-assigned context, alongside the tasks already in it (§6.2). |
| `--task-id <id>` | Continue a specific existing task — for example, to reply to one waiting in `INPUT_REQUIRED`. Requires `--context-id`; a rejected identifier fails rather than starting a new task (§6.2). |
| `--metadata <json>` | Attach caller-supplied metadata to the outgoing message/request, for protocol extensions (A2A §3.2.5). This sends metadata to the agent; it is not a request for server-side metadata. |
| `--stream` | Consume the agent's live event stream (§7.2). Under `-o text` it renders events as they arrive; under `-o json` it emits JSONL. MUST be set explicitly — never enabled from config, env, or terminal detection (§9.3). |
| `--wait` / `--watch` | Block until the task reaches a terminal or interrupted state. This is the default for `send` (§4.5); stating it explicitly overrides a configured default. On `task get` it turns the one-shot read into a poll loop (§7.3). |
| `--async` / `--return-immediately` / `--no-wait` | Do not wait; return the task identifiers immediately for later polling (default is to wait, §4.5 / §7.3). |
| `--poll-interval <duration>` / `--timeout <duration>` | How often to re-check task status while waiting, and how long to wait before giving up (§7.3). |
| `-o, --output <text\|json>` | Output format. Default `text` (§4.5, §9.2). `json` emits the protocol's own types (Appendix B): one document by default, or JSON Lines (JSONL) when `--stream` is set (§9.3). |
| `--verbose` | **User-facing presentation:** show the full, human-readable breakdown of message parts and the data exchanged with the agent, rather than collapsing parts into one representation. For understanding *what was sent and received*, not how the tool got there. |
| `--transport <binding>` | Client transport preference, **repeatable and ordered** (highest first). Overrides the card's preference order (§11.1); a binding absent from the card is skipped. |
| `--a2a-version <version>` | Protocol version to signal to the server on every request (§11). |
| `--insecure` | Disable TLS verification for the negotiated transport (development only; MUST emit a warning). Transport security is on unless this is passed. |
| `--bearer <token>` / `--api-key <key>` | Pass a bearer token or an API key as the request credential (§10.1). |
| `-H, --header <k:v>` | Add an arbitrary service parameter (e.g. an HTTP header), repeatable; general-purpose, not authentication-specific (§10.1). |
| `--config <path>` | Load configuration from an explicit `.env` file in place of the local `.env` in the working directory (§6.4). Environment variables still take precedence over it (§4.5). |
| `--debug` | **Developer diagnostics:** verbose logging to stderr for troubleshooting the tool's own behavior — request/response timing, retries, transport and version negotiation; at Tier 2 this includes the raw protocol messages exchanged on the wire. For *how the tool is performing the action*, not for reading the data itself (`--verbose`). |
| `-h, --help` | Show usage for the tool or the given command, and exit. |
| `-v, --version` | Print the tool version and exit. |

This table lists **global** options only. Command-specific flags — for example `--history` and `--include-artifacts` on `task get`, `--validate` and `--extended` on `inspect`, or `--text` / `--file` / `--data` on `send` — are defined with each command in §8.

**Setting options from the environment or a config file.** Where §4.5 allows an option's value to come from the environment or a configuration file, the environment variable is named `A2ACLI_` followed by the long flag in upper snake case — `--agent-card` → `A2ACLI_AGENT_CARD`, `--context-id` → `A2ACLI_CONTEXT_ID`, `--task-id` → `A2ACLI_TASK_ID`, `--a2a-version` → `A2ACLI_A2A_VERSION`, `--bearer` → `A2ACLI_BEARER` (credential variables are REQUIRED at Tier 1, §10.1). The same names MAY instead live in a `.env` (dotenv) file — one `A2ACLI_KEY=value` per line — loaded per §6.4 (`~/.config/a2a-cli/.env`, then a local `.env`) or from an explicit file via `--config <path>`. Precedence is fixed (§4.5, §6.4): flag > environment variable > `.env` file > built-in default. `--stream` is excluded — it MUST be an explicit flag, never read from the environment or a file (§9.3) — as are the action flags `-h/--help` and `-v/--version`.

---

## 6. Interaction state & configuration

A2A interactions MAY span multiple invocations. **The CLI itself is stateless**: it reports every identifier it receives and accepts every identifier as input, but it does not remember one invocation in the next. What persists is *configuration* (§6.4), not session state.

### 6.1 Identifiers

| Identifier | Assigned by | Role | Constraints |
| --- | --- | --- | --- |
| `messageId` | Client | Turn identity / idempotency | SHOULD be reused when retrying a turn, because Send is not guaranteed idempotent; reuse avoids duplicated work. |
| `taskId` | Server | Unit of work | A tool MUST NOT invent a `taskId` for a new task. A client-supplied `taskId` MUST reference an existing task; otherwise the server returns a not-found error. |
| `contextId` | Server | Context grouping | Opaque; a tool SHOULD NOT fabricate one. A `contextId` and `taskId` that do not correspond MUST be rejected by the server; a tool MUST NOT attempt to reconcile them. |

A conformant tool never *creates* server identifiers. It reports the ones the server assigned (§6.3) and accepts them back as explicit input (§6.2); it MUST NOT store them and replay them on the caller's behalf.

### 6.2 Continuing an interaction (MUST)

A conformant tool MUST allow continuation via explicit options:

- **`--context-id <id>`** attaches this turn to an existing context (a new task within that context).
- **`--task-id <id>`** continues an existing task — for example, to respond to a task waiting in `INPUT_REQUIRED` (§7.1).

Rules:
- `--task-id` MUST be accompanied by `--context-id` (the task's context). Requiring both does not by itself prevent a mismatch — a caller can pair a valid `--task-id` with the wrong `--context-id` — which is exactly why a rejected pair MUST fail rather than be reconciled (below).
- When `--task-id` is supplied, the tool MUST send the message against that task. If the server rejects the identifier — not found, a terminal-state conflict (A2A §3.1.1), or a `--context-id` that does not correspond to the task — the tool MUST surface the protocol error, exit non-zero (§9.6), and **MUST NOT create a new task**. Silently starting a fresh task would risk writing into a context the caller did not intend; a rejected identifier is an error to surface, not a condition to work around. The tool MUST point the caller at `--debug` for the underlying protocol error.
- When only `--context-id` is supplied, the tool sends a message under that context which MAY return a message or Task
- Both supplied is the normal case for task continuation; the tool MUST pass them through unchanged.
- Interactive `chat` (Tier 3) MUST carry the `contextId` — and the active `taskId` while a task is interrupted — across turns automatically.

### 6.3 Reporting identifiers back (MUST)

Because the next invocation depends on them, every command that touches a task MUST expose, on completion and on interruption:

- the **`taskId`**, the **`contextId`**, and the current **task state**;
- in `-o json` (a single document or streamed as JSONL under `--stream`), these are carried by the protocol response type itself (Appendix B) — a tool MUST NOT flatten or rename them into fields of its own;
- in human-facing modes, these MUST be printed in a copy-pasteable form, and the tool SHOULD print the exact command required to resume (for example, `a2a-cli send --task-id <id> "<reply>"`).

### 6.4 Configuration (SHOULD)

A tool persists **configuration**, never session state. It MUST NOT record the last `contextId` or `taskId` and offer to resume from them: a caller that wants to continue supplies the identifier (§6.2), which keeps the tool stateless and the contract obvious.

Configuration values resolve in one fixed order, highest wins:

1. an explicit flag,
2. an environment variable,
3. a local configuration file (a local `.env`, or the file given by `--config`),
4. a global configuration file (`~/.config/a2a-cli/.env`),
5. the built-in default (§4.5).

Files use the `.env` (dotenv) format — one `A2ACLI_<OPTION>=value` per line, the same names as the environment-variable equivalents (§5.2), so the environment and a file share one namespace. They SHOULD be discovered the way `git` discovers its configuration: a global file at `~/.config/a2a-cli/.env`, then a local `.env` found by walking up from the working directory. `--config <path>` loads an explicit file in place of that local `.env`. A real environment variable overrides a same-named value read from any file, matching the order above. Settings SHOULD be scopeable **by agent-card reference** — for example by selecting an agent-specific file with `--config` — so a caller inherits the right credentials and defaults for that agent without naming a profile.

The `show-config` command (§5.1) is **read-only**: it prints the effective value of each setting together with the source it resolved from — an explicit flag, an environment variable, a named file, or the built-in default — so a caller can confirm the tool applied the precedence above. Configuration is added, edited, or removed by setting the environment variables or editing the `.env` files directly; the command itself never mutates them.

Persisted data MUST reside under a conventional configuration path, MUST NOT store secrets in world-readable files (secret files MUST be mode `0600` or the platform equivalent), and MUST be inspectable via the read-only `show-config` command and directly editable and removable by the user.

---

## 7. Task status & polling

### 7.1 Task states

Task states: `SUBMITTED`, `WORKING`, `INPUT_REQUIRED`, `AUTH_REQUIRED`, `COMPLETED`, `FAILED`, `CANCELED`, `REJECTED` (A2A §4.1.3). The canonical wire values are the A2A `TaskState` protobuf enum names (`TASK_STATE_*`, e.g. `TASK_STATE_COMPLETED`); this document uses the short forms for readability.

- **Terminal:** `COMPLETED`, `FAILED`, `CANCELED`, `REJECTED`. Streams close; no further messages are accepted.
- **Interrupted (caller action required):** `INPUT_REQUIRED`, `AUTH_REQUIRED`. A tool MUST stop waiting and return/prompt so the caller can act (§6.2).

### 7.2 Update-delivery mechanisms

A2A provides three ways to observe task progress (A2A §3.5). A conformant tool MUST implement **polling**, SHOULD implement **streaming**, and MAY implement **push notifications**. Managing push-notification *configuration* is an ordinary API call (`push-config`, Tier 2); only *hosting the receiver* is Tier 3:

1. **Streaming (SSE)** — live status/artifact events; the first event MUST be the `Task`. Available only when the Agent Card advertises the streaming capability.
2. **Polling** — repeated `task get` until a terminal or interrupted state. Always available; the REQUIRED fallback when streaming is unsupported or a connection drops.
3. **Push notifications** — server-initiated webhook callbacks (Tier 3); require the tool to host a receiver, which is beyond the client baseline.

### 7.3 Polling behavior (MUST)

A conformant tool MUST provide a polling path:

- **`task get <taskId>`** — one-shot retrieval of task state (with artifacts via `--include-artifacts`, history via `--history <n>`).
- **A blocking/watch mode** that repeatedly polls until a **terminal** state and **stops immediately on an interrupted** state, returning so the caller can act. This is the default behavior of `send` (§4.5) and is available on `task get` via `--wait` / `--watch`.
- Polling controls: `--poll-interval` (RECOMMENDED default 2 seconds) and `--timeout` (on expiry the tool MUST exit non-zero with the timeout code, §9). A tool SHOULD apply bounded backoff, MUST NOT busy-loop, and MUST remain interruptible without losing the already-printed `taskId`.
- When both streaming and polling are available, a blocking wait MAY prefer streaming and MUST fall back to polling on stream failure. *(Reconciling after a reconnect needs no separate rule: A2A streaming already re-sends the full `Task` as the first event on reconnect.)*

### 7.4 Stream resumption (SHOULD)

For long-running tasks, a tool SHOULD support reconnection via `task subscribe` (whose first event is the `Task`, closing the gap between a poll and a subscribe) and, where the server supports it, resumption from the last received event. Because that first event carries the full `Task`, state is reconciled by the protocol itself and no additional `task get` is required.

---

## 8. Command specifications

§8.1–§8.4 specify the **Tier 1** commands normatively. Higher-tier commands are listed with their tier in §3.2 and §5.1, and their requirements are enumerated per identifier in `COMPLIANCE.md` (§3.3); this section does not restate them.

### 8.1 `inspect` (Tier 1, MUST)
Resolve the Agent Card from `--agent-card` — a host (the well-known path `/.well-known/agent-card.json` is appended), an explicit card URL, or a local `file://` path — then parse and present: identity, advertised capabilities (streaming, push notifications, extended card), declared interfaces/transports, security schemes, and skills. The tool MUST use the card to select a transport (§11). It SHOULD offer `--validate` to check the card against the A2A schema, SHOULD offer `--extended` to fetch the authenticated extended card (Tier 3, §10.4), and SHOULD cache the card honoring HTTP caching semantics.

The command is named `inspect` rather than `discover` because "discovery" already names the *process* of resolving a card from a reference; using it for the command invites confusion with that broader sense.

### 8.2 `send` (Tier 1, MUST)
Send a message to **start or continue** an interaction.
- Blocking by default (the operation waits until the task reaches a terminal or interrupted state, §4.5); `--async` / `--return-immediately` returns the `taskId` immediately instead.
- Accepts `--context-id` / `--task-id` (§6.2); `--stream` (§7.2); polling controls (§7.3); `--metadata` for extension data.
- **Message parts.** A message MAY carry more than one part, and the part flags are repeatable and order-preserving: `--text <string>`, `--file <path>` (a local filesystem path), `--data <path|->` (a JSON file, or `-` to read from stdin). A tool MUST let the caller set the media type of a part explicitly — file extensions are not reliable indicators and are frequently absent — and SHOULD infer it only when not given.
- With `--stream`, the tool consumes the event stream when the streaming capability is present. If the agent returns a `Message` rather than a `Task` the stream contains exactly that one object and closes; the tool MUST exit cleanly with the output rather than treating it as an error. If streaming is unsupported the tool MUST fall back or error clearly, and MUST NOT hang.
- On `INPUT_REQUIRED` or `AUTH_REQUIRED`, the tool MUST stop and report the `taskId`, `contextId`, and state with a resume hint (§6.3), and MUST NOT deadlock.
- **The tool MUST render produced artifacts**, meaning: a text part is printed as readable text rather than dumped as raw structure; a data part is printed as formatted JSON; a file part has its name, media type and size reported, and its content written to disk only when the caller asked for it (`download`, Tier 2). Rendering never silently discards a part.

### 8.3 `task get` (Tier 1, MUST)
Retrieve a task by identifier: state, artifacts, and optionally history (`--history <n>`). One-shot by default; `--wait` / `--watch` polls until a terminal or interrupted state (§7.3). MUST report `taskId`, `contextId`, and state.

When a result carries artifacts, the tool MUST render them to stdout in the selected output format (§8.2) — a text part as readable text, a data part as JSON, a file part reported by name, media type, and size. A tool MUST NOT silently strip artifacts the server returned. `--include-artifacts` remains available to request artifact content where a server treats its inclusion as optional; it is not a licence to discard artifacts that were returned, and in `-o json` the full protocol `Task` is emitted unchanged regardless.

### 8.4 `task cancel` (Tier 1, MUST)
Cancel an active task by identifier. The operation is idempotent and MAY return a not-cancelable error if the task has already reached a terminal state. MUST report the resulting state.

---

## 9. Output & exit codes

9.1 In the machine-readable format (`-o json`, whether a single document or streamed as JSONL under `--stream`), a tool MUST emit only the structured payload on **stdout**; all diagnostics, prompts, progress indicators, and logs MUST go to **stderr**. The two streams MUST NOT be mixed.

9.2 **The `text` mode floor.** `text` is the default (§4.5) and MUST be safe to parse and to pipe. A tool MUST emit one `Label: value` field per line, MUST use the same labels across invocations, MUST NOT emit terminal control sequences, and MUST include the task identifier, the context identifier, and the task state for any command that touches a task.

Any interactive mode a tool offers beyond this (for example `chat`, Tier 3) MUST auto-degrade to `text` when stdout is not a terminal, and MUST never block on interactive input in that case.

### 9.3 Machine-readable output: `json`, and its streamed form `jsonl`

The machine-readable format is `-o json`. Its **cardinality follows `--stream`**: one document when the caller waits, JSON Lines when the caller streams. Both carry the A2A protocol's own response types (Appendix B) — never a substitute schema.

| Invocation | Shape | Use it when |
| --- | --- | --- |
| **`-o json`** (no `--stream`) | Exactly **one** complete JSON document — the terminal protocol object (the final `Task`, or the `Message` where no task was created) | The caller wants the outcome in a single parse — the common scripting case |
| **`-o json --stream`** | **JSON Lines** ([JSONL](https://jsonlines.org/)): one complete JSON object per line, flushed as each event occurs | The caller consumes progress incrementally — streaming agents, and agentic apps/harnesses that render or act on partial output |

- **Without `--stream`, `json` MUST be a single document** — exactly one object, never a concatenation of events, so a consumer can `JSON.parse` stdout in one shot. Size is bounded by the task, not by how many events it produced.
- **With `--stream`, `json` MUST be emitted as JSONL** — each line a complete, independently parseable JSON object terminated by a newline, flushed as produced. Lines MUST NOT be pretty-printed across multiple physical lines, and the final line MUST carry the terminal object so a reader that keeps only the last line still obtains the identifiers and state.
- The switch is **caller-controlled, not implicit**: `--stream` MUST be an explicit command-line flag, and a tool MUST NOT enable it from configuration, an environment variable, or terminal detection. A caller that did not pass `--stream` always gets a single document, so the output shape is a function of the invocation the caller wrote.
- If the caller passes `--stream` but the interaction cannot stream (streaming unsupported by the agent, or a one-shot command such as `cancel`), the tool MUST still honor JSONL by emitting the applicable object(s), one per line — a single-line result is valid JSONL.
- Both forms MUST emit the A2A protocol's own response types (Appendix B); a tool MUST NOT define a substitute schema.

9.4 Errors MUST be machine-readable (the error envelope in Appendix B) and MUST be normalized across transports so that the same A2A error yields the same tool-level result regardless of binding. Under `--stream`, an error terminating the stream MUST be emitted as a final error object on its own line.

Errors come in two layers, and a tool MUST NOT blur them.

**Protocol failures** carry the A2A error the server reported, by name, unchanged — `TaskNotFoundError`, `UnsupportedOperationError`, `ContentTypeNotSupportedError` and the rest of the set defined in **A2A §3.3.2**, mapped across bindings per **A2A §5.4**. This specification does not restate that set and does not rename it. (§11.6 of A2A defines the same handling for the HTTP+JSON binding only; citing it here would tie the CLI's errors to one transport, contradicting §4.4.)

**CLI-local failures** are the conditions the protocol has no opinion on, because they happen before or outside any request: a malformed flag, an unresolvable `--agent-card` reference, an unreadable local card, no network. Those carry a symbolic `A2ACLI_ERR_<SYMBOL>` identifier from Appendix E.

A condition the protocol already names MUST carry the protocol's error rather than an `A2ACLI_ERR_` code. A tool that cannot classify a CLI-local condition MUST report `A2ACLI_ERR_INTERNAL`. Tools MUST NOT invent codes in the `A2ACLI_ERR_` namespace; vendor-specific codes MUST use a distinct prefix.

A tool SHOULD also populate the envelope's `hint` field with an actionable next step (Appendix B). A precise code tells a program what happened; a good hint tells a person what to do about it, and costs far less to implement than the rest of this section.

9.5 When the caller does not wait for completion (`--async` / `--return-immediately` / `--no-wait`), the tool MUST still emit a result object carrying the identifiers required to resume or poll later — at minimum `taskId` and `contextId` (§6.3) — so the caller can query status with `task get` at a later time.

9.6 Exit codes. The exit code is the coarse signal for shells and CI — the only result a caller gets without parsing output. The error code (§9.4) is the precise one.

**Required.** A conformant tool MUST implement these:

| Code | Meaning |
| --- | --- |
| 0 | Success — the operation completed, and any task it created reached a successful terminal state |
| 1 | Failure — any error with no more specific code the tool implements |
| 2 | Usage error — invalid arguments, flags, or flag combination |

**Reserved.** These identifiers carry the meanings below and MUST NOT be used for any other purpose. A tool MAY implement any of them; where it does not, it MUST report `1` instead.

| Code | Meaning |
| --- | --- |
| 3 | Agent or transport unreachable |
| 4 | Authentication required or failed |
| 5 | Task failed or rejected |
| 6 | Input required, in a non-interactive run |
| 7 | Timeout |

Whatever a tool emits MUST agree with the error it reported (§9.4). A run that reports a timeout and exits `5` is non-conformant regardless of which codes it implements.

A later version of this specification MAY promote reserved codes to required. A tool that implements them early is unaffected by that change, and a caller testing only for a non-zero status is unaffected either way.

---

## 10. Authentication

10.1 **Tier 1 (MUST):** scriptable, caller-supplied credentials — `--bearer` and `--api-key`, with environment-variable equivalents. Credentials are attached per request as **service parameters** (A2A §3.2.6), which each binding maps to its own mechanism — an HTTP header, a query parameter, or gRPC metadata. A2A conveys identity at the transport layer, not in the payload.

`-H/--header` is a **separate, general-purpose** option for attaching any additional service parameter, not only credentials, and MUST NOT be documented as an authentication flag.

10.2 **Tier 2 (SHOULD):** interactive OAuth 2.1 via `auth login`, supporting the device-code flow ([RFC 8628](https://www.rfc-editor.org/rfc/rfc8628), designed for input-constrained clients such as a CLI) and the client-credentials flow, with secure token storage and automatic attachment on subsequent calls.

10.3 **Tier 3 (MAY):** mutual TLS and OpenID Connect. A tool at this tier SHOULD also handle the in-task `AUTH_REQUIRED` state, a second authentication path that can occur mid-task.

10.4 Fetching the authenticated extended Agent Card MUST use a security scheme advertised on the public Agent Card.

---

## 11. Transport & version negotiation

11.1 **Transport selection (MUST):** a tool MUST select a binding from the Agent Card's declared interfaces and MUST NOT assume a single transport. `supported_interfaces` is **always in server preference order**, so absent any client preference a tool MUST use the first entry it supports.

A client MAY express its own preference with `--transport`, which is **repeatable and ordered**: the tool takes the first client-preferred binding the card also offers, and falls back to the card's order when none matches. A single-valued preference is insufficient — it leaves a tool with no way to negotiate against a card that does not offer it. When an interface declares a routing identifier, the tool MUST echo it on every request.

Cross-cutting options such as `--insecure` apply to whichever transport is negotiated. Where a future option is meaningful only for one binding (for example a gRPC keepalive setting that HTTP has no analogue for), a tool SHOULD namespace it per transport rather than overloading a global flag — the reserved convention is `--<binding>-<option>` (for example `--grpc-keepalive`). This specification defines no such per-transport option today; the convention is reserved so that adding one later is not a breaking change.

11.2 **Protocol version (MUST):** a tool MUST signal the A2A protocol version on every request. This is a per-binding service parameter conveyed as an HTTP header, a query parameter, or gRPC metadata depending on the transport; an empty value causes the server to assume a legacy version, so the tool MUST set it explicitly. The tool SHOULD expose `--a2a-version`. Absent an explicit value, a tool SHOULD negotiate down to the highest version supported by both itself and the agent as declared on the Agent Card, but MUST NOT negotiate below **1.0** — versions earlier than 1.0 are legacy and MUST require an explicit opt-in. A tool MUST surface a version-unsupported error clearly rather than silently downgrading.

11.3 **Capability validation (SHOULD):** before invoking a capability-gated operation (streaming, push notifications, extended card), a tool SHOULD verify the capability on the Agent Card.

Declaring the server-required extensions a tool supports is a separate, Tier 3 requirement (`A2ACLI_VER_002`) and is not required of a Tier 1 tool.

---

## 12. Agent integration (Agent Skills)

> **Terminology.** "Agent Skill" in this section means a directory containing a `SKILL.md`, per the Agent Skills format [AGENT-SKILLS]. It is unrelated to the `AgentSkill` object A2A defines at §4.4.5, which describes a capability advertised on an Agent Card. The two are different things that unfortunately share a name.

12.1 **Conditional, and exactly one.** A tool is not required to ship an Agent Skill. **If it ships one, it MUST ship exactly one** — not one per tier, per command, or per capability. A specification that standardises the command surface makes a single skill sufficient for any conformant tool, so shipping several is duplication rather than coverage.

The skill instructs an AI coding agent how to drive the tool: use `-o json` for a single parseable result, or `-o json --stream` to consume progress incrementally as JSONL (§9.3); rely on blocking completion rather than ad-hoc sleeps; determine success from the reported task state; pass `--context-id` and `--task-id` explicitly, since the tool holds no session state (§6); and use scriptable credentials rather than interactive login.

12.2 **Lean, deferring to runtime help.** The `SKILL.md` itself MUST be generic and token-efficient: it MUST NOT enumerate the full command surface or embed every capability inline, and MUST direct the agent to discover capabilities at runtime (for example `a2a-cli help`, `a2a-cli <command> --help`), keeping the always-loaded context footprint small.

Worked examples — a command with representative input and output — are genuinely useful and SHOULD live in a references file alongside `SKILL.md` rather than in the body, so they cost nothing until an agent needs them.

12.3 **Distinct layers.** The specification (the behavioural contract) and the skill (agent-facing usage guidance) are DISTINCT layers and MUST be maintained separately. The skill MUST NOT restate normative requirements: an agent needs to know how to invoke the tool, not which clause obliges it.

12.4 **Distribution.** A skill SHOULD be installable into the cross-client location `<scope>/.agents/skills/<tool>/`. *(The Agent Skills format does not define an installation location; this is a widely-adopted convention, not a normative requirement of that format.)*

A skill **MUST NOT** assume it can install the tool's binary — no current agent-facing standard defines an installation mechanism. It SHOULD declare the dependency as human-readable prose in the `compatibility` frontmatter field, and SHOULD include a preflight check and install pointers in its body. Binary distribution is out of scope here and belongs to platform package managers.

Distribution is expected to arrive in two stages, because the priority is adoption — the widest set of agents able to use the tool with the least friction:

1. **Baseline — the tool and one skill.** The first form is the tool's binary plus a single Agent Skill (§12.1), the skill installed at the location above. When a tool ships a skill, this baseline stands on its own and MUST NOT depend on any plugin machinery, so any agent that understands Agent Skills can use the tool immediately.
2. **Next — an Agent Plugin.** The project SHOULD then also publish an **Agent Plugin package** [AGENT-PLUGINS], which carries the skill as a plugin component and MAY add an MCP server, so a plugin-aware agent installs, versions, and updates them as one unit. Agent Plugins defines exactly two component types — Agent Skills and MCP servers — so the plugin packages the *agent-facing* pieces; the tool's binary itself is not a plugin component and its installation still belongs to platform package managers. The plugin references the binary via the preflight check above rather than embedding it.

The two are complementary, not exclusive: the skill shipped in stage 1 is the same skill the plugin carries in stage 2, keeping them co-versioned. A caller MAY always install the skill and the binary independently — the plugin is a convenience, never a precondition — and Agent Plugins defines packaging only; installation, distribution, and permissions remain client-controlled.

## 13. Compliance report & compatibility matrix

13.1 A tool asserting conformance MUST publish a **compliance report** stating: the tier claimed; the outcome **per requirement identifier** (§3.3), not merely per command; the agent it was exercised against, and whether that agent is TCK-conformant (§3.2); transports covered; and a note on every result that is not a plain pass.

A requirement the reporter could not provoke — an agent that never returns `INPUT_REQUIRED`, say — MUST be recorded as such with the reason, never assumed to pass. An unobservable requirement and a satisfied one are different results.

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

Proposals enter through two channels, both submitted for review under the project's governance process:

- a **feature request** proposes a new capability — a command, flag, tier member, or output form not yet covered;
- a **request for change (RFC)** proposes modifying or withdrawing an existing normative requirement.

An accepted proposal is applied under the change-control rules above — recorded in the revision history while the specification is a Draft, and gated on ratification with a version bump once it is Proposed or later.

---

## Appendix A — Command to A2A operation mapping (informative)

| Command | A2A operation | A2A reference | Tier |
| --- | --- | --- | --- |
| `inspect` | Get Agent Card / Get Extended Agent Card | §8 / §3.1.11 | 1 (extended: 3) |
| `send` | Send Message / Send Streaming Message | §3.1.1 / §3.1.2 | 1 |
| `task cancel` | Cancel Task | §3.1.5 | 1 |
| `task get` | Get Task | §3.1.3 | 1 |
| `push-config` | Create / Get / List / Delete Push Notification Config | §3.1.7–§3.1.10 | 2 |
| `task subscribe` | Subscribe to Task | §3.1.6 | 2 |
| `task list` | List Tasks | §3.1.4 | 2 |

## Appendix B — Machine-readable output (normative)

**This specification defines no output schema of its own.** In `json` output — a single document, or streamed as JSONL under `--stream` — a tool MUST emit the A2A protocol's own response types, unmodified, as defined by `spec/a2a.proto` and rendered per A2A's JSON field-naming convention (A2A §5.5). Inventing a CLI-level envelope would oblige every consumer to unwrap it, break protocol schema validators, and commit this specification to a second versioned schema with its own deprecation policy.

**Which type, per command:**

| Command | `-o json` emits (one document) | `-o json --stream` emits, one per line |
| --- | --- | --- |
| `send` | `SendMessageResponse` — the terminal object: `task` when a task was created, otherwise `message` | `StreamResponse` per event |
| `task get` | `Task` | `Task` (single line) |
| `task cancel` | `Task` | `Task` (single line) |
| `task list` | `ListTasksResponse` | one `Task` per line |
| `task subscribe` | the terminal `Task` | `StreamResponse` per event |
| `inspect` | `AgentCard` | `AgentCard` (single line) |

`SendMessageResponse` and `StreamResponse` are discriminated unions (protobuf `oneof`), which is what makes them scriptable: a consumer switches on which field is present rather than inspecting the shape. A tool MUST NOT add a discriminator field of its own — the `oneof` is the discriminator.

Tools MAY add fields outside the protocol types only where the protocol provides an extension point; consumers MUST ignore unknown fields.

**Streaming (`-o json --stream`).** One `StreamResponse` per line, flushed as produced, each a complete JSON object on a single physical line. The final line MUST carry the terminal `Task` (or `Message`), so a reader that keeps only the last line still obtains the task identifier, the context identifier, and the state.

**Single document (`-o json`, no `--stream`).** Exactly one object for the whole invocation — the terminal object above, or one error object. Never a concatenation, and never the event log.

**Errors.** A failure emits one error object instead of a result:

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
- `code` — REQUIRED. For a protocol failure this is the A2A error name (§9.4). For a condition the protocol has no opinion on — a malformed flag, an unresolvable `--agent-card` reference, an unreadable local card — it is an `A2ACLI_ERR_<SYMBOL>` identifier from Appendix E.
- `message` — REQUIRED, human-readable.
- `hint` — RECOMMENDED. A short, actionable next step, ideally a copy-pasteable command. Derive it from context where possible — for example, reading the Agent Card's security schemes to name the exact login command for that agent. Omit it, or set `null`, when there is nothing useful to say; never pad it.
- `a2aCode` — the underlying transport-level code when one exists, else `null`.

## Appendix C — References

**Normative.** These define terms this specification depends on.

- A2A Protocol Specification v1.0 — https://a2a-protocol.org/latest/specification/ — the authority for all protocol semantics, data types, error names and binding behaviour referenced as "A2A §x".
- RFC 2119 — key words for requirement levels.
- RFC 8628 — OAuth 2.0 Device Authorization Grant (§10.2).
- [AGENT-PLUGINS] Agent Plugins Specification v1.0.0 — https://agent-plugins.org/specification (§12.4).

**Informative.**

- [AGENT-SKILLS] Agent Skills specification — https://agentskills.io/specification (accessed 2026-08-11). Cited informatively: the document carries no version and publishes no governance model, so this specification does not bind conformance to it.

## Appendix D — Revision history

While the specification is in Draft, notable revisions are recorded by date; the version number changes only at ratification milestones (§15.4). Newest first.

| Version | Date | Notes |
| --- | --- | --- |
| 0.1 (Draft) | 2026-08-12 | Configuration surface. `show-config` is now **read-only** — it shows each effective setting and the source it resolved from (flag, environment, file, or built-in), letting a caller verify precedence (§5.1, §6.4); it no longer edits or clears, which is done by setting environment variables or editing `.env` files directly. §5.2 gains a footer defining the environment-variable convention `A2ACLI_<LONG_FLAG>` (e.g. `A2ACLI_AGENT_CARD`, `A2ACLI_CONTEXT_ID`) shared with a `.env` (dotenv) config file, plus a new `--config <path>` option to load an explicit file in place of the local `.env`; `--stream` and the `-h`/`-v` action flags are excluded. §6.4 names the files (`~/.config/a2a-cli/.env` global, local `.env`), keeps env-over-file precedence, and retains agent-card scoping via `--config`. |
| 0.1 (Draft) | 2026-08-12 | Consistency pass. `subscribe` renamed to `task subscribe` throughout, matching the resource-namespacing convention already used by `task get`, `task list`, and `task cancel` (§3.2, §5.1, §7.4, Appendices A–B). §5.2 distinguishes `--verbose` (user-facing: the data exchanged with the agent) from `--debug` (developer-facing: how the tool itself is behaving), and §4.5's Detail-level row matches. §8.5 removed: it was a third inventory of higher-tier commands alongside §3.2 and §5.1, it had already drifted from both, and every item it listed is defined more precisely elsewhere; §8 now opens by stating that it specifies Tier 1 only. §3.3: areas `SUB` and `POLL` renamed to `TASK_SUBSCRIBE` and `TASK_POLL` for consistency with `TASK_GET` / `TASK_CANCEL` / `TASK_LIST`; both had cited the wrong sections (`SUB` omitted §7.2, `POLL` claimed all of §7 including streaming); `SERVE` restated as a local demo agent for practising CLI commands; the closing paragraph condensed and every §8.5 citation repointed. |
| 0.1 (Draft) | 2026-08-11 | Consistency fixes surfaced while aligning the compliance registry. `chat` is Tier 3 in §6.2 (§5.1/§8.5 already agreed). §3.3 area table no longer encodes tiers on `PUSH`/`SERVE` (tier is not encoded in an identifier, §3.3). §3.1 tier satisfaction restated: a tier claim holds the tool to every applicable requirement listed for that tier — SHOULD-worded rows included — with genuinely inapplicable requirements excused and unobservable ones not counted as satisfied (§13.1). |
| 0.1 (Draft) | 2026-08-11 | Second review round. Output format is `-o <text\|json>`; `--stream` selects live delivery, rendering events under `text` and emitting JSONL under `json`, so `json` cardinality follows an explicit `--stream` (never inferred from config, env, or a TTY). Added `-v/--version`, with `--verbose` keeping only its long form. Dropped named profiles and `--env`: configuration resolves through an environment variable and local/global files, keeping the CLI stateless. `-H/--header` split from the credential flags as a general-purpose service parameter. `--metadata` clarified as caller-sent request metadata. `agent-inspect` renamed to `inspect`. push-config is Tier 2; only the receiver is Tier 3. A rejected `--task-id` fails and creates nothing, rather than starting a task in an unintended context. Artifacts returned by the server MUST be rendered, never silently stripped. A reserved `--<binding>-<option>` convention allows future per-transport flags. Distribution is phased — the tool and one skill first, then an Agent Plugin carrying the skill (and optionally an MCP server), with independent installation still allowed. §15.4 adds feature-request and RFC intake channels. |
| 0.1 (Draft) | 2026-08-11 | Terminology: interaction, not conversation. Transport honours the Agent Card's preference order; `--transport` is repeatable and ordered; version negotiates down only within 1.x. Machine-readable output emits the protocol's own types — Appendix B defines no schema; `tui` removed and §9.2 pins the `text` floor. The CLI is stateless: no capture-and-replay, no `--continue`; §6.4 keeps configuration only, with a documented precedence. Commands namespaced (`task get`, `task list`), `discover` → `inspect`, `--service-url` and `--card-url` → `--agent-card`. Errors defer to the A2A set (§3.3.2, §5.4); Appendix E reduced to CLI-local conditions. `chat` → Tier 3, `push-config` → Tier 2. Shipping a skill is conditional. Conformance is demonstrated against a live agent rather than the TCK, which validates agents rather than clients. Identifier permanence binds from Proposed. Exit codes split into three required statuses and five reserved ones, so conformance is achievable while the vocabulary stays fixed. |
| 0.1 (Draft) | 2026-08-04 | Initial draft. Client-only baseline; Tier 1 normative with Tiers 2–3 outlined; interaction and session handling (§6) and task polling (§7) as first-class; opinionated defaults (§4.5); conformant-versus-official and governance (§1.4, §15). |

## Appendix E — CLI-local error codes (normative)

Values for the `code` field (Appendix B) **when the failure is the CLI's own**. A failure the protocol already names carries that A2A error instead (§9.4, A2A §3.3.2); this registry does not restate or rename the protocol's error set.

The registry stays small by construction: any condition the protocol already names belongs to the protocol, not here.

The **Exit** column gives the status each code maps to when the tool implements that exit code. Codes `0`, `1` and `2` are required; a tool that does not implement a reserved code reports `1` in its place (§9.6).

A consumer MUST tolerate an unrecognized `A2ACLI_ERR_*` value and SHOULD fall back to the exit code. Codes MAY be added; a published code MUST NOT be reused for a different meaning. Renaming follows the same rule as requirement identifiers (§3.3).

| Code | Meaning | Exit |
| --- | --- | --- |
| `A2ACLI_ERR_USAGE` | Invalid arguments, flags, or flag combination | 2 |
| `A2ACLI_ERR_CARD_NOT_FOUND` | The `--agent-card` reference could not be resolved to a card | 3 |
| `A2ACLI_ERR_CARD_INVALID` | Card fetched or read but malformed or schema-invalid | 1 |
| `A2ACLI_ERR_UNREACHABLE` | Agent could not be reached — DNS, connection, or TLS | 3 |
| `A2ACLI_ERR_AUTH_REQUIRED` | Credentials required but not supplied | 4 |
| `A2ACLI_ERR_AUTH_FAILED` | Credentials supplied but rejected | 4 |
| `A2ACLI_ERR_TASK_FAILED` | Task ended unsuccessfully (`FAILED` or `REJECTED`) | 5 |
| `A2ACLI_ERR_INPUT_REQUIRED` | Task needs caller input in a non-interactive run | 6 |
| `A2ACLI_ERR_TIMEOUT` | `--timeout` expired before a terminal state | 7 |
| `A2ACLI_ERR_INTERNAL` | Unexpected tool-side failure, or any condition with no better code | 1 |

Task states are not protocol errors — a task reaching `FAILED`, or a run stopping at `INPUT_REQUIRED`, is a normal protocol outcome that the CLI classifies so a shell can act on it. That is why those two codes live here rather than in the protocol's set.
