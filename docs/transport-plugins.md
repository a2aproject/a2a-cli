# Custom transport plugins

The `a2a` CLI speaks JSON-RPC, REST and gRPC natively. Any other transport
binding (a proprietary message bus, a SLIM variant, WebSockets, …) can be added
**without recompiling the CLI** by installing a *transport plugin* binary on your
`PATH`.

A plugin is a small **local proxy**: the CLI launches it as a subprocess, the
plugin stands up a loopback A2A server that speaks one of the standard bindings,
and it forwards every request to the custom upstream.

```
  a2a send --transport slimrpc --endpoint slim://agents.example/agent "hi"
        │
        │  1. discover a2a-transport-slimrpc on PATH
        │  2. launch:  a2a-transport-slimrpc serve --endpoint slim://agents.example/agent
        ▼
  ┌─────────────────────────┐      standard A2A (jsonrpc/rest/grpc) on 127.0.0.1
  │ a2a (host CLI)          │ ────────────────┐
  └─────────────────────────┘                 │
                                              ▼
                   ┌─────────────────────────────────┐
                   │ a2a-transport-slimrpc (plugin)  │
                   │  loopback A2A server  ──►  SLIM │──► upstream agent
                   └─────────────────────────────────┘
```

## Using a plugin

1. Put the plugin binary on your `PATH`, named `a2a-transport-<name>`.
2. List what's installed:

   ```console
   $ a2a transport list
   NAME  VERSION  PROTOCOL  DESCRIPTION                    PATH
   echo  1.0.0    1.0       Echoes the caller's message…   /usr/local/bin/a2a-transport-echo
   ```

3. Use it like any built-in transport, either explicitly:

   ```console
   $ a2a send --transport echo --endpoint echo://demo "hello"
   ```

   or automatically, when an agent card advertises a `protocolBinding` that
   matches an installed plugin:

   ```console
   $ a2a send -a https://agents.example.com "hello"   # card says binding "echo" → plugin used
   ```

If no plugin matches an explicitly requested `--transport`, the CLI errors and
lists the plugins it did find.

> **Trust:** a transport plugin is an executable you install yourself. Treat it
> like any other binary on your `PATH` — only install plugins you trust. The CLI
> does not sandbox plugins; it only isolates them in a subprocess and secures the
> loopback channel with per-session TLS and a per-launch token (see below).

## Writing a plugin in Go (recommended)

Use the devkit at
[`github.com/a2aproject/a2a-cli/devkit/clitransport`](../devkit/clitransport).
You supply an [`a2aclient.Transport`](https://pkg.go.dev/github.com/a2aproject/a2a-go/v2/a2aclient#Transport)
that implements your custom protocol; the devkit turns it into a CLI-compatible
plugin binary — subcommand parsing, the loopback server, the handshake, the
token check and graceful shutdown are all handled for you.

```go
package main

import (
	"context"

	"github.com/a2aproject/a2a-cli/devkit/clitransport"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
)

func main() {
	clitransport.Main(clitransport.Config{
		Name:        "slimrpc",
		Version:     "1.0.0",
		Description: "SLIM RPC transport for A2A",
		NewTransport: func(ctx context.Context, endpoint string) (a2aclient.Transport, error) {
			// Return your custom client transport, connected to `endpoint`.
			return slim.Dial(ctx, endpoint)
		},
	})
}
```

Build it with the required name and drop it on your `PATH`:

```console
$ go build -o a2a-transport-slimrpc .
$ mv a2a-transport-slimrpc ~/bin/    # somewhere on PATH
```

A complete, runnable example lives in
[`examples/a2a-transport-echo`](../examples/a2a-transport-echo).

## The plugin contract (any language)

The devkit is the easy path, but the contract is deliberately simple so a plugin
can be written in any language that can run an HTTP or gRPC server. A plugin
binary named `a2a-transport-<name>` must implement two subcommands.

### `info`

Print a single JSON object to stdout and exit 0:

```json
{"name":"slimrpc","version":"1.0.0","description":"SLIM RPC transport","protocol":"1.0","binding":"jsonrpc"}
```

Used by `a2a transport list`.

### `serve --endpoint <url>`

1. Start an A2A server on a loopback address using one of the standard bindings
   (`jsonrpc`, `rest`, or `grpc`), backed by your custom transport.
2. Generate a random per-launch **token**.
3. **Secure the channel with TLS (recommended).** Mint an ephemeral, self-signed
   certificate valid for loopback (`127.0.0.1`/`::1`/`localhost`), serve TLS with
   it, and advertise it as `certPem` in the handshake. The host pins that exact
   certificate as its only trusted root, so there is no CA or on-disk key to
   manage. TLS is optional: omit `certPem` to serve plaintext (the token still
   authenticates the caller), which keeps very simple non-Go plugins easy to
   write. The devkit always does TLS for you.
4. **Confirm the server is actually accepting requests** (a readiness probe)
   before announcing — the devkit gates the handshake on an HTTPS GET to the card
   path for the HTTP bindings and on the standard gRPC health service for `grpc`.
5. Print exactly one JSON line to stdout — the **handshake envelope** — then keep
   serving. On success:

   ```json
   {"success":true,"endpoint":{"address":"127.0.0.1:53821","binding":"GRPC","protocol":"1.0","token":"9f3c…","certPem":"-----BEGIN CERTIFICATE-----\n…\n-----END CERTIFICATE-----\n"}}
   ```

   and on a startup failure (so the host can report a clean error instead of a
   dropped connection):

   ```json
   {"success":false,"error":"creating upstream transport: dial slim://…: connection refused"}
   ```

   The `endpoint` fields are:
   * `address` — where the host connects. A full `https://…` URL (or `http://…`
     when serving plaintext) for `jsonrpc` and `rest`; a `host:port` dial target
     for `grpc`.
   * `binding` — the standard binding your server speaks.
   * `protocol` — the A2A protocol version (e.g. `1.0`).
   * `token` — the shared secret for this launch.
   * `certPem` — the PEM-encoded certificate the host must pin to dial over TLS.
     Omit (or leave empty) to serve plaintext.
6. On **every** request, the host presents the token as the `A2A-Plugin-Token`
   service parameter (an HTTP header for `jsonrpc`/`rest`, gRPC metadata for
   `grpc`). Reject any request whose token does not match, and do **not** forward
   the token upstream.
7. Other request service parameters (e.g. `Authorization`) should be forwarded to
   the upstream as your protocol requires.
8. Shut down cleanly when **stdin reaches EOF** (the host closes it to stop you)
   or on `SIGTERM`/`SIGINT`.

Anything the plugin writes to stderr is surfaced to the user for debugging.

### Minimal non-Go example (`info` in shell)

The `info` half of the contract is trivial in any language. Here it is in POSIX
shell:

```sh
#!/bin/sh
# a2a-transport-demo
case "$1" in
  info)
    printf '%s\n' '{"name":"demo","version":"0.1.0","binding":"JSONRPC","protocol":"1.0"}'
    ;;
  serve)
    # Start your A2A JSON-RPC server on 127.0.0.1:PORT, confirm it is accepting
    # requests, then announce the handshake envelope. This stub serves plaintext
    # (no certPem), so the token alone authenticates the caller; add a certPem
    # field and serve https:// to secure the channel with TLS:
    #   printf '%s\n' '{"success":true,"payload":{"address":"http://127.0.0.1:PORT","binding":"JSONRPC","protocol":"1.0","token":"'"$TOKEN"'"}}'
    # and keep serving until stdin closes. On failure, announce instead:
    #   printf '%s\n' '{"success":false,"error":"could not reach upstream"}'
    echo "serve not implemented in this stub" >&2
    printf '%s\n' '{"success":false,"error":"serve not implemented in this stub"}'
    exit 1
    ;;
esac
```

The `serve` half needs a real A2A server for your chosen binding. In Python, for
example, you can build the JSON-RPC/REST server with the
[A2A Python SDK](https://github.com/a2aproject/a2a-python) and print the same
handshake line. In Go, the devkit does all of this for you.

## How the host selects a plugin

`internal/flagparse` keeps `rest`, `jsonrpc` and `grpc` as built-in aliases and
treats any other `--transport` value as a custom binding
(`a2a.TransportProtocol` is explicitly not an enum in the A2A spec). When a
client is built:

* **`--endpoint` mode** — an explicitly named custom transport must resolve to a
  plugin, otherwise the CLI errors.
* **card mode** — the CLI registers a plugin for each custom `protocolBinding`
  the card advertises (and for any custom `--transport` preference). Card
  bindings with no installed plugin are skipped so other interfaces can still be
  tried.

The registered factory (`internal/transportplugin`) launches the plugin, reads
the handshake, and builds a built-in transport pointed at the loopback address —
pinning the handshake's `certPem` as the sole trusted root when the plugin serves
TLS — with the token injected on every call. When the CLI is done, closing the
client tears the subprocess down.
