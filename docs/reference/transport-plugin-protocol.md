# Transport Plugin Protocol

A transport plugin is an executable named `a2a-transport-<name>` that adds the binding `<name>` to the CLI. It implements two subcommands: `info`, which describes it, and `serve`, which starts a loopback A2A server in a standard binding and forwards each call to the agent over the custom protocol. This page is the full contract, as implemented by `internal/transportplugin` on the CLI side and `devkit/clitransport` on the plugin side. [Transport Plugins](../advanced/transport-plugins.md) explains when the CLI loads a plugin and why it works this way.

## Naming

The plugin for binding `B` is `a2a-transport-<b>`, with `<b>` the binding in lower case, found on `PATH`. The binding name is what a card lists in `protocolBinding` and what users pass to `--transport`. Built-in binding names (`jsonrpc`, `grpc`, `http+json`) work too and replace the built-in transport when a card lists that binding.

## `info`

`a2a-transport-<name> info` prints one JSON object to stdout and exits 0:

```json
{"name":"echo","version":"1.0.0","description":"Echoes the caller's message back as a completed task","protocol":"1.0","binding":"GRPC"}
```

| Field | Type | Content |
| --- | --- | --- |
| `name` | string | The transport name |
| `version` | string | The plugin's own version |
| `description` | string | Optional summary |
| `protocol` | string | Optional A2A protocol version the plugin targets, such as `1.0` |
| `binding` | string | Optional default loopback binding: `GRPC`, `JSONRPC` or `HTTP+JSON` |

Only `a2a transport list` runs `info`, with a 5-second limit and the CLI's process environment. It shows `version`, `protocol` and `description`; the `NAME` column comes from the file name.

## `serve`

The CLI starts the plugin when a client selects one of its interfaces:

```bash
a2a-transport-<name> serve --endpoint <interface URL>
```

`<interface URL>` is the interface URL from the card, or the `--endpoint` value, unchanged.

| Stream | Use |
| --- | --- |
| stdin | Held open by the CLI. End of input means stop. |
| stdout | The handshake line, first. Anything after it is read and discarded. |
| stderr | Shown to the user as is. Use it for logs. |

The environment is the CLI's process environment plus every key from the local configuration file (`--config` or the discovered `.env`) and the global `~/.config/a2a-cli/.env` that the environment does not set. The user `config.yaml` is not passed.

The devkit also accepts `--bind GRPC|JSONRPC|HTTP+JSON` to override the default binding. The CLI never passes it.

### What `serve` must do

1. Create the upstream connection for `--endpoint`.
2. Generate a random token for this launch.
3. Listen on a loopback address and serve one standard binding.
4. Optionally serve TLS with a self-signed certificate for `127.0.0.1`, `::1` and `localhost`. The devkit always does, with an ECDSA P-256 certificate valid for 24 hours.
5. Make sure the server accepts requests before announcing it. The devkit probes `/.well-known/agent-card.json` for the HTTP bindings and the gRPC health service for `GRPC`, for up to 10 seconds.
6. Print the handshake line within 15 seconds of starting.
7. Check the token on every call and remove it before forwarding.
8. Stop when stdin reaches end of input. The devkit also stops on SIGINT and SIGTERM.

## Handshake

One line of JSON on stdout, ending with a newline.

On success:

```json
{"success":true,"payload":{"address":"127.0.0.1:42119","binding":"GRPC","protocol":"1.0","token":"01a1d541-cd1e-70db-8369-259a4072a6f2","certPem":"-----BEGIN CERTIFICATE-----\nMIIBxTCC...\n-----END CERTIFICATE-----\n"}}
```

On failure, followed by a non-zero exit:

```json
{"success":false,"error":"creating upstream transport: dial slim://agents.example/agent: connection refused"}
```

| Field | Type | Content |
| --- | --- | --- |
| `success` | bool | Whether the server started and passed its readiness check |
| `error` | string | Why it failed; on failure only |
| `payload` | object | Connection details; on success only |
| `payload.address` | string | `host:port` for `GRPC`. A URL for `JSONRPC` and `HTTP+JSON`: `https://127.0.0.1:<port>` with TLS, `http://127.0.0.1:<port>` without. |
| `payload.binding` | string | `GRPC`, `JSONRPC` or `HTTP+JSON`, exactly. Lower-case names are rejected. |
| `payload.protocol` | string | The A2A protocol version the server speaks, such as `1.0`. Informational; the CLI does not use it. |
| `payload.token` | string | The per-launch token |
| `payload.certPem` | string | The PEM certificate the server presents. The CLI trusts only this certificate, with TLS 1.2 or later. Empty or absent means plaintext. |

The CLI fails the launch when the line is missing after 15 seconds, is not JSON, has `success: false`, or lacks `payload.address` or `payload.binding`. A failed launch fails that interface, and the CLI tries the next interface on the card if there is one.

## Calls

The CLI connects to `address` with its built-in client for `binding` and sends every call with the service parameter `A2A-Plugin-Token: <token>`, as an HTTP header for `JSONRPC` and `HTTP+JSON` and as gRPC metadata for `GRPC`.

The plugin must:

- Reject a call that does not carry exactly one `A2A-Plugin-Token` value equal to the token. The devkit compares in constant time and returns an unauthenticated error.
- Remove `A2A-Plugin-Token` before forwarding.
- Forward the other service parameters, such as `Authorization`, `A2A-Version` and headers from credential helpers, as the upstream protocol requires.

## Shutdown

When the command ends, the CLI closes the plugin's stdin and waits up to 5 seconds for it to exit, then kills it. The exit status is ignored. A Ctrl-C in the terminal also sends SIGINT to the plugin, since it shares the CLI's process group.

## Go: `devkit/clitransport`

The devkit implements everything above. You provide an `a2aclient.Transport` for the custom protocol:

```go
package main

import (
	"context"

	"github.com/a2aproject/a2a-cli/devkit/clitransport"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
)

func main() {
	clitransport.Main(clitransport.Config{
		Name:        "echo",
		Version:     "1.0.0",
		Description: "Echoes the caller's message back as a completed task",
		NewTransport: func(_ context.Context, endpoint string) (a2aclient.Transport, error) {
			return newEchoTransport(endpoint), nil
		},
	})
}
```

| `Config` field | Meaning |
| --- | --- |
| `Name` | Required. Reported by `info`. |
| `Version`, `Description` | Reported by `info` |
| `Binding` | Loopback binding to serve: `a2a.TransportProtocolGRPC` (default), `a2a.TransportProtocolJSONRPC` or `a2a.TransportProtocolHTTPJSON` |
| `NewTransport` | Required. Builds the upstream transport for the `--endpoint` URL. Its `Destroy` is called at shutdown. |

`clitransport.Main` reads `os.Args`, handles `info` and `serve`, prints the handshake, and exits 1 on error. `clitransport.Run` does the same with explicit arguments and writers, for tests.

```bash
go build -o ~/bin/a2a-transport-echo .
a2a transport list
a2a send --transport echo --endpoint echo://demo "hello there"
```

```text
Task:     01a1d541-d526-7691-b033-2a7f14402905
Context:  01a1d541-d526-7694-b601-235862aef7e5
Status:   completed
Artifacts:
  [01a1d541-d526-7696-8659-901a097265df] hello there
History:
  [user] hello there
```

## Other languages

Any language that can run a JSON-RPC, REST or gRPC A2A server can implement the contract. `info` is short in shell:

```bash
#!/bin/sh
# a2a-transport-demo
case "$1" in
  info)
    printf '%s\n' '{"name":"demo","version":"0.1.0","protocol":"1.0","binding":"JSONRPC"}'
    ;;
  serve)
    # Start an A2A JSON-RPC server on 127.0.0.1:$PORT that checks A2A-Plugin-Token,
    # wait until it answers, then announce it and keep serving until stdin closes:
    #   printf '%s\n' '{"success":true,"payload":{"address":"http://127.0.0.1:'"$PORT"'","binding":"JSONRPC","protocol":"1.0","token":"'"$TOKEN"'"}}'
    printf '%s\n' '{"success":false,"error":"serve is not implemented in this stub"}'
    exit 1
    ;;
esac
```

This stub serves plaintext, so only the token protects the loopback port. Add `certPem` and serve `https://` to use TLS.
