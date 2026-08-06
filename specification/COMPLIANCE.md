# a2a-cli Compliance Report

> **Template.** Copy this file into your tool's repository, complete every field, and replace all `<…>` placeholders. Link the finished report from the A2A compatibility matrix. A tool MUST NOT advertise a tier it has not demonstrated here.

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
| A2A TCK version(s) tested | `<e.g. 1.0.0>` |
| Transports covered | `<HTTP+JSON / JSON-RPC / gRPC>` |
| Designation | `<conformant / official (project-designated)>` |

## 2. Legend

`✅ Pass` · `◐ Partial` · `❌ Fail` · `— N/A`. Every `◐`/`❌`/`—` MUST carry a note.

## 3. Tier 1 — Core (required)

| # | Requirement | Spec § | Status | Notes |
| --- | --- | --- | --- | --- |
| 1 | `discover` — fetch & parse Agent Card | §8.1 | `<>` | |
| 2 | `send` — start/continue; blocking by default with `--async` override | §8.2, §4.5 | `<>` | |
| 3 | `send --stream` — SSE when supported; no hang when unsupported | §8.2, §7.2 | `<>` | |
| 4 | `get` — task state, artifacts, history | §8.3 | `<>` | |
| 5 | `cancel` — idempotent cancel | §8.4 | `<>` | |
| 6 | Continuation via `--context-id` / `--task-id` (no invented IDs; no silent new task) | §6.1, §6.2 | `<>` | |
| 7 | Reports `taskId` / `contextId` / `state` back on completion & interruption | §6.3 | `<>` | |
| 8 | Session persistence *(if state is persisted)* — conventional path, secrets not world-readable (`0600`), explicit flags override stored state | §6.4 | `<>` | |
| 9 | Polling path — `get` + wait/watch with `--poll-interval` / `--timeout` | §7.3 | `<>` | |
| 10 | Handles interrupted states (`INPUT_REQUIRED` / `AUTH_REQUIRED`) without deadlock | §7.1, §8.2 | `<>` | |
| 11 | Output contract — structured payload on stdout, diagnostics on stderr, auto-degrade off-TTY | §9.1, §9.2 | `<>` | |
| 12 | `--output json` — exactly one complete document, buffered even when the interaction streams | §9.3 | `<>` | |
| 13 | `--output jsonl` — one complete JSON object per line, flushed as produced | §9.3 | `<>` | |
| 14 | Minimal envelope (`taskId`/`contextId`/`state`, error object) in both machine-readable modes | Appendix B | `<>` | |
| 15 | Errors machine-readable **and normalized across transports** (same A2A error → same result) | §9.4 | `<>` | |
| 16 | Async (`--no-wait`) still emits `taskId` + `contextId` for later polling | §9.5 | `<>` | |
| 17 | Exit-code scheme (0–7) | §9.6 | `<>` | |
| 18 | Tier-1 auth — bearer / API key / custom header (scriptable) | §10.1 | `<>` | |
| 19 | Transport selection from the Agent Card; HTTP+JSON default | §11.1, §4.5 | `<>` | |
| 20 | `A2A-Version` signaled on every request (explicit; no silent downgrade) | §11.2 | `<>` | |
| 21 | Opinionated defaults, each overridable by flag | §4.5 | `<>` | |
| 22 | Ships exactly one lightweight, generic `SKILL.md`; spec & skill kept as distinct layers (skill does not restate normative requirements) | §12.1–§12.3 | `<>` | |

## 4. Tier 2 — Standard

| # | Requirement | Spec § | Status | Notes |
| --- | --- | --- | --- | --- |
| 1 | `list` — cursor-paginated, filter by status/context | §8.5 | `<>` | |
| 2 | `subscribe` — (re)subscribe / stream reconnect | §7.4, §8.5 | `<>` | |
| 3 | `auth login` — OAuth 2.1 device-code + client-credentials; secure token store | §10.2 | `<>` | |
| 4 | ≥ 2 transports selectable from the card | §11.1 | `<>` | |
| 5 | Config profiles / named environments (`--env`) | §6.4, §8.5 | `<>` | |
| 6 | Interactive `chat` — carries context/task across turns | §6.2, §8.5 | `<>` | |
| 7 | `download` — save artifacts | §8.5 | `<>` | |
| 8 | Wire debug (`--dump-wire`) | §8.5 | `<>` | |
| 9 | `conformance` — TCK smoke check | §8.5 | `<>` | |
| 10 | Shell completions | §8.5 | `<>` | |

## 5. Tier 3 — Advanced

| # | Requirement | Spec § | Status | Notes |
| --- | --- | --- | --- | --- |
| 1 | `push-config` CRUD + webhook receiver | §8.5 | `<>` | |
| 2 | gRPC transport | §11.1 | `<>` | |
| 3 | Authenticated extended Agent Card | §8.5, §10.4 | `<>` | |
| 4 | Agent Card signature verification | §8.5 | `<>` | |
| 5 | mTLS | §10.3 | `<>` | |
| 6 | OpenID Connect | §10.3 | `<>` | |
| 7 | `serve` / mock agent | §8.5 | `<>` | |
| 8 | Catalog / registry integration | §8.5 | `<>` | |
| 9 | Protocol extensions | §11.3 | `<>` | |

## 6. Test evidence

- **How the tool was exercised:** `<command(s) / TCK invocation>`
- **Results / logs:** `<link to CI run, TCK output, or transcript>`
- **Environment:** `<OS, runtime versions, target agent>`
- **Known gaps / caveats:** `<free text>`

## 7. Attestation

Reported by `<name, role>` on `<YYYY-MM-DD>`. The tier claimed in §1 reflects the evidence above.
