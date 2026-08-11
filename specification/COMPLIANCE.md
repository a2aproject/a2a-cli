# a2a-cli Compliance Report

> **Template.** Copy this file into your tool's repository, complete every field, and replace all `<…>` placeholders. Link the finished report from the A2A compatibility matrix. A tool MUST NOT advertise a tier it has not demonstrated here.

This file is also the **authoritative registry of requirement identifiers** (`SPEC.md` §3.3). An identifier is never **reused** — a retired number never returns meaning something else — but **renumbering is permitted while the specification is a Draft** and freezes from the first Proposed version. Tier membership is *not* encoded in the identifier, so a requirement can move tiers and still be tracked by the same ID. From Proposed onward, withdrawn requirements stay listed and marked `Withdrawn`.

**This list is not fixed — it is expected to grow.** If you are building a tool and hit a real use case that no requirement covers, please open a request against the specification repository. It can be added in a future revision rather than left undocumented. Adding requirements never changes existing identifiers, so reports and test suites that cite them keep working.

**Why report at this level of detail.** The project's goal is a single official CLI. A per-requirement report is what makes progress toward that goal visible: it shows where a tool stands today, what it does not do yet, and — for open-source contributors — exactly which gaps are open to pick up.

**On authentication.** Authentication, security, and compliance are large topics that need more careful treatment than a checklist row can give them. The `A2ACLI_AUTH_*` requirements below cover the ground the specification defines today, and they are expected to expand. Treat the current coverage as a starting point, not a complete security review.

**On exit codes.** An exit code is the number a command hands back to the shell when it finishes: `0` means success, anything else signals a failure. It matters because it is the only result a script or CI job gets without parsing output — `a2a-cli send … && deploy.sh` behaves correctly only if the tool exits non-zero when the task actually failed. Three statuses are required (`0`, `1`, `2`); the rest are reserved, so a tool that implements them reports more precisely rather than merely more. Record which reserved statuses the tool emits in the notes.

**On errors.** Failures come in two layers (`SPEC.md` §9.4): a protocol failure carries the A2A error by name (A2A §3.3.2, mapped per §5.4), and a CLI-local failure carries an `A2ACLI_ERR_*` identifier from Appendix E. A tool that renames protocol errors into a vocabulary of its own does not satisfy `A2ACLI_OUT_004`.

## 1. Summary

| Field | Value |
| --- | --- |
| Tool name | `<tool>` |
| Tool version | `<x.y.z>` |
| Implementation language | `<Go / Rust / Python / …>` |
| Repository | `<url>` |
| Maintainer / contact | `<name or org>` |
| Report date | `<YYYY-MM-DD>` |
| Specification version targeted | `0.1` |
| **Tier claimed** | `<Tier 1 / Tier 2 / Tier 3>` |
| A2A protocol version(s) | `<e.g. 1.0>` |
| Agent exercised against | `<name / URL>` |
| That agent is TCK-conformant? | `<yes / no / unknown>` |
| Transports covered | `<HTTP+JSON / JSON-RPC / gRPC>` |
| Designation | `<conformant / official (project-designated)>` |

## 2. Legend

| Mark | Meaning |
| --- | --- |
| `✅ Pass` | Implemented and verified |
| `◐ Partial` | Partially implemented — state what is missing |
| `❌ Fail` | Not implemented, or does not behave as specified |
| `— N/A` | Not applicable to this tool — state why |

Every `◐`, `❌`, or `—` MUST carry a note.

## 3. Results at a glance

| Tier | Requirements | `✅` | `◐` | `❌` | `—` | Tier satisfied? |
| --- | --- | --- | --- | --- | --- | --- |
| Tier 1 — Core | 28 | `<>` | `<>` | `<>` | `<>` | `<yes/no>` |
| Tier 2 — Standard | 13 | `<>` | `<>` | `<>` | `<>` | `<yes/no>` |
| Tier 3 — Advanced | 11 | `<>` | `<>` | `<>` | `<>` | `<yes/no>` |

A tier is satisfied only when every requirement in it is `✅`. Tiers are cumulative (`SPEC.md` §3.1).

## 4. Tier 1 — Core (required)

| ID | Requirement | Spec § | Status | Notes |
| --- | --- | --- | --- | --- |
| `A2ACLI_INSPECT_001` | `agent-inspect` — resolve and parse an Agent Card from a host, an explicit URL, or a `file://` path | §8.1 | `<>` | |
| `A2ACLI_SEND_001` | Send a message to start an interaction | §8.2 | `<>` | |
| `A2ACLI_SEND_002` | Blocking by default; `--async` / `--return-immediately` / `--no-wait` overrides | §8.2, §4.5 | `<>` | |
| `A2ACLI_SEND_003` | `--stream` consumes SSE when supported; never hangs when unsupported | §8.2, §7.2 | `<>` | |
| `A2ACLI_SEND_004` | Renders produced artifacts | §8.2 | `<>` | |
| `A2ACLI_TASK_GET_001` | `task get` — retrieve task state, artifacts, and history by identifier | §8.3 | `<>` | |
| `A2ACLI_TASK_CANCEL_001` | `task cancel` — cancel a task; idempotent | §8.4 | `<>` | |
| `A2ACLI_INTERACT_001` | Continue an interaction via `--context-id` | §6.2 | `<>` | |
| `A2ACLI_INTERACT_002` | Continue a task via `--task-id`, which MUST be accompanied by `--context-id`; a rejected identifier warns and continues in the same context, reporting both identifiers | §6.2 | `<>` | |
| `A2ACLI_INTERACT_003` | Never invents server-assigned identifiers | §6.1 | `<>` | |
| `A2ACLI_INTERACT_004` | Reports `taskId` / `contextId` / `state` on completion and interruption | §6.3 | `<>` | |
| `A2ACLI_INTERACT_005` | Persisted configuration *(if any)*: conventional path, secrets not world-readable, documented precedence | §6.4 | `<>` | |
| `A2ACLI_POLL_001` | Polling path available — `get` plus wait/watch | §7.3 | `<>` | |
| `A2ACLI_POLL_002` | `--poll-interval` and `--timeout` honored; bounded backoff; no busy-loop | §7.3 | `<>` | |
| `A2ACLI_POLL_003` | Stops immediately on interrupted states (`INPUT_REQUIRED` / `AUTH_REQUIRED`) without deadlock | §7.1, §7.3 | `<>` | |
| `A2ACLI_POLL_004` | Falls back to polling on stream failure and reconciles final state with `get` | §7.3 | `<>` | |
| `A2ACLI_OUT_001` | **Standard output** — every response about a task says which context and which task it concerns and what state it is in, in every output mode; the payload goes to stdout and diagnostics to stderr, never mixed | §6.3, §9.1, §9.2, §9.5 | `<>` | |
| `A2ACLI_OUT_002` | `--output json` — exactly one document, the **terminal** protocol object rather than an event log, and never switched implicitly to `jsonl` | §9.3, App. B | `<>` | |
| `A2ACLI_OUT_003` | `--output jsonl` — one complete JSON object per line, flushed as produced | §9.3, App. B | `<>` | |
| `A2ACLI_OUT_004` | Errors are machine-readable and consistent across transports: protocol failures carry the A2A error name, CLI-local failures an `A2ACLI_ERR_*` code | §9.4, App. E | `<>` | |
| `A2ACLI_EXIT_001` | Implements the three required exit statuses (`0`, `1`, `2`); any reserved status it emits carries the documented meaning and agrees with the error reported | §9.6, App. E | `<>` | |
| `A2ACLI_AUTH_001` | Scriptable credentials — bearer, API key, env equivalents, attached as service parameters; `-H/--header` available separately for any service parameter | §10.1 | `<>` | |
| `A2ACLI_TX_001` | Transport selected from the Agent Card, honoring declared preference | §11.1 | `<>` | |
| `A2ACLI_TX_002` | Uses the first `supported_interfaces` entry it supports absent a client preference; `--transport` is repeatable and ordered | §11.1, §4.5 | `<>` | |
| `A2ACLI_VER_001` | Protocol version signaled explicitly on every request; negotiates down only within 1.x, never below 1.0; no silent downgrade | §11.2 | `<>` | |
| `A2ACLI_DEFAULT_001` | Ships the baseline defaults, each overridable by an explicit flag. **Complete the breakdown below** — a bare pass hides which default is missing | §4.5 | `<>` | |
| `A2ACLI_SKILL_001` | *(Conditional — mark `— N/A` if the tool ships no skill.)* Ships **exactly one** Agent Skill, generic and token-efficient, deferring to runtime `help` rather than inlining the command surface | §12.1, §12.2 | `<>` | |
| `A2ACLI_SKILL_002` | *(Conditional — mark `— N/A` if the tool ships no skill.)* Skill and specification kept as distinct layers; the skill does not restate normative requirements | §12.3 | `<>` | |

### 4a. `A2ACLI_DEFAULT_001` breakdown

`DEFAULT_001` passes only when every row is present and overridable. Report each one; "mostly defaults" is not a result anyone can act on.

| Default (§4.5) | Shipped? | Overridable by flag? | Notes |
| --- | --- | --- | --- |
| Transport — server preference order | `<>` | `<>` | |
| Task completion — wait by default | `<>` | `<>` | |
| Output — human-readable `text` | `<>` | `<>` | |
| Detail level — concise | `<>` | `<>` | |
| Protocol version — highest mutually supported, never below 1.0 | `<>` | `<>` | |
| Transport security — TLS verification on | `<>` | `<>` | |

## 5. Tier 2 — Standard

| ID | Requirement | Spec § | Status | Notes |
| --- | --- | --- | --- | --- |
| `A2ACLI_TASK_LIST_001` | `task list` — cursor-paginated, filterable by status and context | §8.5 | `<>` | |
| `A2ACLI_SUB_001` | `subscribe` — (re)subscribe to a task's event stream | §8.5 | `<>` | |
| `A2ACLI_SUB_002` | Stream resumption after disconnect, reconciled with `get` | §7.4 | `<>` | |
| `A2ACLI_AUTH_002` | `auth login` — OAuth 2.1 device-code flow | §10.2 | `<>` | |
| `A2ACLI_AUTH_003` | `auth login` — OAuth 2.1 client-credentials flow | §10.2 | `<>` | |
| `A2ACLI_AUTH_004` | Secure token storage with automatic attachment on later calls | §10.2 | `<>` | |
| `A2ACLI_TX_003` | At least two transports supported and selectable | §11.1 | `<>` | |
| `A2ACLI_CONFIG_001` | Configuration precedence (flag → env → local → global → built-in), scopeable by agent-card reference | §6.4, §8.5 | `<>` | |

| `A2ACLI_DOWNLOAD_001` | `download` — save task artifacts to disk | §8.5 | `<>` | |
| `A2ACLI_OUT_005` | `--debug` enables diagnostic logging; `--dump-wire` emits raw protocol JSON to stderr | §8.5 | `<>` | |
| `A2ACLI_CONFORM_001` | `conformance` — smoke-check a live agent against the A2A TCK | §8.5 | `<>` | |
| `A2ACLI_OUT_006` | Shell completions provided | §8.5 | `<>` | |
| `A2ACLI_PUSH_001` | `push-config` create / get / list / delete | §8.5 | `<>` | |

## 6. Tier 3 — Advanced

| ID | Requirement | Spec § | Status | Notes |
| --- | --- | --- | --- | --- |

| `A2ACLI_PUSH_002` | Local webhook receiver able to accept push notifications | §8.5, §7.2 | `<>` | |
| `A2ACLI_CHAT_001` | Interactive `chat` carrying context and task across turns | §6.2, §8.5 | `<>` | |
| `A2ACLI_TX_004` | gRPC transport | §11.1 | `<>` | |
| `A2ACLI_INSPECT_002` | Authenticated extended Agent Card | §8.5, §10.4 | `<>` | |
| `A2ACLI_INSPECT_003` | Agent Card signature verification | §8.5 | `<>` | |
| `A2ACLI_AUTH_005` | Mutual TLS | §10.3 | `<>` | |
| `A2ACLI_AUTH_006` | OpenID Connect | §10.3 | `<>` | |
| `A2ACLI_AUTH_007` | Handles in-task `AUTH_REQUIRED` resolution | §10.3 | `<>` | |
| `A2ACLI_SERVE_001` | `serve` / mock agent mode | §8.5 | `<>` | |
| `A2ACLI_INSPECT_004` | Catalog / registry integration | §8.5 | `<>` | |
| `A2ACLI_VER_002` | Declares server-required protocol extensions | §11.3 | `<>` | |

## 7. Test evidence

- **How the tool was exercised:** `<command(s) / TCK invocation>`
- **Results / logs:** `<link to CI run, TCK output, or transcript>`
- **Environment:** `<OS, runtime versions, target agent>`
- **Known gaps / caveats:** `<free text>`

## 8. Error code coverage *(optional but recommended)*

List which `A2ACLI_ERR_*` codes (Appendix E) the tool can emit. Helps consumers write reliable error handling.

| Code | Emitted | Notes |
| --- | --- | --- |
| `<A2ACLI_ERR_…>` | `<>` | |

## 9. Attestation

Reported by `<name, role>` on `<YYYY-MM-DD>`. The tier claimed in §1 and §3 reflects the evidence above.
