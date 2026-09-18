# Command plugins

The `a2a` CLI can be extended with new top-level commands **without recompiling
it** by installing a *command plugin* binary on your `PATH`.

A command plugin is a standalone executable named `a2a-<name>`. When `a2a`
starts, it scans `PATH` for matching binaries and registers each one as a
dynamic command. Running `a2a <name> [args...]` execs the plugin with all
remaining arguments forwarded verbatim and the full environment inherited.

```
  a2a audit --output json --since 2024-01-01
        │
        │  1. discover a2a-audit on PATH
        │  2. exec: a2a-audit --output json --since 2024-01-01
        ▼
  ┌─────────────────┐
  │ a2a-audit       │  (runs as the process, stdin/stdout/stderr inherited)
  └─────────────────┘
```

## Installing a plugin

Build or download a binary named `a2a-<name>` and place it anywhere on your
`PATH`:

```console
$ go build -o a2a-hello ./examples/a2a-hello
$ mv a2a-hello ~/bin/    # somewhere on PATH
```

List discovered plugins:

```console
$ a2a plugin list
NAME   VERSION  DESCRIPTION                                  PATH
hello  1.0.0    Greets the world — reference command plugin  ~/bin/a2a-hello
```

Run it:

```console
$ a2a hello Alice
Hello, Alice! (from a2a-hello plugin)
```

Plugins also appear in `a2a --help` under **Available Commands**.

> **Trust:** a command plugin is an executable you install yourself. Treat it
> like any other binary on your `PATH` — only install plugins you trust. The
> CLI does not sandbox plugins; it passes arguments and environment through
> unchanged.

## Writing a plugin in Go (recommended)

Use the devkit at
[`github.com/a2aproject/a2a-cli/devkit/cliplugin`](../devkit/cliplugin).
Implement the optional `info` subcommand with `cliplugin.ServeInfo` to supply
a description for `a2a --help` and `a2a plugin list`:

```go
package main

import (
    "fmt"
    "os"

    "github.com/a2aproject/a2a-cli/devkit/cliplugin"
)

func main() {
    if len(os.Args) > 1 && os.Args[1] == cliplugin.SubcommandInfo {
        cliplugin.ServeInfo(cliplugin.Info{
            Name:        "myplugin",
            Version:     "1.0.0",
            Description: "Does something useful",
        })
        return
    }
    // ... plugin logic using os.Args[1:] ...
    fmt.Println("hello from myplugin")
}
```

Build it with the required name and drop it on your `PATH`:

```console
$ go build -o a2a-myplugin .
$ mv a2a-myplugin ~/bin/
```

A complete, runnable example lives in
[`examples/a2a-hello`](../examples/a2a-hello).

## The plugin contract (any language)

The devkit is the easy path, but the contract is minimal enough to implement in
any language.

### Naming

The binary must be named `a2a-<name>` and be executable. Binaries in the
`a2a-transport-*` namespace are handled separately by the transport plugin
system and are excluded here.

### Invocation

When the user runs `a2a <name> [args...]`, the CLI execs the binary with
`os.Args[1:]` set to `[args...]`. All flags and positional arguments are
forwarded as-is — the plugin owns its own argument parsing. The full
environment is inherited.

### `info` (optional)

If invoked as `a2a-<name> info`, the binary may print a single JSON object to
stdout and exit 0:

```json
{"name":"myplugin","version":"1.0.0","description":"Does something useful"}
```

Used by `a2a plugin list` and to populate the description in `a2a --help`. If
the binary does not respond within 3 seconds, or exits non-zero, the CLI falls
back gracefully — the command is still registered, just without a description.

### Minimal shell example

```sh
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

## Conflict rules

Built-in commands always win. A plugin named `a2a-send` or `a2a-help` will be
silently ignored — the built-in `send` and `help` commands take precedence.
First match in `PATH` wins for duplicate plugin names (same as shell behaviour).

## Relation to transport plugins

Command plugins extend the CLI's command surface. Transport plugins
(`a2a-transport-<name>`) extend the set of A2A transport bindings. They use
different mechanisms — command plugins are exec'd directly; transport plugins
run a loopback proxy server. See [transport-plugins.md](transport-plugins.md)
for details.
