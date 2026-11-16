# Configuration

Every setting of the `a2a` CLI, with its flag, environment variable, config key, type, default and the sources it can come from. Every flag except `--stream`, `--help`, `--version` and `--config` can be set from the environment or a configuration file. [Configuration Layers](../advanced/configuration.md) explains how values are resolved, and the [Configuration guide](../guide/configuration.md) shows everyday use.

## Global settings

These apply to every command. The environment variable is `A2ACLI_` followed by the flag name in upper case with `-` replaced by `_`. In files, the key is the flag name or the environment variable name.

| Flag | Environment variable | Type | Default | Discovered `.env` | Notes |
| --- | --- | --- | --- | --- | --- |
| `-a`, `--agent-card` | `A2ACLI_AGENT_CARD` | host, URL or path | | yes | Card reference. See [reference normalization](../advanced/discovery.md#reference-normalization). Exclusive with `--endpoint`. |
| `--catalog` | `A2ACLI_CATALOG` | name or URL | | yes | AI Catalog to look the `--agent-card` identifier up in: a registered name, or a catalog URL. See [AI Catalogs](../guide/ai-catalogs.md). Experimental. |
| `-e`, `--endpoint` | `A2ACLI_ENDPOINT` | URL | | yes | Interface URL; skips the card. Needs exactly one `--transport`. |
| `--transport` | `A2ACLI_TRANSPORT` | list | | yes | `rest`, `jsonrpc`, `grpc` or a plugin name, most preferred first. Comma separated in the environment and `.env`. |
| `--a2a-version` | `A2ACLI_A2A_VERSION` | string | | yes | `1.0` or `0.3` registers only that version's transports. Unset registers both. |
| `--svc-param` | `A2ACLI_SVC_PARAM` | repeatable `key=value` or `key:value` | | yes | Request header or gRPC metadata. Split on the first `=` or `:`. In the environment and `.env`, one entry; use a YAML or JSON list for several. |
| `--auth` | `A2ACLI_AUTH` | string | | yes | Same as `--svc-param Authorization=<value>`. |
| `--tenant` | `A2ACLI_TENANT` | string | | yes | Tenant on every request. |
| `--timeout` | `A2ACLI_TIMEOUT` | duration | `30s` | yes | Whole request, or time between events when streaming. See [Streaming and Timeouts](../advanced/streaming.md). |
| `-o`, `--output` | `A2ACLI_OUTPUT` | `text`, `json`, `jsonl` | `text` | yes | See [Output](output.md). |
| `-v`, `--verbose` | `A2ACLI_VERBOSE` | bool | `false` | yes | Progress lines on stderr, prefixed `# `. |
| `--insecure` | `A2ACLI_INSECURE` | bool | `false` | yes | Plaintext gRPC. |
| `--ca-cert` | `A2ACLI_CA_CERT` | path | | no | Trust only the PEM certificates in the file, for HTTP and gRPC. Read when a command connects. |
| `--unix-socket` | `A2ACLI_UNIX_SOCKET` | path | | no | Send every HTTP request and gRPC connection through the socket. |
| `--credential-helper` | `A2ACLI_CREDENTIAL_HELPER` | list of helpers | | no | `[scope=]path` on the command line and in the environment; `{scope, command}` objects in YAML and JSON. See [below](#credential-helper-entries). |
| `--config` | never read | path | | | Use this file as the local file in place of the discovered `.env`. |
| | `A2ACLI_PLUGINS_ENABLED` | bool | `false` | yes | Config key `plugins-enabled`. Discover command plugins. Not read from the `--config` file. |

"Discovered `.env`" is the `.env` found by walking up from the working directory. A setting marked no is ignored there with a warning; it can still come from the flag, the environment, the user `config.yaml`, the global `.env` or a file passed with `--config`.

Booleans accept the values of Go's `strconv.ParseBool`: `1`, `t`, `T`, `true`, `TRUE`, `True`, `0`, `f`, `F`, `false`, `FALSE`, `False`. Durations use Go's syntax, such as `500ms`, `30s`, `5m`, `1h30m`; in YAML and JSON write them as strings.

## Command settings

The flags of each command bind the same way, through `A2ACLI_<NAME>` or the flag name in a file. A value applies to every command that has a flag of that name, and is ignored by commands that don't. An invalid value fails only the commands that have the flag.

| Flag | Environment variable | Commands |
| --- | --- | --- |
| `--async` | `A2ACLI_ASYNC` | `send` |
| `--context-id` | `A2ACLI_CONTEXT_ID` | `send`, `chat` |
| `--task-id` | `A2ACLI_TASK_ID` | `send`, `chat` |
| `--history` | `A2ACLI_HISTORY` | `send`, `task get`, `task list` |
| `--poll-interval` | `A2ACLI_POLL_INTERVAL` | `send`, `chat`, `task get` |
| `--save-fileparts` | `A2ACLI_SAVE_FILEPARTS` | `send`, `task get`, `task subscribe` |
| `--request-payload` | `A2ACLI_REQUEST_PAYLOAD` | `send` |
| `--text-part`, `--file-part`, `--data-part`, `--media-type` | `A2ACLI_TEXT_PART`, ... | `send` |
| `--metadata` | `A2ACLI_METADATA` | `send`, `task cancel` |
| `--wait` | `A2ACLI_WAIT` | `task get` |
| `--context`, `--status`, `--since`, `--with-artifacts` | `A2ACLI_CONTEXT`, ... | `task list` |
| `--limit`, `--page-token` | `A2ACLI_LIMIT`, `A2ACLI_PAGE_TOKEN` | `task list`, `task push-config list` |
| `--url`, `--id`, `--token`, `--auth-scheme`, `--auth-credentials` | `A2ACLI_URL`, ... | `task push-config create` |
| `--extended` | `A2ACLI_EXTENDED` | `card get` |
| `--reveal` | `A2ACLI_REVEAL` | `auth get` |
| `--no-descriptions` | `A2ACLI_NO_DESCRIPTIONS` | `completion` subcommands |

`--stream` is never read from configuration. The [command reference](commands/a2a.md) describes each flag.

> [!WARNING]
> Message flags bind too. `A2ACLI_FILE_PART` or `A2ACLI_TEXT_PART` adds a part to every message `send` builds, and `A2ACLI_TASK_ID` makes every `send` continue the same task. Settings with names as generic as `A2ACLI_URL` or `A2ACLI_TOKEN` apply to `task push-config create`.

`config show` lists the global settings only.

## Precedence

The first source with a value wins:

1. Command-line flag
2. Environment variable
3. Local file: the `--config` file, or else the nearest `.env` walking up from the working directory
4. `~/.config/a2a-cli/config.yaml`
5. `~/.config/a2a-cli/.env`
6. Flag default

## Files

| File | Format | Notes |
| --- | --- | --- |
| `.env` in the working directory or a parent | dotenv | Discovered; protected settings ignored. Not read when `--config` is set. |
| `--config <path>` | by extension | Replaces the discovered `.env`. Must exist. |
| `~/.config/a2a-cli/config.yaml` | YAML | User config. `a2a plugin set-enabled` writes it, mode `0600`. |
| `~/.config/a2a-cli/.env` | dotenv | Global defaults. |

`~` is the home directory from Go's `os.UserHomeDir`: `$HOME` on Unix, `%USERPROFILE%` on Windows. `XDG_CONFIG_HOME` is not used.

The format of a `--config` file comes from its extension:

| Extension | Format |
| --- | --- |
| `.json` | JSON object |
| `.yaml`, `.yml` | YAML mapping |
| `.env` | dotenv |
| anything else | JSON, then YAML, then dotenv; the first that parses as a mapping |

An empty file is valid in every format. Keys that match no flag are ignored.

### dotenv

dotenv files are parsed by the `github.com/joho/godotenv` library:

- One `KEY=value` per line. `KEY: value` also works.
- Keys contain only letters, digits, `_` and `.`. Any other character, such as `-` in `agent-card=`, is a parse error that fails every command, so use the environment variable names.
- A leading `export ` is ignored.
- Lines starting with `#` are comments, and so is ` #` and anything after it in an unquoted value.
- Single quotes keep the value as is. Double quotes allow `\n` escapes.
- `${KEY}` and `$KEY` in unquoted and double-quoted values expand to a key defined earlier in the same file. The process environment is not used; an unknown key expands to nothing.
- Every value is a string. For `--transport`, the string is split on commas.

```bash
# .env
A2ACLI_AGENT_CARD=https://agent.example.com
A2ACLI_TRANSPORT=jsonrpc,rest
A2ACLI_TIMEOUT=60s
export A2ACLI_TENANT="my team"
```

### YAML and JSON

Use the flag name or the environment variable name as the key, with native types. Lists set repeatable flags.

```yaml
agent-card: https://agent.example.com
transport: [jsonrpc, rest]
timeout: 60s
insecure: false
tenant: my-team
svc-param:
  - "Authorization=Bearer <token>"
  - "X-Client-ID=cli-app"
plugins-enabled: true
```

```json
{
  "agent-card": "https://agent.example.com",
  "transport": ["jsonrpc", "rest"],
  "timeout": "60s",
  "svc-param": ["X-Client-ID=cli-app"]
}
```

Numbers and booleans are converted to strings before they are parsed, so `timeout: 60` is the invalid duration `60`.

## Credential helper entries

In YAML and JSON, `credential-helper` is a list of objects:

| Key | Type | Required | Meaning |
| --- | --- | --- | --- |
| `command` | list of strings | yes | Program and arguments; `get` is appended when it runs |
| `scope` | string | no | `host` or `*.domain`, without a port. Omit for every host. |

Other keys are an error.

```yaml
credential-helper:
  - command: [a2a-serve, credential]
  - scope: "*.corp.example.com"
    command: [/usr/local/bin/corp-helper, --profile, work]
```

A single string, and the flag and environment variable, take `[scope=]path`: one program without arguments, with an optional scope before the first `=`. Each `--credential-helper` flag adds a helper; the environment variable holds one. See [Credential Helpers](../advanced/credential-helpers.md).
