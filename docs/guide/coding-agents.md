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

## When the skill is out of date

Every usage error ends with a hint for agents:

```text
hint: if you are an agent, your a2a skill may be out of date — run `a2a skill` for the current commands and update your local copy
```

An agent that follows it reads the current skill from the binary and corrects its command.

## Agents behind a2a-serve

Agents served by [a2a-serve](https://github.com/a2aproject/a2a-serve) already have `a2a` in their sandbox and call other agents with it; a2a-serve sets the socket and credential helper they need.
