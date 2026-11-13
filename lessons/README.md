# Lessons

A short course on the `a2a` CLI. Each lesson explains what happens at every step and why, and ends where the next one starts. The recording at the top of each lesson plays it from start to end.

| # | Lesson | You learn |
|---|---|---|
| 1 | [Your first message](01-first-message/README.md) | Start an agent, read its card, send a message, stream a reply, save the agent in `.env`. |
| 2 | [Configuration](02-configuration/README.md) | Flags, `A2ACLI_*` variables, `.env` and `config.yaml`, and how `config show` tells you which won. |
| 3 | [Messages and tasks](03-messages-and-tasks/README.md) | Multi-part messages, async sends, waiting for, following, listing and canceling tasks. |
| 4 | [Conversations](04-conversations/README.md) | Contexts, answering an agent's question, and `a2a chat`. |
| 5 | [Credentials](05-credentials/README.md) | How `a2a` proves who you are: tokens, credential helpers and `auth get`. |

## What you need

- The `a2a` CLI. See [Getting Started](../docs/getting-started.md#install).
- [a2a-serve](https://github.com/a2aproject/a2a-serve), which serves the agent you talk to in every lesson:

  ```bash
  brew install a2aproject/a2a-cli/a2a-serve && a2a-serve doctor --fix
  ```

- Docker, for the agent's sandbox. Without Docker, install the agent on your machine and add `--sandbox local` where the lessons start it.
- A model API key. The lessons use opencode with an [OpenRouter](https://openrouter.ai) key in `OPENROUTER_API_KEY`. With Claude Code, start the agent with `a2a-serve start claude --secret ANTHROPIC_API_KEY` instead.

Lesson 1 starts the agent in `~/a2a-lessons/agent`, and the later lessons reuse it. When you're done, stop it with `a2a-serve stop --dir ~/a2a-lessons/agent`.

The agent's replies in the recordings come from a scripted test agent, so they are shorter than a model's.
