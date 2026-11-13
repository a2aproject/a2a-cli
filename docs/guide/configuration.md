# Configuration

Every flag can also be set in the environment or a file, so repeated commands stay short. `a2a config show` prints each setting and where its value came from.

| Where | Example | Applies to |
|---|---|---|
| A flag | `--timeout 2m` | One command. |
| An environment variable | `export A2ACLI_TIMEOUT=2m` | The shell session. |
| `.env` | `A2ACLI_TIMEOUT=2m` | Commands run in that directory or below. |
| `--config <file>` | `--config ./prod.yaml` | One command, in place of `.env`. |
| `~/.config/a2a-cli/config.yaml` | `timeout: 2m` | Every command. |
| `~/.config/a2a-cli/.env` | `A2ACLI_TIMEOUT=2m` | Every command. |

The first match in this order wins. The variable name is the flag's long name in upper snake case with an `A2ACLI_` prefix: `--agent-card` is `A2ACLI_AGENT_CARD`. YAML and JSON files use the flag name: `agent-card`.

## A project `.env`

`a2a` reads the nearest `.env` file above the working directory:

```dotenv
A2ACLI_AGENT_CARD=https://agent.example.com
A2ACLI_TIMEOUT=2m
A2ACLI_TRANSPORT=jsonrpc,rest
```

Lists are comma-separated in variables and `.env` files.

## Your defaults

```yaml title="~/.config/a2a-cli/config.yaml"
output: text
timeout: 1m
credential-helper:
  - command: [a2a-serve, credential]
```

## See what won

```bash
a2a config show
```

```text
SETTING            VALUE                      SOURCE
agent-card         https://agent.example.com  local-file
credential-helper  a2a-serve credential       user-file
output             text                       default
timeout            2m                         local-file
...
```

Sources are `flag`, `env`, `local-file` (`.env` or `--config`), `user-file` (`config.yaml`), `global-file` (`~/.config/a2a-cli/.env`) and `default`. `-o json` adds the file path. Credentials show as `<redacted>`.

## Exceptions

- `--stream`, `--config`, `--help` and `--version` are only read from the command line.
- `--credential-helper`, `--unix-socket` and `--ca-cert` are ignored, with a warning, when they come from a `.env` found by walking up. A cloned repository could otherwise run a program, redirect requests or choose which certificates to trust. Set them anywhere else.

The [configuration reference](../reference/configuration.md) lists every key, and [Configuration layers](../advanced/configuration.md) explains how they combine.
