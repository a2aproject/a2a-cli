# a2a-cli agent skill

An [Agent Skill](https://agentskills.io/) that teaches an AI coding agent to
drive the [`a2a` CLI](https://github.com/a2aproject/a2a-cli), the command-line
client for [A2A (Agent2Agent)](https://a2a-protocol.org/) agents.

It is one file, `SKILL.md`. Once it is installed, your agent can:

- fetch an agent's card,
- send it a task,
- stream or poll for the result, and
- log each task it starts, so you can follow up later.

## Prerequisite

The skill drives the `a2a` binary; it does not install it. Install `a2a` first:

```bash
go install github.com/a2aproject/a2a-cli@latest
```

Confirm it is on your `PATH`:

```bash
a2a version
```

## Installation

Fetch the one skill file into the place your agent loads skills from. Many agents
read `~/.agents/skills/`. Use `~/.gemini/config/skills/` for Google Antigravity.

```bash
mkdir -p ~/.agents/skills/a2a-cli
wget -O ~/.agents/skills/a2a-cli/SKILL.md \
  https://raw.githubusercontent.com/a2aproject/a2a-cli/main/skills/a2a-cli/SKILL.md
```

## Verify

Check the file landed:

```bash
ls ~/.agents/skills/a2a-cli   # SKILL.md
```

Then start a new agent session. Confirm the agent lists `a2a-cli` among its
available skills.

## Try it — confirm the skill works

Hand these two prompts to your agent, in order. Together they exercise the whole
path: loading the skill, starting an agent, sending a message, and logging the
task.

**1. Start a local echo agent.**

> Using the a2a-cli skill, start a local echo A2A agent in the background on port 9090.

Your agent should run `a2a server --echo --port 9090 &`. Confirm it is up:

```bash
a2a card get http://localhost:9090
# Name: Echo Agent ... Interfaces: HTTP+JSON  http://127.0.0.1:9090
```

**2. Send it a message and log the task.**

> Send "hello from a2a" to the agent at http://localhost:9090, show me the reply, and log the task.

The echo agent returns your text, and the task lands in the log:

```bash
cat .a2a-cli-logs/tasks.jsonl
# one record: agent, taskId, contextId, state=TASK_STATE_COMPLETED, summary
```

Stop the background agent when done:

```bash
pkill -f "a2a server --echo"
```

## Usage

You do not run the skill yourself. Your agent loads it and follows it when a task
calls for A2A. Just ask, for example:

> Fetch the agent card at `http://localhost:8080` and send it "summarize this repo".

## Uninstall

Remove the directory you created:

```bash
rm -rf ~/.agents/skills/a2a-cli
```

## License

Apache License 2.0, matching the
[a2a-cli project](https://github.com/a2aproject/a2a-cli/blob/main/LICENSE).
