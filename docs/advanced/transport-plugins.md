# Transport Plugins

The CLI speaks JSON-RPC, REST and gRPC itself. A transport plugin adds another binding, such as a message bus or a proprietary RPC, without rebuilding the CLI. The plugin is a local proxy: the CLI starts it, the plugin runs a loopback A2A server in a standard binding, and forwards each request to the agent over the custom protocol. This page explains how the CLI loads, connects to and stops a plugin. The [Transport Plugin Protocol](../reference/transport-plugin-protocol.md) reference covers writing one.

## Overview

```text
  a2a send --transport slimrpc --endpoint slim://agents.example/agent "hi"
     |
     | 1. find a2a-transport-slimrpc on PATH
     | 2. start: a2a-transport-slimrpc serve --endpoint slim://agents.example/agent
     | 3. read one handshake line from its stdout
     v
  +----------------+  TLS on 127.0.0.1,     +-------------------------+
  | a2a            |  pinned certificate    | a2a-transport-slimrpc   |
  | proxyTransport | ---------------------> |  loopback A2A server    |
  |                |  standard binding,     |  checks token, strips it|
  +----------------+  A2A-Plugin-Token      |  custom transport ------+--> agent
                                            +-------------------------+
```

The CLI side is `internal/transportplugin`. The plugin side, for Go plugins, is `devkit/clitransport`, which provides the server, the handshake, the token check and the shutdown.

## When a plugin is loaded

A binding other than `JSONRPC`, `HTTP+JSON` and `GRPC` is a custom binding. The plugin for binding `B` is the program `a2a-transport-<b>` found by `exec.LookPath`, with `<b>` in lower case.

| Mode | Behavior |
| --- | --- |
| `--endpoint` with `--transport B` | The plugin must exist, or the command fails and lists the plugins on `PATH`. |
| `--agent-card` | For every binding the card lists, the CLI registers the plugin if it exists. Bindings with no plugin are skipped, and the client picks among the other interfaces. |

Registering a plugin starts nothing. The CLI starts the plugin only when the client selects that interface, once per client. A plugin that fails to start fails that interface, and the client tries the next one. See [Discovery and Transports](discovery.md#interface-selection).

In card mode the lookup covers every binding on the card, including the built-in ones. A program named `a2a-transport-jsonrpc` on `PATH` replaces the built-in JSON-RPC transport for v1.0 interfaces.

`a2a transport list` runs each plugin's `info` subcommand, with a 5-second limit each, and prints what it reports.

## Launch and handshake

The CLI runs:

```bash
a2a-transport-<b> serve --endpoint <interface URL>
```

The interface URL is passed as given, with any scheme. With the devkit, the plugin then:

1. Builds its custom transport for the endpoint.
2. Creates a random per-launch token.
3. Creates a self-signed certificate: ECDSA P-256, valid for 24 hours, for `127.0.0.1`, `::1` and `localhost`.
4. Listens on `127.0.0.1` with a random port and serves TLS with that certificate.
5. Probes itself until it answers: a GET of `/.well-known/agent-card.json` for the HTTP bindings, the standard gRPC health service for `GRPC`. The probe gives up after 10 seconds.
6. Prints one JSON line to stdout and keeps serving.

```json
{"success":true,"payload":{"address":"127.0.0.1:42119","binding":"GRPC","protocol":"1.0","token":"01a1d541-cd1e-70db-8369-259a4072a6f2","certPem":"-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----\n"}}
```

On a startup failure it prints `{"success":false,"error":"..."}` and exits 1.

The CLI reads the first line of stdout and waits up to 15 seconds for it, or less if the request's context ends first. It fails the launch when the line is not valid JSON, `success` is false, or `payload` lacks `address` or `binding`. After the handshake, the CLI reads and discards the rest of stdout, so a plugin that keeps printing does not block on a full pipe.

## The loopback connection

The CLI builds a built-in transport for the binding in the handshake and points it at `address`:

| Binding | Address | Client |
| --- | --- | --- |
| `GRPC` | `host:port` | gRPC client |
| `JSONRPC` | URL | JSON-RPC client |
| `HTTP+JSON` | URL | REST client |

When `certPem` is set, that certificate is the only trusted root, with TLS 1.2 or later. When it is empty, the CLI connects in plaintext. The devkit always sets it.

None of the CLI's connection settings apply to this connection: `--ca-cert`, `--unix-socket`, `--insecure` and the activity tracking for streams all affect only the built-in transports. `--timeout` still bounds each call through the request context.

Every call carries the token in the `A2A-Plugin-Token` service parameter: an HTTP header for the HTTP bindings and gRPC metadata for `GRPC`. The devkit compares it with the expected token in constant time and rejects a call without exactly one matching value as unauthenticated. It removes the token from the service parameters before it passes the call to the custom transport, so the token never reaches the agent. All other service parameters, such as `Authorization`, `A2A-Version` and the credential helper headers, are passed on.

## Shutdown

When the command ends, closing the client stops the plugin:

1. The CLI closes the plugin's stdin.
2. The devkit treats end of input on stdin, SIGINT and SIGTERM as a stop request. It stops the server and closes the custom transport.
3. The CLI waits up to 5 seconds for the process to exit, then kills it.

The exit status of the plugin is not reported. A plugin that ignores stdin is killed after the 5 seconds.

## stderr and environment

The plugin's stderr is the CLI's stderr, so its logs appear in the terminal. Its stdout is reserved for the handshake.

The `serve` process gets the CLI's environment plus every key from the local configuration file (the `--config` file or the discovered `.env`) and the global `.env` that the environment does not set. The user `config.yaml` is not passed. A plugin can read its own settings, such as an upstream token, from the same `.env` files that configure the CLI. The `info` query gets the CLI's environment only. See [Configuration Layers](configuration.md#environment-of-transport-plugins).

## Design notes

- **Why a proxy.** The plugin speaks a binding the CLI already has a client for, so plugins need no CLI-specific RPC, and the CLI treats a plugin interface like any other.
- **Why a token and a pinned certificate.** Other local users and processes can reach a loopback port. The token stops them from calling the proxy, which may hold upstream credentials. The certificate is created per launch and passed over the pipe, so there is no key on disk and no CA to manage.
- **Why stdin for shutdown.** When the CLI exits, even after a crash, the pipe closes and the plugin sees end of input. A signal would not be sent in that case.

## Trust

A transport plugin runs with your permissions and receives your configuration files' values in its environment. Install only plugins you trust. See [Security](security.md).
