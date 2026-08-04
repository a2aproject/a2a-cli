# a2a-cli Specification

This directory holds the specification for **`a2a-cli`** — a command-line client for the [Agent2Agent (A2A) Protocol](https://a2a-protocol.org/latest/specification/) — together with the compliance-report template used to demonstrate conformance.

`a2a-cli` lets developers and AI coding agents discover, message, stream, poll, and inspect A2A agents from the terminal. The specification defines the behavior a tool must exhibit so that independently built CLIs — in any language — converge on one predictable command surface, output contract, and conversation model, verifiable through a published compliance report.

## Contents

| File | Description |
| --- | --- |
| [`SPEC.md`](./SPEC.md) | The a2a-cli behavior specification (normative). |
| [`COMPLIANCE.md`](./COMPLIANCE.md) | A template a tool completes to declare and evidence its conformance. |

## Conformance at a glance

Conformance is tiered and cumulative; a tier is satisfied only when every MUST in it is satisfied.

| Tier | Name | Summary |
| --- | --- | --- |
| **Tier 1** | Core | Discover, send, get, cancel; multi-turn conversation state; task polling; structured output + exit codes; token auth; version signaling; a `SKILL.md`. |
| **Tier 2** | Standard | Task listing, streaming subscribe, OAuth login, multiple transports, config profiles, interactive chat, artifact download, wire debug, conformance check, completions. |
| **Tier 3** | Advanced | Push notifications + webhook receiver, gRPC, extended card, signature verification, mTLS, OpenID Connect, serve/mock, catalog, extensions. |

See [`SPEC.md`](./SPEC.md) for the normative detail.

## Claiming conformance

1. Implement a tier of the specification.
2. Exercise your tool against the A2A Technology Compatibility Kit (TCK) and `SPEC.md`.
3. Copy [`COMPLIANCE.md`](./COMPLIANCE.md) into your tool's repository and fill it in.
4. Submit it to be listed in the A2A **compatibility matrix**.

Conformance is open to any implementation, in any language. "Official" is a separate, project-level designation (see `SPEC.md` §1.4 and §15.3) made outside the specification; it does not restrict or privilege conformance.

## Status

**v0.1 — Draft.** Open for review; not yet ratified. Feedback is welcome via issues and pull requests.

## References

- A2A Protocol Specification v1.0 — https://a2a-protocol.org/latest/specification/
- A2A Agent Discovery — https://a2a-protocol.org/latest/topics/agent-discovery/
- A2A Streaming & Asynchronous Operations — https://a2a-protocol.org/latest/topics/streaming-and-async/
