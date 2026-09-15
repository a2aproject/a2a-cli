# a2a-hello

A reference [A2A CLI command plugin](../../docs/command-plugins.md) built with
the [`cliplugin` devkit](../../devkit/cliplugin).

It implements the optional `info` subcommand and accepts an optional name
argument. It exists to demonstrate the command plugin contract end to end.

## Build and install

```console
$ go build -o a2a-hello .
$ mv a2a-hello ~/bin/    # anywhere on your PATH
```

## Try it

```console
$ a2a plugin list
NAME   VERSION  DESCRIPTION                              PATH
hello  1.0.0    Greets the world — reference command plugin  …/a2a-hello

$ a2a hello
Hello, World! (from a2a-hello plugin)

$ a2a hello Alice
Hello, Alice! (from a2a-hello plugin)

$ a2a --help
# "hello" appears in the Available Commands list
```

## What to look at

* [`main.go`](main.go) — the entire plugin; wires info via `cliplugin.ServeInfo`.

Everything else — PATH discovery, invocation, argument forwarding, and
`--help` registration — is handled automatically by the `a2a` CLI.
