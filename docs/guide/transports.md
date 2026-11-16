# Transports

A2A runs over three standard bindings: JSON-RPC, REST (`HTTP+JSON`) and gRPC. `a2a` speaks all three, for protocol versions 1.0 and 0.3, and picks one from the agent card. These flags change the choice and how the connection is made.

| Flag | What it does |
|---|---|
| `--transport <name>` | Prefers `jsonrpc`, `rest`, `grpc` or a [plugin](plugins.md). Repeat to order several. |
| `--endpoint <url>` | Connects to one interface without reading the card. Needs exactly one `--transport`. |
| `--a2a-version <1.0\|0.3>` | Uses only that protocol version. |
| `--tenant <id>` | Sends a tenant ID on every request, for agents that host several. |
| `--insecure` | Connects to gRPC agents without TLS. |
| `--ca-cert <file>` | Trusts only the CA certificates in a PEM file. |
| `--unix-socket <path>` | Sends every connection through a Unix socket. |

## Choose a binding

Without `--transport`, `a2a` tries the card's interfaces from the newest protocol version down, in the card's order, and uses the first that connects. `--transport` reorders interfaces of the same version:

```bash
a2a send -a localhost:8080 --transport rest --transport jsonrpc "hello"
```

`--verbose` prints which card was read and when the client is created.

## gRPC

gRPC connections use TLS and the system's certificates by default. For a local agent serving plaintext gRPC, add `--insecure`; `a2a` warns when that sends a credential. The hint on a TLS handshake error suggests it.

```bash
a2a send -e 127.0.0.1:50051 --transport grpc --insecure "hello"
```

## Private certificates and sockets

`--ca-cert` replaces the system's trusted roots for the card request and all three bindings, for agents with a certificate from a private CA. `--unix-socket` routes every HTTP request and gRPC connection through a socket, whatever host the URL names; a2a-serve uses it to let a sandboxed agent call other agents. Neither is read from a project `.env`.

[Discovery and transports](../advanced/discovery.md) describes the selection in detail.
