# Getting Started

Install `a2a`, start an agent to talk to, and send it your first message.

## Install

On macOS or Linux with Homebrew:

```bash
brew tap a2aproject/a2a-cli https://github.com/a2aproject/a2a-cli
brew install a2a
```

On Windows:

```powershell
winget install a2aproject.a2acli
```

With the install script, which puts `a2a` in `/usr/local/bin` (`A2A_INSTALL_DIR` changes the directory, `A2A_VERSION` pins a version):

```bash
curl -fsSL https://raw.githubusercontent.com/a2aproject/a2a-cli/main/install.sh | bash
```

With Go 1.26 or later. `go install` names the binary `a2a-cli`, so rename it:

```bash
go install github.com/a2aproject/a2a-cli@latest
mv "$(command -v a2a-cli)" "$(dirname "$(command -v a2a-cli)")/a2a"
```

Prebuilt archives for every platform are on the [releases page](https://github.com/a2aproject/a2a-cli/releases/latest).

Check the install:

```bash
a2a version
```

## Start an agent

If you already have an A2A agent, skip to [Talk to it](#talk-to-it) and use its URL.

Otherwise, [a2a-serve](https://github.com/a2aproject/a2a-serve) serves a coding agent such as Claude Code or opencode as an A2A agent. Install it and start an agent in a project directory:

```bash
brew install a2aproject/a2a-cli/a2a-serve && a2a-serve doctor --fix
cd ~/src/my-project
a2a-serve start claude --secret ANTHROPIC_API_KEY --detach
```

`a2a-serve doctor --fix` also configures `a2a`: it adds a credential helper that signs your requests to the agent, and turns on command plugins so `a2a serve` runs `a2a-serve`. The agent runs in a Docker container; without Docker, install the agent on your machine and add `--sandbox local`. For opencode, run `a2a-serve start opencode --model openrouter/~google/gemini-flash-latest --secret OPENROUTER_API_KEY --detach`.

`a2a-serve status --url=local` prints the agent's URL. Keep it in a variable:

```bash
AGENT="$(a2a-serve status --url=local)"
```

## Talk to it

Read the agent card to see what the agent is and how to reach it:

```bash
a2a card get "$AGENT"
```

```text
Name:         my-project
Description:  my-project is a claude coding agent reached through `a2a-serve`. ...
Version:      1.0.0
Interfaces:
  JSONRPC (A2A v1.0)     http://127.0.0.1:43229
  HTTP+JSON (A2A v1.0)   http://127.0.0.1:43229
Streaming:    true
Skills:
  coding               Coding agent
```

Send it a message. `a2a send` waits until the task finishes and prints it:

```bash
a2a send -a "$AGENT" "What does this project do?"
```

```text
Task:     01a1d530-3d41-7075-bda9-a201e3d486e3
Context:  01a1d530-3d41-7165-85f7-1902789752d6
Status:   completed (2026-11-13T13:54:30Z)
Artifacts:
  [response] A command-line client for A2A agents, written in Go.
```

Watch the agent work with `--stream`:

```bash
a2a send -a "$AGENT" --stream "List the Go packages in this repository."
```

Or hold a conversation:

```bash
a2a chat -a "$AGENT"
```

## Stop typing the URL

Put the agent in a `.env` file. `a2a` reads the nearest `.env` above the working directory:

```bash
echo "A2ACLI_AGENT_CARD=$AGENT" > .env
a2a send "Which tests are slow?"
```

Every flag works the same way. [Configuration](guide/configuration.md) lists the options.

## Next steps

- [Lessons](../lessons/README.md) walk through the CLI one topic at a time.
- [Streaming and async](guide/streaming-and-async.md) covers long tasks.
- [Coding agents](guide/coding-agents.md) shows how to give your own coding agent the `a2a` tool.
