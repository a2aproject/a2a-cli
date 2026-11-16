<div class="hero" markdown>

<div class="hero-text" markdown>

# Let your coding agent work with A2A agents

The `a2a-cli` plugin teaches Claude Code, Codex, Gemini CLI and other coding agents to hand tasks to [A2A](https://a2a-protocol.org) agents, follow them and collect the results. It drives `a2a`, a command-line client you can also use yourself.

[Install the plugin](#install-the-plugin){ .md-button .md-button--primary }
[Get started with the CLI](getting-started.md){ .md-button }

```bash title="install a2a"
brew tap a2aproject/a2a-cli https://github.com/a2aproject/a2a-cli
brew install a2a
```

</div>

<div class="hero-demo" markdown>

![a2a discovering an agent, sending it a message and streaming the reply](assets/demo.gif)

</div>

</div>

## Install the plugin

The plugin carries the [a2a-cli skill](../skills/a2a-cli/SKILL.md) and is published in the [a2aproject/agent-plugins](https://github.com/a2aproject/agent-plugins) marketplace. It runs the `a2a` binary, so install `a2a` first. [Getting Started](getting-started.md#install) has the other ways to install it.

=== "Claude Code"

    ```bash
    /plugin marketplace add a2aproject/agent-plugins
    /plugin install a2a-cli@a2a-plugins
    ```

=== "Copilot CLI"

    ```bash
    copilot plugin marketplace add a2aproject/agent-plugins
    copilot plugin install a2a-cli@a2a-plugins
    ```

=== "Codex"

    ```bash
    codex plugin marketplace add a2aproject/agent-plugins
    ```

    Then run `codex`, open `/plugins`, pick the **A2A Agent Plugins** tab and install `a2a-cli`.

=== "Gemini CLI"

    ```bash
    gemini skills install https://github.com/a2aproject/agent-plugins.git --path plugins/a2a-cli/skills/a2a-cli
    ```

=== "OpenCode, Cursor and others"

    ```bash
    npx skills add https://github.com/a2aproject/agent-plugins/tree/main/plugins/a2a-cli/skills/a2a-cli -g
    ```

The [agent-plugins README](https://github.com/a2aproject/agent-plugins#installation) covers OpenClaw, Hermes Agent and updates. Then ask your agent:

```text
Ask the agent at https://review.example.com to review the diff on this branch,
and tell me what it says.
```

The agent reads the other agent's card, sends the task, follows it until it finishes, and keeps a log of the tasks it started so you can come back to them. [Coding agents](guide/coding-agents.md) has more.

## Use the CLI yourself

```bash title="~"
a2a card get https://agent.example.com                   # what can it do?
a2a send -a https://agent.example.com "Summarize this"   # ask and wait
a2a send -a https://agent.example.com --stream "..."     # watch it work
```

`a2a` speaks JSON-RPC, REST and gRPC, picks the transport from the agent card, and prints JSON for scripts. No agent to try it on? [a2a-serve](https://github.com/a2aproject/a2a-serve) serves Claude Code, opencode or any ACP coding agent as an A2A agent, and [Getting Started](getting-started.md) sets one up.

## Learn more

<div class="grid cards" markdown>

-   **User Guide**

    Short pages on each task: agent cards, messages, streaming, tasks, configuration and credentials.

    [→ Read the guide](guide/agent-cards.md)

-   **Lessons**

    A course from your first message to conversations and credentials, with a recording of each lesson.

    [→ Start lesson 1](../lessons/01-first-message/README.md)

-   **Examples**

    Recorded recipes: scripting with `jq`, a credential helper, a command plugin, a transport plugin.

    [→ Browse examples](../examples/README.md)

-   **Advanced**

    How discovery, transport choice, streaming timeouts, credential helpers and plugins work.

    [→ Read more](advanced/architecture.md)

-   **Reference**

    Every command and flag, configuration keys, output formats and exit codes.

    [→ Look it up](reference/commands/a2a.md)

-   **AI Catalogs**

    Find agents in an AI Catalog and talk to them by identifier. Experimental.

    [→ Read the guide](guide/ai-catalogs.md)

</div>
