# Plugins

Two kinds of plugins extend `a2a` without rebuilding it. Both are programs on your `PATH`.

| Kind | Binary | Adds |
|---|---|---|
| Command plugin | `a2a-<name>` | A command, `a2a <name>`. |
| Transport plugin | `a2a-transport-<name>` | A transport, `--transport <name>`. |

Plugins run as you, with your environment. Install only plugins you trust.

## Command plugins

Command plugins are off by default, so an `a2a-*` program that happens to be on your `PATH` doesn't become an `a2a` command until you turn them on:

```bash
a2a plugin set-enabled true
a2a plugin list
```

```text
Command plugins: enabled
NAME   VERSION  DESCRIPTION                                                   PATH
hello  1.0.0    Greets the world — reference command plugin                   /home/me/bin/a2a-hello
serve  v0.6.0   Run a coding agent as an A2A server, or a local test server  /opt/homebrew/bin/a2a-serve
```

Plugins appear in `a2a --help`, and run with every argument passed through:

```bash
a2a hello Alice
a2a serve status
```

The setting is stored as `plugins-enabled` in `~/.config/a2a-cli/config.yaml`. `A2ACLI_PLUGINS_ENABLED=true` turns them on for one shell, and a project's `.env` can set it too. Built-in commands always win over a plugin with the same name.

## Transport plugins

Transport plugins need no switch. `a2a` starts one when `--transport` names it, or when an agent card lists a binding an installed plugin provides:

```bash
a2a transport list
a2a send --transport echo --endpoint echo://demo "hello"
```

```text
NAME  VERSION  PROTOCOL  DESCRIPTION                                           PATH
echo  1.0.0    1.0       Echoes the caller's message back as a completed task  /home/me/bin/a2a-transport-echo
```

## Writing plugins

- [A command plugin](../../examples/a2a-hello/README.md) and [a transport plugin](../../examples/a2a-transport-echo/README.md) are runnable examples in Go.
- [Command plugins](../advanced/command-plugins.md) and [Transport plugins](../advanced/transport-plugins.md) explain how they run.
- The [command plugin protocol](../reference/command-plugin-protocol.md) and [transport plugin protocol](../reference/transport-plugin-protocol.md) specify the contracts.
