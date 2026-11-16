# Command Plugin Protocol

A command plugin is an executable named `a2a-<name>` on `PATH` that `a2a` runs as `a2a <name>`. The contract has three parts: the name, the invocation, and an optional `info` subcommand. This page is the full contract, with examples in shell and Go. [Command Plugins](../advanced/command-plugins.md) explains discovery and conflicts, and the [Plugins guide](../guide/plugins.md) covers installing.

## Naming

| Rule | Detail |
| --- | --- |
| Name | `a2a-<name>`, where `<name>` is not empty. The command is `a2a <name>`. |
| Reserved | `a2a-transport-*` names are transport plugins and are never command plugins. |
| Executable | Unix: a regular file with an execute bit set. Windows: a `.exe`, `.bat` or `.cmd` file; the extension is part of the command name. |
| Duplicates | The first directory on `PATH` wins. |
| Conflicts | A name of a built-in command, including `help` and `completion`, is ignored. |

Discovery runs only when `plugins-enabled` is true. See [Configuration](configuration.md).

## Invocation

`a2a <name> [args...]` runs `a2a-<name> [args...]`:

- Every argument after `<name>` is passed unchanged, including `--help`, `-o json` and `-v`. The plugin parses its own flags.
- Global flags before `<name>` are not passed.
- `argv[0]` is the plugin's path as found on `PATH`.
- The environment is the CLI's process environment. Values from `.env` and configuration files are not added.
- stdin, stdout and stderr are the terminal's.
- The exit code of `a2a` is the plugin's.

On Unix the plugin replaces the `a2a` process and keeps its PID. On Windows it runs as a child process.

Before running the plugin, `a2a` loads its configuration, so an invalid `A2ACLI_*` value fails the command with exit code 2 and the plugin does not start.

## `info`

A plugin may implement `info`. Invoked as `a2a-<name> info`, it prints one JSON object to stdout and exits 0:

```json
{"name":"hello","version":"1.0.0","description":"Greets the world — reference command plugin"}
```

| Field | Type | Required | Use |
| --- | --- | --- | --- |
| `name` | string | expected | The plugin name, the part after `a2a-`. The CLI does not check it. |
| `version` | string | no | The `VERSION` column of `a2a plugin list` |
| `description` | string | no | The description in `a2a --help` and `a2a plugin list` |

| Caller | Timeout | On failure |
| --- | --- | --- |
| `a2a --help` | 3 seconds for all plugins together, run in parallel | Listed without a description |
| `a2a plugin list` | 3 seconds per plugin, run in turn | The error is printed in the description column |

A non-zero exit, output that is not JSON, or a timeout counts as a failure. The command still works. `info` gets the CLI's process environment and no stdin.

A plugin without `info` should not treat `info` as an ordinary argument that does something slow or destructive, since root help runs it.

## Shell example

```bash
#!/bin/sh
# a2a-greet
case "$1" in
  info)
    printf '%s\n' '{"name":"greet","version":"0.1.0","description":"Greets someone"}'
    exit 0
    ;;
esac
echo "Hello, ${1:-World}!"
```

```bash
chmod +x a2a-greet && mv a2a-greet ~/bin/
a2a plugin set-enabled true
a2a greet Alice
```

```text
Hello, Alice!
```

## Go example

The `github.com/a2aproject/a2a-cli/devkit/cliplugin` package provides the `Info` type and `ServeInfo`, which prints the document and exits 0:

```go
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/a2aproject/a2a-cli/devkit/cliplugin"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == cliplugin.SubcommandInfo {
		cliplugin.ServeInfo(cliplugin.Info{
			Name:        "hello",
			Version:     "1.0.0",
			Description: "Greets the world — reference command plugin",
		})
	}

	name := "World"
	if len(os.Args) > 1 {
		name = strings.Join(os.Args[1:], " ")
	}
	if _, err := fmt.Fprintf(os.Stdout, "Hello, %s! (from a2a-hello plugin)\n", name); err != nil {
		fmt.Fprintf(os.Stderr, "a2a-hello: %v\n", err)
		os.Exit(1)
	}
}
```

```bash
go build -o ~/bin/a2a-hello .
a2a hello Alice
```

```text
Hello, Alice! (from a2a-hello plugin)
```

## Calling agents from a plugin

A plugin that talks to A2A agents can run `a2a` itself, for example `a2a send -o json ...`, and parse the JSON. The child `a2a` reads the user's configuration and credential helpers as usual. See [Output](output.md) and [Errors and Exit Codes](errors.md).
