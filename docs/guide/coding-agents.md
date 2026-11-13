# Coding Agents

A coding agent that can run shell commands can use `a2a` to hand work to A2A agents. The `a2a-cli` [Agent Skill](https://agentskills.io/) teaches it how: fetch a card, send a task, stream or poll the result, and keep a log of the tasks it started.

Install `a2a` first; the skill drives the binary and doesn't install it.

## Install the skill

With the [`skills`](https://www.skills.sh) CLI:

```bash
npx skills add https://github.com/a2aproject/a2a-cli --skill a2a-cli
```

As a plugin in Claude Code, from the [a2aproject/agent-plugins](https://github.com/a2aproject/agent-plugins) marketplace:

```bash
/plugin marketplace add a2aproject/agent-plugins
/plugin install a2a-cli@a2a-plugins
```

By hand, into the directory your agent loads skills from (many read `~/.agents/skills/`):

```bash
mkdir -p ~/.agents/skills/a2a-cli
a2a skill > ~/.agents/skills/a2a-cli/SKILL.md
```

`a2a skill` prints the skill bundled into the binary, so it always matches the installed version. The source is [`skills/a2a-cli/SKILL.md`](../../skills/a2a-cli/SKILL.md).

## Then ask

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
