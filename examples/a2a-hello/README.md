# A Command Plugin

`a2a-hello` adds a command, `a2a hello`, to the CLI without rebuilding it. It is a complete command plugin in about 40 lines of Go, built with the [`cliplugin`](../../devkit/cliplugin) devkit.

![Building a2a-hello, enabling command plugins, listing and running it](demo.gif)

## Build and install

From this directory, build the binary with its required name and put it on your `PATH`:

```bash
go build -o ~/bin/a2a-hello .    # any directory on PATH
```

Command plugins are off by default. Turn them on once:

```bash
a2a plugin set-enabled true
```

## Try it

```bash
a2a plugin list
```

```text
Command plugins: enabled
NAME   VERSION  DESCRIPTION                                  PATH
hello  1.0.0    Greets the world — reference command plugin  /home/you/bin/a2a-hello
```

```bash
a2a hello
a2a hello Alice
```

```text
Hello, World! (from a2a-hello plugin)
Hello, Alice! (from a2a-hello plugin)
```

`hello` also appears under **Available Commands** in `a2a --help`.

## How it works

`a2a` finds every executable named `a2a-<name>` on `PATH` and registers it as a command. `a2a hello Alice` runs `a2a-hello Alice`: the plugin gets every argument and your environment, and owns its output and exit code.

The one thing a plugin may implement for `a2a` is the `info` subcommand, which describes it in `plugin list` and `--help`. [`main.go`](main.go) answers it with `cliplugin.ServeInfo`:

```go
if len(os.Args) > 1 && os.Args[1] == cliplugin.SubcommandInfo {
	cliplugin.ServeInfo(cliplugin.Info{
		Name:        "hello",
		Version:     "1.0.0",
		Description: "Greets the world — reference command plugin",
	})
}
```

Any language works; `info` only has to print `{"name": ..., "version": ..., "description": ...}`. The [command plugin protocol](../../docs/reference/command-plugin-protocol.md) has the contract, and [Command plugins](../../docs/advanced/command-plugins.md) explains discovery and conflicts.
