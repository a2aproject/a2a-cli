# Credential Helpers

A credential helper is a program that returns the HTTP headers to send to an agent. The CLI implements the [Bazel credential helper protocol](https://github.com/EngFlow/credential-helper-spec), so helpers written for Bazel work unchanged. This page explains how the CLI picks a helper, what URL it passes, how it caches the answer and how the headers combine with `--auth` and `--svc-param`. The [Credentials guide](../guide/credentials.md) shows setup, and the [Credential Helper Protocol](../reference/credential-helper-protocol.md) reference has the wire format.

## Configuring helpers

There are two forms.

On the command line and in environment variables, a helper is `[scope=]path`, the form Bazel's `--credential_helper` takes. `path` is a single program with no arguments. The text before the first `=` is a scope when it looks like a host pattern: non-empty, no spaces, slashes, backslashes or quotes, and at most a leading `*.` wildcard. Otherwise the whole value is the path.

```bash
a2a send --credential-helper '*.corp.example.com=/usr/local/bin/corp-helper' \
         --credential-helper /usr/local/bin/default-helper -a agent.corp.example.com "hi"
```

In YAML and JSON files, `credential-helper` is a list of objects with `scope` (optional) and `command` (a program and its arguments). Unknown keys are an error.

```yaml
# ~/.config/a2a-cli/config.yaml
credential-helper:
  - command: [a2a-serve, credential]      # every host
  - scope: "*.corp.example.com"
    command: [/usr/local/bin/corp-helper, --profile, work]
```

A YAML or JSON string, an environment variable and a `.env` value are each one `[scope=]path` spec, commas included.

A scope is a host without a port, such as `example.com`, or `*.example.com` for the domain and all its subdomains. Scopes are compared in lower case. A scope with a `:` is an error.

`--credential-helper` is a protected setting: a `.env` found by walking up from the working directory cannot set it, because it runs a program. See [Configuration Layers](configuration.md).

## Choosing a helper

For each URL, `credhelper.Select` ranks the helpers whose scope matches the URL's host:

| Rank | Scope |
| --- | --- |
| Highest | Exactly the host |
| | `*.domain` matching the host, longer domains first |
| Lowest | No scope |

`*.example.com` matches `example.com` itself and any name ending in `.example.com`. Among equal ranks the last helper wins, so a later entry overrides an earlier one. When no helper matches, no helper runs and no headers are added.

`a2a auth get <url>` prints the selected helper and its headers, with the values redacted unless you pass `--reveal`:

```bash
a2a auth get https://agent.corp.example.com
```

## The URL the helper receives

The helper receives the URL of the target as JSON on stdin, `{"uri": "..."}`, and the same URL selects the helper.

| Request | URI |
| --- | --- |
| Card request | The card reference as given to `--agent-card` or `card get`, after [normalization](discovery.md#reference-normalization). Only `http` and `https` references; a `file://` card runs no helper. |
| A2A calls | The URL of the interface the client connected to. A gRPC `host:port` address gets an `https://` prefix. |
| `auth get` | The argument, normalized. It must be `http` or `https`. |

Bazel sends the full URL of each request. The CLI sends the reference or interface URL without appending `/.well-known/agent-card.json` or the A2A method path, because the call interceptor does not see the method path. A helper written for Bazel that keys on paths may need to match by host or prefix.

The card reference and the interface URL often differ, for example `https://agent.example.com` and `https://agent.example.com/a2a/v1`. The helper runs once for each URI.

## Applying the headers

On the card request, the helper's headers are added as request headers. On A2A calls, `credhelper.Interceptor` adds them as service parameters to every call, which the transport sends as HTTP headers or gRPC metadata.

A header that `--auth` or `--svc-param` already sets is skipped. An explicit `--auth` therefore always wins over the helper's `Authorization`, and the helper still supplies its other headers.

When `--insecure` is set and the helper's headers include `Authorization`, the CLI warns once on stderr:

```text
warning: --insecure disables TLS for gRPC; a credential sent over gRPC may be exposed.
```

`--insecure` affects only gRPC, so the warning appears whatever the selected transport. The same warning appears at client creation when `--auth` or an `Authorization` service parameter is set.

## Running and caching

`credhelper.Source` runs the selected helper the first time a URI is needed, and caches the response per URI for the rest of the command:

- A response with `expires` (RFC 3339) is reused until that time, then the helper runs again. An `expires` value that is not RFC 3339 fails the command.
- A response without `expires` is reused for the whole command.
- The cache lives in memory and ends with the process.

The helper runs as `<command...> get`. Its stderr is captured and added to the error message when it fails.

| Outcome | Result |
| --- | --- |
| Exit 0 with `{"headers": {...}}` | Headers applied |
| Exit 0 with `{}` | No headers; the request goes out without credentials |
| Non-zero exit | The command fails, with the helper's stderr in the message |
| Output that is not JSON, or an empty header name | The command fails |
| Runs longer than the timeout | Killed; the command fails with exit code 5 |

The run is bounded by the context of the request that needs it. For the card request and the first call that is the `--timeout` of client creation or of the call. In streaming commands it is the inactivity timer. A slow helper uses up part of the request's budget. `-v` logs each run:

```text
# asking credential helper "a2a-serve credential" for http://127.0.0.1:35261
```

## a2a-serve

[a2a-serve](https://github.com/a2aproject/a2a-serve) ships a helper, `a2a-serve credential`, that returns the tokens saved by `a2a-serve pair join`. `a2a-serve doctor --fix` adds it to `~/.config/a2a-cli/config.yaml` as an unscoped entry:

```yaml
credential-helper:
  - command: [a2a-serve, credential]
```
