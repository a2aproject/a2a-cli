# a2a-cli Specification

This directory holds the specification for **`a2a-cli`** — a command-line client for the [Agent2Agent (A2A) Protocol](https://a2a-protocol.org/latest/specification/) — together with the compliance-report template used to demonstrate conformance.

`a2a-cli` lets developers and AI coding agents inspect, message, stream, poll, and manage A2A agents from the terminal. The specification defines the behavior a tool must exhibit so that independently built CLIs — in any language — converge on one predictable command surface, output contract, and interaction model, verifiable through a published compliance report.

## Contents

| File | Description |
| --- | --- |
| [`SPEC.md`](./SPEC.md) | The a2a-cli behavior specification (normative). |
| [`COMPLIANCE.md`](./COMPLIANCE.md) | The requirement registry, and the template a tool completes to declare and evidence its conformance. |
| [`compliance-report.template.yaml`](./compliance-report.template.yaml) | The same report in machine form — the artifact a tool links from the compatibility matrix. |
| [`OPEN-QUESTIONS.md`](./OPEN-QUESTIONS.md) | Questions the specification deliberately leaves open, each with the position it takes today. |

## Conformance at a glance

Conformance is tiered and cumulative. A tier is satisfied only when every applicable requirement listed for it is satisfied — including the ones worded as SHOULD, which a tier claim promotes to required (`SPEC.md` §3.1).

| Tier | Name | Summary |
| --- | --- | --- |
| **Tier 1** | Core | `card get`, `send`, `task get`, `task cancel`, `help`; explicit interaction state; task polling; the output contract + exit codes; token auth; version signaling. |
| **Tier 2** | Standard | `task list`, `task subscribe`, `auth login`, multiple transports, configuration precedence, `task push-config`, `task download`, wire debug, `conformance`, `completion`. |
| **Tier 3** | Advanced | Push-notification webhook receiver, interactive `chat`, gRPC, extended card, signature verification, mTLS, OpenID Connect, `serve`/mock, catalog resolution, extensions. |

See [`SPEC.md`](./SPEC.md) for the normative detail.

## Claiming conformance

1. Implement a tier of the specification.
2. Exercise your tool against a live A2A agent — ideally one that is itself TCK-conformant, so a failure can be attributed to the CLI rather than to the agent. The TCK validates *agents*, not clients, so it cannot grade a CLI directly (`SPEC.md` §3.2).
3. Copy [`COMPLIANCE.md`](./COMPLIANCE.md) into your tool's repository and fill it in, recording an outcome for every requirement identifier in the tier you claim.
4. Publish the machine-readable report and submit it to be listed in the A2A **compatibility matrix**.

Conformance is open to any implementation, in any language. "Official" is a separate, project-level designation (see `SPEC.md` §1.4 and §15.3) made outside the specification; it does not restrict or privilege conformance.

## Status

**v0.2 — Review.** Open for comment; pre-Proposed and not yet ratified, so normative requirements may still change (`SPEC.md` §15.1, §15.4). Feedback is welcome via issues and pull requests.

## References

- A2A Protocol Specification v1.0 — https://a2a-protocol.org/latest/specification/
- A2A Agent Discovery — https://a2a-protocol.org/latest/topics/agent-discovery/
- A2A Streaming & Asynchronous Operations — https://a2a-protocol.org/latest/topics/streaming-and-async/
