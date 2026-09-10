# a2a-cli — Agent Plugin

An [Agent Plugin](https://agent-plugins.org/) that teaches a plugin-aware AI agent how to drive the [`a2a` CLI](https://github.com/a2aproject/a2a-cli) — discovering, messaging, and managing A2A (Agent2Agent) agents from the command line.

It packages the single Agent Skill the CLI ships (per [SPEC.md §14](../specification/SPEC.md)) as one installable unit. The skill and the binary can always be installed independently — the plugin is a convenience, never a precondition.

## Contents

```text
agent-plugin/
├── plugin.json                 # manifest (Agent Plugins 1.0.0)
├── LICENSE                     # Apache-2.0
└── skills/
    └── a2a-cli/
        └── SKILL.md            # agent-facing usage guidance
```

Skills-only: this plugin carries no MCP server, which is valid under Agent Plugins (a skills-only plugin conforms).

## Requires the `a2a` binary

Agent Plugins packages the agent-facing pieces (skills and MCP servers), not tool binaries. The `a2a` binary is **not** bundled and must be installed separately (Homebrew, WinGet, a prebuilt release, or `go install`) — see the [project README](https://github.com/a2aproject/a2a-cli#installation) and the skill's own Setup section. The skill includes a preflight check (`a2a version`).

## Install

With the [`skills`](https://github.com/vercel-labs/skills) CLI, from this repository:

```bash
# from a local checkout
npx skills add ./agent-plugin/skills/a2a-cli -g

# or directly from GitHub
npx skills add https://github.com/a2aproject/a2a-cli/tree/main/agent-plugin/skills/a2a-cli -g
```

A plugin-aware client can instead load this directory as an Agent Plugin: it reads `plugin.json`, discovers skills under `skills/`, and validates each `SKILL.md`. Installation, enablement, and updates are client-controlled.

## Versioning

The plugin `version` in `plugin.json` (`0.2.0`) tracks the `a2a` CLI release; the bundled skill keeps its own date-based `metadata.version`. The manifest targets Agent Plugins spec **1.0.0**.
