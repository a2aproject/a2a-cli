# Credentials

Agents that need credentials read them from request headers. `a2a` sends them from a flag, or asks a credential helper for them per agent.

| Option | Sends |
|---|---|
| `--auth "<value>"` | `Authorization: <value>` on every request. |
| `--svc-param key=value` | Any header, repeatable. `key:value` works too. |
| `--credential-helper [scope=]path` | The headers a helper program returns for the agent's URL. |
| `a2a auth get <url>` | Prints which helper answers for a URL and the header names it returns. |

```bash
a2a send -a https://agent.example.com --auth "Bearer $TOKEN" "hello"
a2a send -a https://agent.example.com --svc-param "X-API-Key=$KEY" "hello"
```

The same headers go with the card request, so a gateway that protects the card accepts them too. On gRPC they travel as metadata.

## Credential helpers

A credential helper is a program that prints the headers for a URL. `a2a` uses the [Bazel credential helper protocol](https://github.com/EngFlow/credential-helper-spec), so helpers written for Bazel work. List them in `~/.config/a2a-cli/config.yaml`:

```yaml
credential-helper:
  - command: [a2a-serve, credential]          # every host
  - scope: "*.corp.example.com"
    command: [/usr/local/bin/corp-token, --audience, a2a]
```

The helper with the most specific matching scope runs. Its headers are cached until they expire, and `--auth` or `--svc-param` win over a header it returns. [a2a-serve](https://github.com/a2aproject/a2a-serve) ships one: `a2a-serve doctor --fix` adds it, and it returns the tokens a2a-serve keeps for agents you started or joined.

```bash
a2a auth get https://agent.example.com            # header names, values redacted
a2a auth get https://agent.example.com --reveal   # header values
```

```text
helper: a2a-serve credential
HEADER         VALUE
Authorization  <redacted>
```

[A credential helper](../../examples/credential-helper/README.md) shows how to write one, and [Credential helpers](../advanced/credential-helpers.md) how they're chosen.

## Keep secrets out of shared files

`config show` redacts `--auth` and `Authorization` parameters, but a `.env` file stores them in plain text. Keep tokens in the environment, a private `~/.config/a2a-cli` file, or a helper. `--credential-helper` is ignored when it comes from a project `.env`, so a cloned repository can't make `a2a` run a helper of its choice. [Security](../advanced/security.md) lists what a project `.env` can set.
