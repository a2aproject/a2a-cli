# a2a-cli — Open questions

**Status:** Review — a pre-Proposed state (`SPEC.md` §15.1).
**Last updated:** 2026-08-17

Questions `SPEC.md` deliberately leaves open, each with the position the specification takes *today* so an implementer is never blocked waiting for an answer. The revision history (`SPEC.md` Appendix D) cites these by number.

**Numbering.** An `OQ` number is permanent and is never reused. A number missing from the list below was resolved or withdrawn in an earlier revision; what changed is recorded in `SPEC.md` Appendix D.

| # | Question | Current position | Decided by |
| --- | --- | --- | --- |
| OQ1 | Is the 1.0 protocol-version floor right? | Floor stands (§11.2) | A2A TSC |
| OQ5 | Can the TCK smoke-check an arbitrary live agent? | `conformance` stays Tier 2 (§5.1) | A2A TSC |
| OQ6 | What is the tool's invocation name? | `a2a-cli` throughout, for reference only | A2A TSC |
| OQ9 | How does a caller name the target agent? | `--agent-card <ref>` (§5.2, §8.1) | Specification |
| OQ10 | Is `--media-type` the right shape? | `--media-type` binds to the preceding part flag (§8.2) | Specification |
| OQ11 | Should flag aliases be required or optional? | Canonical required, aliases OPTIONAL (§5.2) | Specification |

## OQ1 — The protocol-version floor

§11.2 requires that a tool never negotiate below A2A **1.0**, treating earlier versions as legacy behind an explicit opt-in. The question is whether a client specification may set a floor the protocol itself does not, and whether agents still serving pre-1.0 make the floor a compatibility problem in practice.

Position: the floor stands. It is cheap to relax later and expensive to add.

## OQ5 — What `conformance` can honestly check

§5.1 lists `conformance` (Tier 2) as a smoke-check of a live agent against the A2A TCK. The premise may be unsound: the TCK validates a *system under test* it controls, not an arbitrary deployed agent, so a CLI wrapping it may be promising something the TCK cannot deliver.

Position: the command stays Tier 2 pending a TSC answer. If the premise fails, the honest fix is to narrow it to the checks a client *can* make from outside (card validity, declared-vs-actual capabilities, transport reachability) and rename it accordingly.

## OQ6 — The invocation name

The document refers to the tool as `a2a-cli` for ease of reference (see the Naming note in the introduction). Whether the published binary is invoked as `a2a-cli`, `a2a`, or something else is a project decision that does not affect a single normative requirement.

Position: `a2a-cli` throughout, with the note making clear it is a reference name, not a requirement.

## OQ9 — Naming the target agent

Today the agent is named by a flag: `--agent-card <ref>`, accepting a bare host, a full card URL, or a local file path (§5.2, §8.1). The alternative is a positional argument on `card get` (`a2a-cli card get example.com`), which reads better for the one command whose whole job is the card, at the cost of two ways to say the same thing.

Position: the flag model stands. Adding a positional form later is backward-compatible; removing one is not.

## OQ10 — The media-type syntax

§8.2 requires that a caller be able to set a part's media type explicitly, and defines `--media-type <type>` bound to the part flag immediately preceding it. Positional binding is compact and preserves part order, but it is order-sensitive in a way most CLI flags are not; an attribute suffix (`--file report.bin;type=application/pdf`) avoids that at the cost of a small parser.

Position: positional binding, because the part flags are already order-sensitive (§8.2) and a suffix invents quoting rules of its own.

## OQ11 — Required flags versus optional aliases

§5.2 names one canonical spelling per behavior (`--wait`, `--async`) and marks the rest as OPTIONAL aliases (`--watch`, `--return-immediately`, `--no-wait`). Requiring every spelling maximises the chance that a command a user types works; requiring one maximises the chance that a *script* written against the specification runs everywhere.

Position: canonical required, aliases optional. The specification exists to make scripts portable, and a portable script uses the canonical name.

## Raising a new one

Open questions enter the same way changes do: a feature request or an RFC through the project's governance process (`SPEC.md` §15.4). An accepted answer is applied to `SPEC.md`, recorded in Appendix D, and the entry here is removed — leaving its number spent.
