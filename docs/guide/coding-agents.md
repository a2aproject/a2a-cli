# Coding Agents

A coding agent that can run shell commands can use `a2a` to hand work to A2A agents. The `a2a-cli` plugin teaches it how: fetch a card, send a task, stream or poll the result, and keep a log of the tasks it started. The plugin is published in the [a2aproject/agent-plugins](https://github.com/a2aproject/agent-plugins) marketplace and carries the `a2a-cli` [Agent Skill](https://agentskills.io/).

Install `a2a` first; the plugin drives the binary and doesn't install it.

## Install the plugin

Claude Code:

```bash
/plugin marketplace add a2aproject/agent-plugins
/plugin install a2a-cli@a2a-plugins
```

GitHub Copilot CLI:

```bash
copilot plugin marketplace add a2aproject/agent-plugins
copilot plugin install a2a-cli@a2a-plugins
```

Codex: add the marketplace, then run `codex`, open `/plugins`, pick the **A2A Agent Plugins** tab and install `a2a-cli`:

```bash
codex plugin marketplace add a2aproject/agent-plugins
```

Gemini CLI installs the skill the plugin contains:

```bash
gemini skills install https://github.com/a2aproject/agent-plugins.git --path plugins/a2a-cli/skills/a2a-cli
```

OpenCode, Cursor, Antigravity and most other agents load skills from a directory. The [`skills`](https://www.skills.sh) CLI installs it there:

```bash
npx skills add https://github.com/a2aproject/agent-plugins/tree/main/plugins/a2a-cli/skills/a2a-cli -g
```

The [agent-plugins README](https://github.com/a2aproject/agent-plugins#installation) also covers OpenClaw, Hermes Agent and how to keep the plugin updated.

## Install the skill by hand

`a2a skill` prints the skill bundled into the binary, so it always matches the installed version. Write it into the directory your agent loads skills from; many read `~/.agents/skills/`:

```bash
mkdir -p ~/.agents/skills/a2a-cli
a2a skill > ~/.agents/skills/a2a-cli/SKILL.md
```

The source is [`skills/a2a-cli/SKILL.md`](../../skills/a2a-cli/SKILL.md).

## Use it

```text
Ask the agent at https://review.example.com to review the diff on this branch,
and wait for its answer.
```

## Contacts and agent discovery

In Claude Code the plugin also adds hooks that run `a2a hook` on `SessionStart`, `UserPromptSubmit` and `PostToolBatch`. For opencode, the plugin carries an opencode plugin, `ai.opencode/plugins/a2a-hook.js`, that runs `a2a hook` from opencode's events; copy it into `~/.config/opencode/plugins/`. The [OpenCode calls a contact](../../examples/opencode-contacts/README.md) and [agent discovery](../../examples/agent-discovery/README.md) examples show both in action. The hooks tell the agent which A2A agents it can delegate to:

- **Contacts.** Your [contacts](contacts.md) are listed as trusted at the start of a session, after a compaction, and again whenever the contact book changes. After a compaction the agents discovered earlier in the session are listed again too.
- **Discovered agents.** When a URL appears in a prompt or in a `WebFetch` or `WebSearch` call, `a2a` looks for an agent card at that origin's `/.well-known/agent-card.json`. Agents it finds are listed as unverified, and the agent is told to ask you before delegating to them. Save one as a contact to trust it.

Agents in registered [AI Catalogs](ai-catalogs.md) are not listed, since a catalog can list hundreds; the agent finds them with `a2a catalog search`.

The hook makes no network requests, so it doesn't slow down a turn. Origins without a cached card are probed by a detached `a2a hook probe` process, and agents it finds are reported at a later event. Each agent is reported once per session. Found cards are cached for 24 hours, and origins without a card for 6 hours. A connection failure, timeout, server error or rate limit isn't cached, so the origin is probed again when it's next mentioned. A document without an agent name and interface, or larger than 512 KiB, doesn't count as a card.

The hook never fails the harness event: an unreadable configuration file, contact book or event makes it print nothing, and `-v` shows why.

| Setting | Default | Description |
|---|---|---|
| `discovery` | `true` | Probe origins of URLs in the conversation. Set `false` (or `A2ACLI_DISCOVERY=false`) to keep only the contacts. |
| `discovery-allow-host` | | Hosts discovery may probe whatever address they resolve to, and reach by any redirect. Not read from a discovered `.env`. |
| `discovery-scan-tools` | `WebFetch,WebSearch` | Tools whose calls are scanned for URLs. |

```yaml
# ~/.config/a2a-cli/config.yaml
discovery-allow-host: [agents.internal.example]
```

[Security](../advanced/security.md#agent-discovery) lists where a probe may connect.

## When the skill is out of date

Every usage error ends with a hint for agents:

```text
hint: if you are an agent, your a2a skill may be out of date — run `a2a skill` for the current commands and update your local copy
```

An agent that follows it reads the current skill from the binary and corrects its command.

## Agents behind a2a-serve

Agents served by [a2a-serve](https://github.com/a2aproject/a2a-serve) already have `a2a` in their sandbox and call other agents with it; a2a-serve sets the socket and credential helper they need.
