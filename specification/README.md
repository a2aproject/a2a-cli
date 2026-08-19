# a2a-cli Specification

This directory holds the specification for **`a2a-cli`** — the command-line client for the [Agent2Agent (A2A) Protocol](https://a2a-protocol.org/latest/specification/) — together with the material used to verify an implementation against it.

`a2a-cli` lets developers and AI coding agents fetch Agent Cards, send messages, stream and poll tasks, and read artifacts from the terminal. `SPEC.md` defines how the tool behaves, in terms an implementation in any language can follow.

## Contents

| File | Description |
| --- | --- |
| [`SPEC.md`](./SPEC.md) | The behavior specification (normative). Self-contained: everything needed to build the tool. |
| [`COMPLIANCE.md`](./COMPLIANCE.md) | The conformance model, the requirement-identifier registry, and the report form an implementation completes. |
| [`compliance-report.template.yaml`](./compliance-report.template.yaml) | The same report in machine form — the artifact an implementation publishes. |

## Capability tiers at a glance

`SPEC.md` §5 groups its requirements into three **cumulative** tiers. A tier describes *scope* — what the tool does first, next, and later — not a rank awarded to an implementation.

| Tier | Name | Summary |
| --- | --- | --- |
| **Tier 1** | Core | `card get`, `send`, `task get`, `task cancel`, `help`; explicit interaction state; task polling; the output contract and exit codes; token auth; version signaling. |
| **Tier 2** | Standard | `task list`, `task subscribe`, `auth login`, multiple transports, configuration precedence, `task push-config`, `task download`, wire debug, `conformance`, `completion`. |
| **Tier 3** | Advanced | Push-notification webhook receiver, interactive `chat`, gRPC, extended card, signature verification, mTLS, OpenID Connect, `demo-server`/mock, catalog resolution, extensions. |

See [`SPEC.md`](./SPEC.md) for the normative detail.

## Verifying an implementation

Tier membership is a scope decision and lives in `SPEC.md`. Everything about *claiming* a tier — what counts as satisfying a requirement, and the evidence a claim needs — lives in [`COMPLIANCE.md`](./COMPLIANCE.md).

1. Implement a tier of the specification.
2. Exercise the tool against a live A2A agent — ideally one that is itself TCK-conformant, so a failure can be attributed to the client rather than to the agent. The TCK validates *agents*, not clients, so it cannot grade a CLI directly.
3. Complete [`COMPLIANCE.md`](./COMPLIANCE.md), recording an outcome for every requirement identifier in the tier claimed.
4. Publish the machine-readable report alongside the implementation.

The registry is open: any implementation, in any language, can measure itself against it and publish the result.

## Status

**v0.2 — Review.** Open for comment; pre-Proposed and not yet ratified, so normative requirements may still change (`SPEC.md` §15.1, §15.3). Feedback is welcome via issues and pull requests.

## References

- A2A Protocol Specification v1.0 — https://a2a-protocol.org/latest/specification/
- A2A Agent Discovery — https://a2a-protocol.org/latest/topics/agent-discovery/
- A2A Streaming & Asynchronous Operations — https://a2a-protocol.org/latest/topics/streaming-and-async/
