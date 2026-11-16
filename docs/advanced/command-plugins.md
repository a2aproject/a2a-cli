# Command Plugins

A command plugin adds a top-level command to `a2a` without rebuilding it. It is a program named `a2a-<name>` on `PATH`, and `a2a <name> [args...]` runs it. Discovery is off by default. This page explains how discovery works, how a plugin runs, and the rules for name conflicts. The [Plugins guide](../guide/plugins.md) covers installing and enabling plugins, and the [Command Plugin Protocol](../reference/command-plugin-protocol.md) reference covers writing one.

## Enabling discovery

The CLI looks for command plugins only when `plugins-enabled` is true:

```bash
a2a plugin set-enabled true     # writes plugins-enabled: true to ~/.config/a2a-cli/config.yaml
a2a plugin set-enabled false
```

The setting can also come from `A2ACLI_PLUGINS_ENABLED`, a `.env` found by walking up from the working directory, or the global `.env`. A file passed with `--config` is not read for it. `newRootCmd` reads the setting before cobra parses the command line, because it decides which commands exist; see [Configuration Layers](configuration.md#plugins-enabled).

`a2a plugin list` scans `PATH` whether discovery is enabled or not, and prints the state:

```text
Command plugins: enabled
NAME   VERSION  DESCRIPTION                                  PATH
hello  1.0.0    Greets the world — reference command plugin  /home/me/bin/a2a-hello
```

## Discovery

`commandplugin.Discover` (`internal/commandplugin/discover.go`) reads each directory on `PATH` in order:

- It takes entries whose name starts with `a2a-` and has something after it.
- It skips names that start with `a2a-transport-`. Those are [transport plugins](transport-plugins.md).
- It keeps only executables. On Unix that is a regular file with any execute bit set. On Windows it is a file ending in `.exe`, `.bat` or `.cmd`; the extension stays part of the command name, so `a2a-hello.exe` is the command `hello.exe`.
- The first directory on `PATH` that has a name wins, as in a shell.

Unreadable directories are skipped. Discovery runs no plugin.

## Conflicts

Built-in commands always win. Before it adds plugins, the CLI creates cobra's `help` and `completion` commands, so the reserved names are every built-in command plus those two. A plugin with a reserved name, such as `a2a-send` or `a2a-help`, is skipped without a message.

## Running a plugin

A plugin command disables flag parsing, so every argument after the command name goes to the plugin unchanged, including `--help`, `-o` and `-v`. Global flags before the name are parsed by `a2a` and then ignored; the plugin does not receive them. The configuration hook that runs before built-in commands still runs, so an invalid `A2ACLI_*` value or configuration file fails `a2a <name>` before the plugin starts.

| Platform | How the plugin runs |
| --- | --- |
| Unix | `syscall.Exec` replaces the `a2a` process with the plugin. Same PID; stdin, stdout and stderr are the terminal's; the exit code is the plugin's. |
| Windows | The plugin runs as a child process with the same stdin, stdout and stderr. `a2a` exits with the child's exit code. |

The plugin receives the CLI's process environment. Values from `.env` and configuration files are not added; a plugin that wants them reads the files itself.

## Descriptions

A plugin may implement an `info` subcommand that prints a JSON object with `name`, `version` and `description`. The CLI uses it in two places:

- **Root help.** The first time `a2a --help` (or `a2a` with no arguments) renders, the CLI runs `info` on every plugin at once, with a shared 3-second deadline, and fills in the description column. Plugins that fail or time out are listed without a description. No other command runs `info`, so plugins add no startup time.
- **`plugin list`.** Runs `info` on each plugin in turn, each with its own 3-second limit, and prints the error in place of the description when it fails.

## a2a-serve

[a2a-serve](https://github.com/a2aproject/a2a-serve) is a command plugin. Installed on `PATH` as `a2a-serve` with discovery enabled, it runs as `a2a serve`; `a2a-serve doctor --fix` sets `plugins-enabled: true` for you. The two invocations are the same program.

```bash
a2a serve status
a2a-serve status
```

## Trust

A command plugin is a program you installed, and it runs with your permissions and your environment. The CLI does not sandbox it. Discovery is opt-in so that an `a2a-*` file on `PATH` does not become a command until you enable plugins. See [Security](security.md).
