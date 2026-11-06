# a2a-cli

An [Agent Plugin](https://agent-plugins.org/) that teaches an AI agent to drive the [`a2a` CLI](https://github.com/a2aproject/a2a-cli): fetch an agent's card, send it a message, stream or poll a task, and list, resume, or cancel tasks on any A2A (Agent2Agent) agent.

> [!NOTE]
> This plugin is generated from [a2aproject/a2a-cli](https://github.com/a2aproject/a2a-cli/tree/main/agent-plugin) on every a2a-cli release. Send changes there.

## Requires the `a2a` binary

The plugin carries the agent-facing skill and hooks only. Install `a2a` separately:

- **Homebrew (macOS / Linux):** `brew tap a2aproject/a2a-cli https://github.com/a2aproject/a2a-cli && brew install a2a`
- **WinGet (Windows):** `winget install a2aproject.a2acli`
- **Prebuilt binary:** download from the [latest release](https://github.com/a2aproject/a2a-cli/releases/latest) and put `a2a` on your `PATH`.

Check it with `a2a version`. The skill runs the same preflight check.

## Install

**Claude Code**

```bash
/plugin marketplace add a2aproject/agent-plugins
/plugin install a2a-cli@a2a-plugins
```

**Any Agent Plugins client:** load the `plugins/a2a-cli` directory of [a2aproject/agent-plugins](https://github.com/a2aproject/agent-plugins).

**Skill only**, with the [`skills`](https://www.skills.sh) CLI:

```bash
npx skills add https://github.com/a2aproject/agent-plugins/tree/main/plugins/a2a-cli/skills/a2a-cli -g
```

## Contents

```text
a2a-cli/
├── plugin.json          # Agent Plugins 1.0.0 manifest
├── LICENSE
├── hooks/
│   └── hooks.json       # Claude Code hooks running `a2a hook`
└── skills/
    └── a2a-cli/
        └── SKILL.md
```

## Usage

Ask your agent for A2A work and it loads the skill on its own, for example:

> Fetch the agent card at `http://localhost:8080` and send it "summarize this repo".

## Contacts and agent discovery (Claude Code)

The hooks run `a2a hook`, which tells your agent which A2A agents it can delegate to:

- **Contacts.** Agents you saved with `a2a contact add <name> <card-ref>` are listed as trusted at the start of a session, after a compaction, and again whenever the contact book changes. The agent can delegate to them with `a2a send -a <name>`.
- **Discovered agents.** When a URL appears in a prompt or in a `WebFetch` or `WebSearch` call, `a2a` looks for an agent card at that site's `/.well-known/agent-card.json`. Agents it finds are listed as unverified, and the agent is told to ask you before delegating to them.

The hooks don't make network requests, so they don't slow down a turn. A detached background process fetches cards for sites it hasn't seen, and the agent hears about them at its next step. Cards are cached under the user cache directory (`~/.cache/a2a-cli` on Linux), and each agent is listed once per session.

Discovery probes public and internal hosts. It never connects to link-local addresses such as cloud metadata endpoints, and it doesn't follow a redirect from a public site to an internal address. To make an exception for a host, list it in `~/.config/a2a-cli/config.yaml`:

```yaml
discovery-allow-host: [agents.internal.example]
```

To turn discovery off and keep the contacts, set `discovery: false` in the same file, or `A2ACLI_DISCOVERY=false` in the environment. To scan other tools' calls for URLs, set `discovery-scan-tools`.

## Versioning

The plugin `version` matches the a2a-cli release it was built from. The bundled skill keeps its own date-based `metadata.version`.
