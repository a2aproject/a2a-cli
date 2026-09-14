# A2A CLI

<div align="center" width="800">
   <img src="https://raw.githubusercontent.com/a2aproject/A2A/refs/heads/main/docs/assets/a2a_logo/color/SVG/a2a_color.svg" width="600" alt="Agent2Agent Protocol Logo"/>
   <h3>
       Discover, message, and manage A2A agents from your terminal.
   </h3>
</div>

<p align="center">
  <a href="https://github.com/a2aproject/a2a-cli/releases/latest"><img src="https://img.shields.io/github/v/release/a2aproject/a2a-cli?sort=semver" alt="Latest release"></a>
  <a href="./LICENSE"><img src="https://img.shields.io/github/license/a2aproject/a2a-cli" alt="License: Apache-2.0"></a>
</p>

The **A2A CLI** (`a2a`) is the official command-line client for [A2A (Agent2Agent) agents](https://a2a-protocol.org/latest/), maintained by the **A2A Project Team**.

<div align="center">
  <img src="./docs/demo/a2a-demo.gif" alt="a2a CLI demo: build a server from a script, then discover and message it" width="720">
</div>


## Why the A2A CLI

The `a2a` CLI is one consistent way to work with A2A agents — no throwaway scripts or raw-JSON parsing just to talk to an agent.

* **AI coding agents** delegate work to A2A agents through one command surface, driven from a bundled skill descriptor — no custom plugins per harness.
* **Developers** inspect and drive any deployed agent from the terminal — fetch a card, send a message, stream updates, list or cancel tasks, one readable command each.
* **Automation and CI** call agents from a stable, scriptable surface: protocol-native JSON (`-o json`) and predictable exit codes, no client library required.

The CLI handles the underlying complexity. It negotiates the transport (JSON-RPC, REST, or gRPC) from the agent's card, waits for a task to finish unless you tell it not to, and behaves the same across agents and languages.


## Installation

**Homebrew (macOS / Linux)**

```bash
brew tap a2aproject/a2a-cli https://github.com/a2aproject/a2a-cli
brew install a2a
```

**WinGet (Windows)**

```powershell
winget install a2aproject.a2acli
```

**Prebuilt binaries** — download an archive from the [latest release](https://github.com/a2aproject/a2a-cli/releases/latest), extract it, and put the `a2a` binary on your `PATH`.

**From source**

```bash
go install github.com/a2aproject/a2a-cli@latest
# go install names the binary `a2a-cli`; rename it to `a2a`
mv "$(command -v a2a-cli)" "$(dirname "$(command -v a2a-cli)")/a2a"
```

## Usage

```bash
# Discover an agent's card
a2a card get https://agent.example.com

# Send a message and wait for the result
a2a send -a https://agent.example.com "Hello, what can you do?"

# Stream events as they arrive
a2a send -a https://agent.example.com --stream "Summarize this document"
```

See the **[full command reference](./internal/README.md)** for all commands, flags, configuration, and server mode.

## Cookbook

Learn the CLI by example. The **[cookbook](./examples/)** is a short, hands-on course — each lesson is a small folder you can run on its own, building from your first A2A server up to streaming, configuration, and the full message and task lifecycle:

1. **[Build a quick A2A server](./examples/01-exec-demo/)** — turn an ordinary script into an A2A server with `--exec`, then send it a message.
2. **[Discover and talk to an agent](./examples/02-card-and-send/)** — read an agent's card, save it, and send a text message.
3. **[Configure the CLI](./examples/03-config/)** — flags, environment variables, and `.env` files, and how to tell which one won.
4. **[Messages and tasks](./examples/04-messages-and-tasks/)** — build multi-part messages, stream replies, and follow a task through its lifecycle.

## Agent Skill

Give your AI coding agent the ability to drive `a2a` directly. The **[a2a-cli Agent Skill](https://www.skills.sh/a2aproject/a2a-cli/a2a-cli)** teaches harnesses like Claude Code, Cursor, and Codex to delegate work to A2A agents from the command line.

Install it into your agent with [`skills`](https://www.skills.sh):

```bash
npx skills add https://github.com/a2aproject/a2a-cli --skill a2a-cli
```

The skill drives the `a2a` binary, so [install the CLI](#installation) first. The descriptor lives at [`skills/a2a-cli/SKILL.md`](./skills/a2a-cli/SKILL.md).

### Agent Plugin

Prefer a plugin-aware client? The **[Agent Plugin](./agent-plugin/)** bundles the same skill as one installable unit ([Agent Plugins](https://agent-plugins.org/) 1.0.0, skills-only). Install it with [`skills`](https://www.skills.sh):

```bash
npx skills add https://github.com/a2aproject/a2a-cli/tree/main/agent-plugin/skills/a2a-cli -g
```

Like the skill, it drives the `a2a` binary — [install the CLI](#installation) first.

## About the Project

Several community and SDK-provided A2A CLIs existed across languages, but they diverged in command names, flags, output shapes, and transport handling. This project reconciles them into one **standardized** CLI — built for long-term stability and cross-transport consistency. It builds on the CLI from the [A2A Go SDK](https://github.com/a2aproject/a2a-go).

The **[Specification (`SPEC.md`)](./specification/SPEC.md)** captured that reconciliation — command taxonomy, output contracts, polling/streaming rules, and exit codes — and served as the roadmap.


## Custom Transport Plugins

The CLI speaks JSON-RPC, REST and gRPC out of the box. Additional transport
bindings can be added **without recompiling** by dropping an
`a2a-transport-<name>` binary on your `PATH`. The CLI launches the plugin as a
local proxy that speaks a standard A2A binding and forwards to the custom
protocol, so `--transport <name>` works uniformly for built-ins and plugins:

```console
$ a2a transport list
$ a2a send --transport slimrpc --endpoint slim://agents.example/agent "hello"
```

Authoring a plugin in Go is a few lines with the
[`devkit/clitransport`](./devkit/clitransport) package — you provide an
`a2aclient.Transport`, it produces a CLI-compatible plugin. See the
**[transport plugin guide](./docs/transport-plugins.md)** and the runnable
**[echo plugin example](./examples/a2a-transport-echo)**.


## How to Contribute & Provide Feedback

We welcome review and input from engineers and the community:

1. Read the **[Specification (`SPEC.md`)](./specification/SPEC.md)** for background.
2. Open an issue or pull request to share suggestions, questions, or edge cases.

You can also help by sharing how you use the tool. See the **[Contributing guide](CONTRIBUTING.md)**; conformance details live in the [Compliance Checklist (`COMPLIANCE.md`)](./specification/COMPLIANCE.md).


## License

The A2A CLI and Specification are open-source and licensed under the [**Apache License 2.0**](LICENSE).
