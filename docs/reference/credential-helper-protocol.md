# Credential Helper Protocol

A credential helper is a program that the CLI runs to get the HTTP headers for a URL. The CLI implements the [Bazel credential helper protocol](https://github.com/EngFlow/credential-helper-spec), so helpers written for Bazel work unchanged. This page is the wire format. [Credential Helpers](../advanced/credential-helpers.md) explains when the CLI runs a helper and which URL it passes.

## Invocation

The CLI runs the configured command with `get` appended:

| Configured as | Runs |
| --- | --- |
| `--credential-helper /usr/local/bin/helper` | `/usr/local/bin/helper get` |
| `command: [a2a-serve, credential]` | `a2a-serve credential get` |

The program is found on `PATH` when it has no `/`. It runs in the CLI's working directory with the CLI's process environment. `--timeout` bounds the run; when it expires, the process is killed.

## Request

One JSON object on stdin, then end of input:

```json
{"uri":"https://agent.example.com"}
```

| Field | Content |
| --- | --- |
| `uri` | The agent card reference for the card request, or the interface URL for A2A calls. A gRPC `host:port` interface is sent as `https://host:port`. No well-known path or method path is appended. |

## Response

One JSON object on stdout:

```json
{
  "headers": {
    "Authorization": ["Bearer eyJhbGciOi..."],
    "X-Goog-User-Project": ["my-project"]
  },
  "expires": "2026-11-13T15:04:05Z"
}
```

| Field | Type | Content |
| --- | --- | --- |
| `headers` | object of string arrays | Header names and their values. Optional. An empty name is an error. |
| `expires` | string | When the headers stop being valid, in RFC 3339. Optional. A value that is not RFC 3339 is an error. |

Other fields are ignored. A helper with no credentials for the URI prints `{}`, and the request goes out without its headers.

The CLI caches the response per URI for the rest of the command, until `expires` when present. A header that `--auth` or `--svc-param` sets is not overridden by the helper.

## Exit status

| Exit | Result |
| --- | --- |
| 0 | stdout is parsed as the response |
| non-zero | The command fails. The helper's stderr is included in the error message. |

On exit 0, stderr is discarded. Write diagnostics to stderr, never to stdout.

## Scope syntax

The command line and the environment take `[scope=]path`. Files take `{scope, command}` objects.

| Scope | Matches |
| --- | --- |
| *(none)* | Every host |
| `example.com` | `example.com` only |
| `*.example.com` | `example.com` and every subdomain |

Scopes are hosts without a port, compared in lower case. The most specific matching scope wins: an exact host, then the longest wildcard, then unscoped helpers. Among equals, the last one configured wins.

```yaml
# ~/.config/a2a-cli/config.yaml
credential-helper:
  - command: [/usr/local/bin/default-helper]
  - scope: "*.corp.example.com"
    command: [/usr/local/bin/corp-helper, --profile, work]
```

```bash
export A2ACLI_CREDENTIAL_HELPER='agent.example.com=/usr/local/bin/token-helper'
```

## Example: a shell helper

A helper that returns a bearer token from an environment variable for one host, and nothing for others:

```bash
#!/bin/sh
# token-helper: a2a credential helper
[ "$1" = get ] || { echo "usage: $0 get" >&2; exit 2; }
uri=$(jq -r .uri)
case "$uri" in
  https://agent.example.com|https://agent.example.com/*)
    [ -n "$AGENT_TOKEN" ] || { echo "AGENT_TOKEN is not set" >&2; exit 1; }
    jq -n --arg t "$AGENT_TOKEN" '{headers: {Authorization: ["Bearer " + $t]}}'
    ;;
  *)
    echo '{}'
    ;;
esac
```

Test it alone, then through the CLI:

```bash
echo '{"uri":"https://agent.example.com"}' | ./token-helper get
a2a auth get https://agent.example.com --credential-helper agent.example.com=$PWD/token-helper --reveal
```

`a2a auth get` prints the helper the CLI selected and the headers it returned:

```text
helper: agent.example.com=/home/me/token-helper
HEADER         VALUE
Authorization  Bearer abc123
```

A helper that mints short-lived tokens should set `expires` so that a long `chat` session fetches a new one.
