# a2a-cli — Agent Plugin

An [Agent Plugin](https://agent-plugins.org/) that teaches an AI agent to drive
the [`a2a` CLI](https://github.com/a2aproject/a2a-cli) — discovering, messaging,
and managing A2A (Agent2Agent) agents from the command line.

This one directory is two things at once:

- a **portable Agent Plugin** (`plugin.json`, `skills/`) that any Agent
  Plugins–aware client can load, and
- a **Claude Code plugin** (`.claude-plugin/plugin.json`, `hooks/hooks.json`,
  `skills/`) installable from a marketplace.

The skill and the binary can always be installed independently — the plugin is a
convenience, never a precondition.

## Contents

```text
agent-plugin/
├── plugin.json                 # portable manifest (Agent Plugins 1.0.0)
├── .claude-plugin/
│   └── plugin.json             # Claude Code plugin manifest
├── hooks/
│   └── hooks.json              # Claude Code progressive-discovery hooks
├── LICENSE                     # Apache-2.0
└── skills/
    └── a2a-cli/
        └── SKILL.md            # agent-facing usage guidance
```

## What it adds

- **A skill** that teaches a harness how to drive `a2a` (fetch a card, send,
  stream/poll, resume, cancel).
- **Discovery hooks** (Claude Code): on every prompt and after web/file/shell
  tool calls, `a2a discover` probes any origin in context for a
  `/.well-known/agent-card.json` and injects the agent's skills — so the model
  learns *when* an A2A agent is worth using, not just *how*. See the runnable
  [progressive-discovery example](../examples/05-progressive-discovery/).

## Requires the `a2a` binary

Agent Plugins packages the agent-facing pieces (skills and hooks), not tool
binaries. The `a2a` binary is **not** bundled and must be installed separately
(Homebrew, WinGet, a prebuilt release, or `go install`) — see the
[project README](https://github.com/a2aproject/a2a-cli#installation). The skill
includes a preflight check (`a2a version`), and the hooks stay silent if `a2a`
is missing.

## Install in Claude Code

From a marketplace (the repo root ships a `.claude-plugin/marketplace.json`):

```text
/plugin marketplace add a2aproject/a2a-cli
/plugin install a2a-cli@a2a
```

To test from a local checkout, point the marketplace at the repo directory:

```text
/plugin marketplace add /path/to/a2a-cli
/plugin install a2a-cli@a2a
```

Validate the manifests (and optionally build a distributable zip) before
publishing:

```bash
./scripts/package-claude-plugin.sh          # validate
./scripts/package-claude-plugin.sh --zip    # validate + build dist/*.zip
```

## Install as a skill only

With the [`skills`](https://github.com/vercel-labs/skills) CLI, if you want the
skill without the hooks:

```bash
npx skills add https://github.com/a2aproject/a2a-cli/tree/main/agent-plugin/skills/a2a-cli -g
```

## Versioning

Both manifests carry the same `version` (`0.3.0`), tracking the plugin's own
releases; the bundled skill keeps its own date-based `metadata.version`. The
portable manifest targets Agent Plugins spec **1.0.0**.
