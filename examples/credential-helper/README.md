# A Credential Helper

This example writes a credential helper: a small program that tells `a2a` which headers to send to each agent. This one reads bearer tokens per host from a file. Real helpers usually fetch short-lived tokens from a login session, a secrets manager or a cloud CLI; the contract is the same.

![Writing a token file, configuring the helper with a scope and checking it with auth get](demo.gif)

## Prerequisites

`a2a` and [jq](https://jqlang.org). No agent is needed: `a2a auth get` runs the helper without calling one.

## Run it

Write a token file, one `host token` pair per line:

```bash
mkdir -p ~/.config/a2a-cli
printf 'agent.example.com s3cr3t-1\nreview.example.org s3cr3t-2\n' > ~/.config/a2a-cli/tokens
chmod 600 ~/.config/a2a-cli/tokens
```

Ask `a2a` what it would send:

```bash
a2a auth get https://agent.example.com --credential-helper "$PWD/token-file-helper" --reveal
```

```text
helper: /home/you/a2a-cli/examples/credential-helper/token-file-helper
HEADER         VALUE
Authorization  Bearer s3cr3t-1
```

A host without a token gets no headers:

```bash
a2a auth get https://other.example.net --credential-helper "$PWD/token-file-helper"
```

```text
helper: /home/you/a2a-cli/examples/credential-helper/token-file-helper
no headers
```

## Scope it

A scope limits a helper to some hosts: `example.com` matches that host, `*.example.com` the domain and its subdomains. On the command line, put it before the path:

```bash
a2a auth get https://review.example.org --credential-helper "agent.example.com=$PWD/token-file-helper"
```

```text
Error: no credential helper for https://review.example.org
```

To use the helper for every command, list it in `~/.config/a2a-cli/config.yaml`. With several helpers, the one with the most specific matching scope runs:

```yaml
credential-helper:
  - command: [a2a-serve, credential]
  - scope: "*.example.com"
    command: [/home/you/bin/token-file-helper]
```

## How it works

`a2a` runs the helper with the argument `get`, writes the agent's URL to its stdin as JSON, and reads headers from its stdout:

```bash
echo '{"uri": "https://agent.example.com"}' | ./token-file-helper get
```

```json
{
  "headers": {
    "Authorization": [
      "Bearer s3cr3t-1"
    ]
  }
}
```

[`token-file-helper`](token-file-helper) does that in a few lines of bash: it takes the host from the URL, looks it up in the file, and prints the header, or `{}` for an unknown host. A helper may add `"expires": "<RFC 3339 time>"`, and `a2a` reuses the headers until then. A non-zero exit fails the command, with the helper's stderr in the message.

This is the [Bazel credential helper protocol](https://github.com/EngFlow/credential-helper-spec), so helpers written for Bazel work with `a2a`. The [protocol reference](../../docs/reference/credential-helper-protocol.md) has the details, and [Credential helpers](../../docs/advanced/credential-helpers.md) how `a2a` chooses and caches them.
