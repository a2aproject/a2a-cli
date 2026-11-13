<div class="hero" markdown>

<div class="hero-text" markdown>

# Talk to any A2A agent from your terminal

`a2a` discovers [A2A](https://a2a-protocol.org) agents, sends them messages and files, streams their replies and manages their tasks over JSON-RPC, REST, gRPC or a transport you plug in. People, scripts and coding agents use the same commands.

[Get started](getting-started.md){ .md-button .md-button--primary }
[Read the lessons](../lessons/README.md){ .md-button }

```bash title="install"
brew tap a2aproject/a2a-cli https://github.com/a2aproject/a2a-cli
brew install a2a
```

</div>

<div class="hero-demo" markdown>

![a2a discovering an agent, sending it a message and streaming the reply](assets/demo.gif)

</div>

</div>

## Find your way

<div class="grid cards" markdown>

-   **User Guide**

    Short pages on each task: agent cards, messages, streaming, tasks, configuration and credentials.

    [→ Read the guide](guide/agent-cards.md)

-   **Lessons**

    A hands-on course, from your first message to conversations and credentials, with a recording of each lesson.

    [→ Start lesson 1](../lessons/01-first-message/README.md)

-   **Examples**

    Recorded recipes to copy: scripting with `jq`, a credential helper, a command plugin, a transport plugin.

    [→ Browse examples](../examples/README.md)

-   **Advanced**

    How discovery, transport choice, streaming timeouts, credential helpers and plugins work inside.

    [→ Look inside](advanced/architecture.md)

-   **Reference**

    Every command and flag, configuration keys, output formats and exit codes.

    [→ Look it up](reference/commands/a2a.md)

-   **For coding agents**

    Give Claude Code, Cursor or Codex the `a2a` skill so they can hand work to A2A agents.

    [→ Install the skill](guide/coding-agents.md)

</div>

## Three commands

```bash title="~"
a2a card get https://agent.example.com                   # what can it do?
a2a send -a https://agent.example.com "Summarize this"   # ask and wait
a2a send -a https://agent.example.com --stream "..."     # watch it work
```

No agent yet? [a2a-serve](https://github.com/a2aproject/a2a-serve) serves Claude Code, opencode or any ACP coding agent as an A2A agent. [Getting Started](getting-started.md) sets one up.
