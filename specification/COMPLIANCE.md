# a2a-cli Compliance Report

**Status:** Review — a pre-Proposed state (`SPEC.md` §15.1).
**Last updated:** 2026-08-17
**Applies to:** `SPEC.md` v0.2 (revised 2026-08-17)

## About this document

This file is **self-contained**. It defines what an `a2a-cli` compliance report is, the full registry of requirement identifiers, the legend, and how to fill and publish a report — you can read and complete it without opening any other document. Where a requirement's precise wording matters, each row cites the governing section of the specification (`SPEC.md`, referenced as "§x"); `SPEC.md` remains the normative authority for behaviour, and this file builds on it.

It serves two roles at once:

- **In the specification repository** it is the **authoritative registry of requirement identifiers** (`SPEC.md` §3.3): the identifiers `A2ACLI_<AREA>_<NNN>` and the error codes `A2ACLI_ERR_*` are *defined* by the tables below.
- **In a tool's repository** it is a **fill-in report template**: copy it, complete every field, and link the finished report from the A2A compatibility matrix.

**Goal.** A compliance report exists to show, per requirement, that a CLI **sufficiently covers the features developers and users need** to drive an A2A agent from a terminal: discover an agent, send a message, follow the task, read artifacts, script it, and recover state. Each requirement identifier is one such feature made measurable. A filled report makes coverage visible: what a tool does, what it does not do yet, and (for contributors) exactly which gaps are open to pick up. A tool MUST NOT advertise a tier it has not demonstrated here.

**Fill order.** (1) `§1` identity; (2) each tier table `§4`–`§6`, completing the `DEFAULT_001` breakdown in `§4a`; (3) tally `§3` last, though it appears first; (4) set **Tier claimed** to the highest tier fully satisfied; (5) list error-code coverage `§8`; (6) sign `§9`. The machine-readable form of the same report is defined in **Appendix A** and shipped as `compliance-report.template.yaml` beside this file.

<!-- ─────────────────────────────────────────────────────────────────────────
     REMOVE THIS BLOCK BEFORE PUBLISHING A FILLED REPORT.
     It is registry policy that belongs to the master copy in the
     specification repository, not to any individual tool's report.
     ───────────────────────────────────────────────────────────────────────── -->

> **Identifier stability.** An identifier is never **reused** (a retired number never returns meaning something else), but **renumbering is permitted while the specification is pre-Proposed (Draft or Review, `SPEC.md` §15.1)** and freezes from its first Proposed version. Tier membership is *not* encoded in the identifier, so a requirement can move tiers and still be tracked by the same ID. From Proposed onward, withdrawn requirements stay listed and marked `Withdrawn`.
>
> **Numbers already spent.** `A2ACLI_OUT_008` (shell completions) was renumbered to `A2ACLI_CLI_002` while pre-Proposed, when the `CLI` area was added for the tool's own surface; `A2ACLI_SERVE_001` was renamed to `A2ACLI_DEMO_SERVER_001` when the command it covers was renamed `serve` → `demo-server` to avoid reading as a generic "server" area. Neither `OUT_008` nor `SERVE` is reused; both are retired for good.
>
> **The list is expected to grow.** If you are building a tool and hit a real use case that no requirement covers, open a request against the specification repository; it can be added in a future revision. Adding requirements never changes existing identifiers, so reports and test suites that cite them keep working.
>
> **On authentication.** Authentication, security, and compliance are large topics a checklist row cannot fully treat. The `A2ACLI_AUTH_*` requirements cover the ground the specification defines today and are expected to expand. Treat current coverage as a starting point, not a complete security review.
>
> **On errors.** Failures come in two layers (`SPEC.md` §9.4): a protocol failure carries the A2A error by name (A2A §3.3.2, mapped per §5.4), and a CLI-local failure carries an `A2ACLI_ERR_*` identifier from `SPEC.md` Appendix E. A tool that renames protocol errors into a vocabulary of its own does not satisfy `A2ACLI_OUT_004`.

<!-- ──────────────────────── END REMOVE-BEFORE-PUBLISHING ──────────────────── -->

## 1. Summary

| Field | Value |
| --- | --- |
| Tool name | `<tool>` |
| Tool version | `<x.y.z>` |
| Implementation language | `<Go / Rust / Python / …>` |
| Repository | `<url>` |
| Maintainer / contact | `<name or org>` |
| Report date | `<YYYY-MM-DD>` |
| Specification version targeted | `0.2` |
| Specification revision (Last updated) | `<YYYY-MM-DD from SPEC.md header>` |
| **Tier claimed** | `<Tier 1 / Tier 2 / Tier 3>` |
| A2A protocol version(s) | `<e.g. 1.0>` |
| Agent exercised against | `<name / URL>` |
| That agent is TCK-conformant? | `<yes / no / unknown>` |
| Transports covered | `<HTTP+JSON / JSON-RPC / gRPC>` |
| Designation | `conformant` |

*Designation is `conformant` for a self-reported compliance report. "Official" is a project decision recorded in the A2A compatibility matrix, never self-asserted here (`SPEC.md` §1.4, §15.3).*

## 2. Legend

| Mark | Meaning |
| --- | --- |
| `✅ Pass` | Implemented and verified |
| `◐ Partial` | Partially implemented — state what is missing |
| `❌ Fail` | Not implemented, or does not behave as specified |
| `⊘ Not measured` | Applicable, but could not be exercised (e.g. the agent never produced the required state). **Not a pass** — it blocks a clean tier claim (`SPEC.md` §13.1) |
| `— N/A` | Inapplicable to this tool (e.g. it ships no skill); state why. Does **not** block tier satisfaction |
| `⊗ Withdrawn` | A retired requirement, kept listed so its ID is never reused (`SPEC.md` §3.3). Applies only from the first **Proposed** version; unused while the spec is pre-Proposed. Excluded from tier totals; never counted for or against satisfaction |

Every `◐`, `❌`, `⊘`, `—`, or `⊗` MUST carry a note. An unobservable requirement and a satisfied one are different results (`SPEC.md` §13.1) — never mark `⊘` as `✅`.

`— N/A` hinges on the **tool**, not the **agent**: use it only for a requirement that does not apply to the tool itself (e.g. skill rows when it ships no skill). A requirement the tool implements but the **test agent** could not exercise (it never emits push notifications, files, or an interrupted state) stays `⊘`. The fix is to report against an agent that can exercise the tier's requirements (`SPEC.md` §3.2, §13.1), not to downgrade it to `— N/A`.

**Requirement kinds.** Every listed requirement is one of:

- **Gating** (default) — MUST be `✅` (or a correct `— N/A`, for a conditional one) for its tier to be satisfied.
- **Conditional** — tagged in its row as *(Conditional — `— N/A` if …)*; a correct `— N/A` does not block the tier.

A future revision MAY introduce **optional (capability-badge)** requirements — claimable but non-gating — should the Tier-3 model move to a base-plus-badges shape (an open question). None exist today, so every requirement currently listed **gates** its tier (`SPEC.md` §3.1).

## 3. Results at a glance

| Tier | Requirements | `✅` | `◐` | `❌` | `⊘` | `—` | Tier satisfied? |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Tier 1 — Core | 39 | `<>` | `<>` | `<>` | `<>` | `<>` | `<yes/no>` |
| Tier 2 — Standard | 15 | `<>` | `<>` | `<>` | `<>` | `<>` | `<yes/no>` |
| Tier 3 — Advanced | 12 | `<>` | `<>` | `<>` | `<>` | `<>` | `<yes/no>` |

Use `yes`/`no` in the last column and integer counts elsewhere. Each row's five mark-counts MUST sum to its **Requirements** total. Withdrawn requirements (once any exist) are retired from the registry's active set and are excluded from that total, so they neither help nor block a tier.

A tier is satisfied when every **applicable** requirement in it is `✅`. A conditional requirement marked `— N/A` (e.g. the tool ships no skill) does not block satisfaction; a `⊘`, `◐`, or `❌` does. Tiers are cumulative — Tier 2 requires Tier 1, Tier 3 requires Tier 2 (`SPEC.md` §3.1).

## 4. Tier 1 — Core (required)

| ID | Requirement | Spec § | Status | Notes |
| --- | --- | --- | --- | --- |
| `A2ACLI_CARD_GET_001` | `card get` — resolve and parse an Agent Card from a host, an explicit URL, or a `file://` path, and use it to select a transport | §8.1, §11 | `<>` | |
| `A2ACLI_SEND_001` | Send a message to start an interaction | §8.2 | `<>` | |
| `A2ACLI_SEND_002` | Blocking by default; `--async` returns identifiers immediately (`--return-immediately` / `--no-wait` are OPTIONAL aliases) | §8.2, §4.5 | `<>` | |
| `A2ACLI_SEND_003` | `--stream` consumes SSE when supported and never hangs when unsupported; a `Message`-only response (no task created) exits cleanly rather than erroring | §8.2, §7.2 | `<>` | |
| `A2ACLI_SEND_004` | Renders produced artifacts; never silently discards a part | §8.2 | `<>` | |
| `A2ACLI_SEND_005` | Lets the caller set a message part's media type explicitly with `--media-type`, bound to the part flag it follows; infers the type only when none was given | §8.2 | `<>` | |
| `A2ACLI_SEND_006` | Message-part flags `--text` / `--file` / `--data` are repeatable and order-preserving, so one message can carry multiple ordered parts (`--data -` reads stdin) | §8.2 | `<>` | |
| `A2ACLI_TASK_GET_001` | `task get` — retrieve task state, artifacts, and history by identifier; renders returned artifacts and never silently strips them | §8.3 | `<>` | |
| `A2ACLI_TASK_CANCEL_001` | `task cancel` — cancel a task; idempotent; reports the resulting state | §8.4 | `<>` | |
| `A2ACLI_INTERACT_001` | Continue an interaction via `--context-id` | §6.2 | `<>` | |
| `A2ACLI_INTERACT_002` | Continue a task via `--task-id`, supplied with or without `--context-id` (the server resolves the task's context); when both are given they MUST correspond; a rejected identifier fails non-zero and creates no new task, surfacing the protocol error and pointing at `--debug` | §6.2 | `<>` | |
| `A2ACLI_INTERACT_003` | Never invents server-assigned identifiers, and never assumes `contextId` denotes a chat session | §6.1, §2 | `<>` | |
| `A2ACLI_INTERACT_004` | Reports `taskId` / `contextId` / `state` on completion and interruption, in copy-pasteable form, and prints the exact command to resume | §6.3 | `<>` | |
| `A2ACLI_INTERACT_005` | Stateless: never stores the last `taskId` / `contextId` to replay on the caller's behalf; provides no `--continue`; interaction state never lives only in process memory | §4.3, §6.1, §6.4 | `<>` | |
| `A2ACLI_TASK_POLL_001` | Polling path available — one-shot `task get`, plus `task get --wait` polling to a terminal or interrupted state | §7.3 | `<>` | |
| `A2ACLI_TASK_POLL_002` | `--poll-interval` and `--timeout` honored; bounded backoff; no busy-loop; remains interruptible without losing the already-printed `taskId` | §7.3 | `<>` | |
| `A2ACLI_TASK_POLL_003` | Stops immediately on interrupted states (`INPUT_REQUIRED` / `AUTH_REQUIRED`) without deadlock; treats `TASK_STATE_UNSPECIFIED` as neither terminal nor interrupted, continuing to poll under `--timeout` | §7.1, §7.3 | `<>` | |
| `A2ACLI_TASK_POLL_004` | When a wait prefers streaming, falls back to polling on stream failure and drives the task to a terminal/interrupted state | §7.3 | `<>` | |
| `A2ACLI_OUT_001` | **Standard output** — every response about a task names its context, its task, and its state, in every output mode; the payload goes to stdout and diagnostics to stderr, never mixed | §9.1, §9.5, §6.3 | `<>` | |
| `A2ACLI_OUT_002` | `-o json` (no `--stream`) — exactly one document, the **terminal** protocol object rather than an event log, and never switched implicitly to its streamed JSONL form | §9.3, App. B | `<>` | |
| `A2ACLI_OUT_003` | `-o json --stream` — JSONL: one complete JSON object per line, flushed as produced, final line carrying the terminal object; a stream-terminating error is emitted as a final error object on its own line | §9.3, §9.4, App. B | `<>` | |
| `A2ACLI_OUT_004` | Errors are machine-readable and consistent across transports: protocol failures carry the A2A error name, CLI-local failures an `A2ACLI_ERR_*` code; the tool never invents codes in the `A2ACLI_ERR_*` namespace (vendor codes use a distinct prefix) and SHOULD populate the `hint` field | §9.4, App. B, App. E | `<>` | |
| `A2ACLI_OUT_005` | **`text` floor** — one `Label: value` field per line, the same labels across invocations, no terminal control sequences; block content (a rendered artifact, a formatted data part) sits under its own `Label:` line, closed by a blank line and never interleaved with field lines; any interactive mode auto-degrades to `text` when stdout is not a TTY and never blocks on interactive input there | §9.2, §4.1 | `<>` | |
| `A2ACLI_OUT_006` | When the caller does not wait (`--async`), still emits a result object carrying at least `taskId` and `contextId` for later polling | §9.5 | `<>` | |
| `A2ACLI_EXIT_001` | Implements the three required exit statuses (`0`, `1`, `2`); any reserved status it emits carries the documented meaning and agrees with the error reported | §9.6, App. E | `<>` | |
| `A2ACLI_EXIT_002` | Tool execution and agent outcome stay decoupled: a turn the CLI conducted and reported exits `0` even when the task ends `FAILED`/`REJECTED` or pauses at `INPUT_REQUIRED`/`AUTH_REQUIRED`; the outcome is carried in the task state, and a non-success or paused outcome SHOULD be named in a stderr warning | §4.6, §9.6 | `<>` | |
| `A2ACLI_AUTH_001` | Scriptable credentials — bearer, API key, env equivalents, attached per the agent's declared security scheme; `--svc-param` available separately for any service parameter, and never documented as an authentication flag | §10.1 | `<>` | |
| `A2ACLI_AUTH_002` | Offers an environment-variable equivalent for each credential flag and documents that a flag-supplied credential is exposed via the process table and shell history; this guidance does not alter the flag > environment precedence (§4.5) | §10.1, §4.5 | `<>` | |
| `A2ACLI_AUTH_003` | Emits a prominent security warning to stderr when a credential is sent over a connection with certificate verification disabled (`--insecure`); never disables TLS verification silently | §10.1, §4.5 | `<>` | |
| `A2ACLI_AUTH_004` | Redacts credential material from diagnostic output, including `--debug` raw-wire logging; the redaction is not defeasible by a verbosity flag | §10.1, §5.2 | `<>` | |
| `A2ACLI_TX_001` | Transport selected from the Agent Card, honoring declared preference order | §11.1 | `<>` | |
| `A2ACLI_TX_002` | Uses the first `supportedInterfaces` entry it supports absent a client preference; `--transport` is repeatable and ordered | §11.1, §4.5 | `<>` | |
| `A2ACLI_TX_003` | *(Conditional — `— N/A` if the selected interface declares no `tenant`.)* Sets the selected `AgentInterface`'s routing identifier (`tenant`) in every request message, exactly as declared, and omits the field when the entry declares none | §11.1 | `<>` | |
| `A2ACLI_VER_001` | `A2A-Version` signaled explicitly on every request (never left empty, which A2A reads as 0.3); negotiates down only within 1.x, never below 1.0; no silent downgrade | §11.2 | `<>` | |
| `A2ACLI_DEFAULT_001` | Ships the baseline defaults, each overridable by an explicit flag, and exposes its effective defaults (e.g. via `--help`). **Complete the breakdown in §4a** — a bare pass hides which default is missing | §4.5 | `<>` | |
| `A2ACLI_CONFIG_001` | Persisted configuration *(if any)*: conventional path, secrets not world-readable (mode `0600` or platform equivalent), inspectable via the read-only `config show` and directly editable and removable by the user; never records session state (`taskId` / `contextId`) to offer resume | §6.4 | `<>` | |
| `A2ACLI_CLI_001` | `help` and `<command> --help` print usage and exit; `-v/--version` prints the tool version; `--help` shows the effective defaults so a caller can see them before overriding | §5.1, §5.2, §4.5 | `<>` | |
| `A2ACLI_SKILL_001` | *(Conditional — mark `— N/A` if the tool ships no skill.)* Ships **exactly one** Agent Skill, generic and token-efficient, deferring to runtime `help` rather than inlining the command surface | §12.1, §12.2 | `<>` | |
| `A2ACLI_SKILL_002` | *(Conditional — mark `— N/A` if the tool ships no skill.)* Skill and specification kept as distinct layers; the skill does not restate normative requirements | §12.3 | `<>` | |

### 4a. `A2ACLI_DEFAULT_001` breakdown

`DEFAULT_001` passes only when every row is present and overridable. Report each one; "mostly defaults" is not a result anyone can act on. This sub-table is the evidence behind the `DEFAULT_001` status cell above — keep the two consistent.

| Default (§4.5) | Shipped? (yes/no) | Overridable by flag? (yes/no) | Notes |
| --- | --- | --- | --- |
| Transport — server preference order | `<>` | `<>` | |
| Task completion — wait by default | `<>` | `<>` | |
| Output — human-readable `text` | `<>` | `<>` | |
| Detail level — concise | `<>` | `<>` | |
| Protocol version — highest mutually supported, never below 1.0 | `<>` | `<>` | |
| Transport security — TLS verification on | `<>` | `<>` | MUST warn when `--insecure` disables it |

## 5. Tier 2 — Standard

| ID | Requirement | Spec § | Status | Notes |
| --- | --- | --- | --- | --- |
| `A2ACLI_CARD_GET_002` | `card get --validate` — validate the Agent Card against the A2A schema | §8.1 | `<>` | |
| `A2ACLI_TASK_LIST_001` | `task list` — cursor-paginated, filterable by status and context | §5.1, App. A | `<>` | |
| `A2ACLI_TASK_SUBSCRIBE_001` | `task subscribe` — (re)subscribe to a task's event stream | §5.1, §7.2 | `<>` | |
| `A2ACLI_TASK_SUBSCRIBE_002` | Stream resumption after disconnect; the first event re-delivers the full `Task`, so no separate `get` is required | §7.4, §7.2 | `<>` | |
| `A2ACLI_AUTH_005` | `auth login` — OAuth 2.1 device-code flow | §10.2 | `<>` | |
| `A2ACLI_AUTH_006` | `auth login` — OAuth 2.1 client-credentials flow | §10.2 | `<>` | |
| `A2ACLI_AUTH_007` | Secure token storage with automatic attachment on later calls | §10.2 | `<>` | |
| `A2ACLI_TX_004` | At least two transports supported and selectable | §11.1 | `<>` | |
| `A2ACLI_VER_003` | Verifies a capability on the Agent Card before invoking a capability-gated operation (streaming, push, extended card) | §11.3 | `<>` | |
| `A2ACLI_CONFIG_002` | Configuration precedence (flag → env → local → global → built-in), scopeable by agent-card reference; `config show` reports each effective value with the source it resolved from | §6.4 | `<>` | |
| `A2ACLI_DOWNLOAD_001` | `task download` — save task artifacts to disk | §5.1 | `<>` | |
| `A2ACLI_OUT_007` | `--debug` enables diagnostic logging to stderr, including the raw protocol messages exchanged on the wire | §5.2 | `<>` | |
| `A2ACLI_CONFORM_001` | `conformance` — smoke-check a live agent against the A2A TCK | §5.1 | `<>` | |
| `A2ACLI_CLI_002` | `completion <shell>` — emits a shell completion script for the named shell | §5.1 | `<>` | |
| `A2ACLI_PUSH_001` | `task push-config` create / get / list / delete | §5.1, App. A | `<>` | |

## 6. Tier 3 — Advanced

| ID | Requirement | Spec § | Status | Notes |
| --- | --- | --- | --- | --- |
| `A2ACLI_CARD_GET_003` | Authenticated extended Agent Card, fetched only via a security scheme advertised on the public card | §8.1, §10.4 | `<>` | |
| `A2ACLI_AUTH_008` | Never presents a credential to an Agent Card endpoint that has not declared a scheme accepting it | §10.4 | `<>` | |
| `A2ACLI_CARD_GET_004` | Agent Card signature verification per A2A §8.4.3 — verifies signatures when the card carries them and reports the outcome (verified / unverifiable / absent); never presents an unverified card as verified, and does not fail on an absent signature (A2A §8.4 makes signing optional) | §8.1 | `<>` | |
| `A2ACLI_CARD_GET_005` | Catalog / registry resolution — accepts a catalog or registry entry wherever `--agent-card` takes a reference and resolves it to an Agent Card before any other operation | §8.1 | `<>` | |
| `A2ACLI_PUSH_002` | Local webhook receiver able to accept push notifications | §7.2 | `<>` | |
| `A2ACLI_CHAT_001` | Interactive `chat` carrying context and task across turns | §6.2, §5.1 | `<>` | |
| `A2ACLI_TX_005` | gRPC transport | §11.1 | `<>` | |
| `A2ACLI_AUTH_009` | Mutual TLS | §10.3 | `<>` | |
| `A2ACLI_AUTH_010` | OpenID Connect | §10.3 | `<>` | |
| `A2ACLI_AUTH_011` | Handles in-task `AUTH_REQUIRED` resolution | §10.3 | `<>` | |
| `A2ACLI_DEMO_SERVER_001` | `demo-server` / mock agent mode | §5.1, §14 | `<>` | |
| `A2ACLI_VER_002` | Declares server-required protocol extensions | §11.3, A2A §4.6 | `<>` | |

## 7. Test evidence

- **How the tool was exercised:** `<command(s) / TCK invocation>`
- **Results / logs:** `<link to CI run, TCK output, or transcript>`
- **Environment:** `<OS, runtime versions, target agent>`
- **Known gaps / caveats:** `<free text>`

## 8. Error code coverage *(optional but recommended)*

Which `A2ACLI_ERR_*` codes (`SPEC.md` Appendix E) the tool can emit. Helps consumers write reliable error handling. Mark `Emitted` as `yes`/`no`.

| Code | Emitted (yes/no) | Notes |
| --- | --- | --- |
| `A2ACLI_ERR_USAGE` | `<>` | |
| `A2ACLI_ERR_CARD_NOT_FOUND` | `<>` | |
| `A2ACLI_ERR_CARD_INVALID` | `<>` | |
| `A2ACLI_ERR_UNREACHABLE` | `<>` | |
| `A2ACLI_ERR_CREDENTIALS_MISSING` | `<>` | |
| `A2ACLI_ERR_AUTH_FAILED` | `<>` | |
| `A2ACLI_ERR_TIMEOUT` | `<>` | |
| `A2ACLI_ERR_INTERNAL` | `<>` | |

## 9. Attestation

Reported by `<name, role>` on `<YYYY-MM-DD>`. The tier claimed in §1 and §3 reflects the evidence above.

---

## Appendix A — Machine-readable report (the published artifact)

A published report MUST also be available as a **single, self-contained YAML file** — the one artifact a tool links from the A2A compatibility matrix. The Markdown registry above stays canonical; the YAML is the same measurement in machine form. A ready-to-fill copy ships beside this file as **`compliance-report.template.yaml`**.

*Producing the file is out of scope of this standard* — how a tool runs its checks and assembles the result is an implementer concern. (A common approach: parallel tests each write an independent per-ID section to a temporary directory, one file per ID, atomically via a temp file plus `os.replace`; an aggregation step then sorts by ID and emits the single report with its summary. That is a non-normative note, carrying no MUSTs.)

### A.1 Structure

- **Top `report:` block** — identity: tool, version, language, repository, maintainer, report date, spec version, **spec revision date**, A2A versions, agent tested + its TCK status, transports covered, tier claimed, and `designation`.
- **`summary:`** — aggregator-produced per-tier rollup (counts + `satisfied`).
- **`requirements:`** — an **ID-keyed mapping**: each requirement identifier is a key, carrying `tier`, `area`, `spec`, a short human-readable `requirement` line, `status`, `note`, and optional `evidence`.
- **`default_001_breakdown:`**, **`error_codes:`**, **`attestation:`** — as in §4a, §8, §9.

### A.2 Closed status vocabulary (words)

`pass` · `partial` · `fail` · `not_measured` · `na` · `withdrawn` — mapping 1:1 to the legend (§2): `✅ ◐ ❌ ⊘ — ⊗`. Every status other than `pass` MUST carry a `note`. `withdrawn` applies only from the first Proposed version and is excluded from tier totals and from satisfaction (§A.3).

### A.3 Fail-closed rules (a broken or empty report is never a pass)

These make a missing or malformed report impossible to mistake for a compliant tool (mirrors `SPEC.md` §13.1):

1. **Every requirement identifier for the claimed tier — and every lower tier, since tiers are cumulative — MUST be present** in `requirements:`. A missing key, an empty `requirements:` mapping, or an all-`not_measured` report MUST roll up to `satisfied: false`, never a pass.
2. **Only `pass` (and a genuine `na`) satisfies.** `partial`, `fail`, `not_measured`, and **any unrecognized status value** MUST be treated as not-satisfied — never silently skipped.
3. **`designation` is always `conformant`.** "Official" is a project decision (§1.4) and MUST NOT be self-asserted in this field.
4. **`DEFAULT_001` cannot pass without its breakdown.** `A2ACLI_DEFAULT_001: pass` is invalid unless every `default_001_breakdown` row is `shipped: yes` and `overridable: yes`.
5. **The file is published.** Fill with placeholders only — never a real filled example — and keep `note`/`evidence` publication-safe: no secrets, no internal-only URLs.
6. **Withdrawn is retirement, not a result.** A `withdrawn` requirement (from the first Proposed version onward) keeps its identifier forever, is excluded from its tier's `total`, and is counted neither as a pass nor as a not-satisfied — it is exempt from rule 2. Its ID MUST NOT be reused (`SPEC.md` §3.3).

### A.4 Schema sketch

The shape in brief; the complete, fill-in copy ships as `compliance-report.template.yaml`. Repeated blocks are shown once and elided.

```yaml
report:
  tool: <tool-name>
  tool_version: <x.y.z>
  language: <Go | Rust | Python | ...>
  repository: <url>
  maintainer: <name or org>
  report_date: <YYYY-MM-DD>        # when this report was produced
  spec_version: "0.2"              # SPEC.md "Version"
  spec_revision: <YYYY-MM-DD>      # SPEC.md "Last updated" — pins a pre-Proposed (Draft/Review) result
  a2a_versions: ["1.0"]
  agent_tested: <name or URL>
  agent_tck_conformant: unknown    # yes | no | unknown
  transports_covered: [HTTP+JSON, JSON-RPC]
  tier_claimed: <1 | 2 | 3>
  designation: conformant          # ALWAYS "conformant" (§A.3)

summary:                           # aggregator-produced; only pass/na satisfy (§A.3)
  tier_1: {total: 39, pass: 0, partial: 0, fail: 0, not_measured: 39, na: 0, satisfied: false}
  # tier_2 (15) and tier_3 (12): same shape

requirements:                      # ID-keyed; every ID of the claimed tier MUST appear
  A2ACLI_CARD_GET_001:
    tier: 1
    area: CARD_GET
    spec: "§8.1, §11"
    requirement: "card get — resolve/parse an Agent Card; select a transport"
    status: not_measured           # closed vocabulary: §A.2
    note: "<REQUIRED unless status is pass>"
    evidence: null                 # optional: log path / CI link
  # … one block per identifier; full set in compliance-report.template.yaml

default_001_breakdown:             # all six §4.5 defaults (see §4a)
  transport_server_preference_order: {shipped: no, overridable: no, note: null}
  # … five more rows, same shape …

error_codes:                       # all eight A2ACLI_ERR_* (see §8)
  A2ACLI_ERR_USAGE: {emitted: no, note: null}
  # … seven more, same shape …

attestation:
  reported_by: <name, role>
  date: <YYYY-MM-DD>
  statement: "The tier claimed reflects the evidence recorded above."
```
