# Configuration Layers

Every flag that the command line does not set can come from an environment variable or a configuration file. The `internal/clicfg` package keeps one store over five sources and binds it to the flags of the running command. This page explains how the store resolves a value and the exceptions to the general rule. [Configuration](../reference/configuration.md) in the reference lists every setting, and the [Configuration guide](../guide/configuration.md) shows everyday use.

## Sources and precedence

For each flag, the first source that has a value wins:

| Rank | Source | `config show` source | Notes |
| --- | --- | --- | --- |
| 1 | Command-line flag | `flag` | |
| 2 | Environment variable `A2ACLI_<NAME>` | `env` | |
| 3 | Local file | `local-file` | The file named by `--config`, or else the nearest `.env` found by walking up from the working directory |
| 4 | `~/.config/a2a-cli/config.yaml` | `user-file` | YAML |
| 5 | `~/.config/a2a-cli/.env` | `global-file` | dotenv |
| 6 | Flag default | `default` | |

`--config` replaces the walk-up search; with it, no `.env` is discovered. A missing `--config` file is a usage error. A missing user or global file is skipped.

The walk-up search starts in the working directory and stops at the first regular file named `.env`, up to the filesystem root. It finds the file of any project you `cd` into, including a repository you just cloned. Some settings are therefore not read from it; see [Protected settings](#protected-settings).

The environment variable name is `A2ACLI_` followed by the long flag name in upper case with `-` replaced by `_`: `--agent-card` is `A2ACLI_AGENT_CARD`, `--poll-interval` is `A2ACLI_POLL_INTERVAL`.

## Keys in files

In every file, a setting can use the flag name or the environment variable name. The flag name is checked first.

```yaml
agent-card: https://agent.example.com
A2ACLI_TIMEOUT: 60s
```

```bash
A2ACLI_AGENT_CARD=https://agent.example.com
```

A dotenv key may only contain letters, digits, `_` and `.`, so `.env` files must use the variable name. A key such as `agent-card=` in a `.env` makes the file fail to parse, and every command fails with exit code 1. Keys that match no flag are ignored, so a `.env` can hold other variables too.

## Which flags bind

`Bind` visits every flag of the running command: the global flags and the command's own. So `A2ACLI_POLL_INTERVAL=5s` applies to `send`, `chat` and `task get`, and `A2ACLI_HISTORY` applies to every command that has `--history`. The binding happens in the root `PersistentPreRunE`, after cobra has parsed the command line.

Four flags never come from configuration: `stream`, `help`, `version` and `config`. Every other flag can, including per-command flags such as `--async`, `--wait`, `--task-id` and `--file-part`. A value set for one command applies to every command with a flag of that name.

An invalid value is a usage error that names the flag and the source:

```text
Error: invalid value for --poll-interval from environment variable A2ACLI_POLL_INTERVAL: time: invalid duration "bogus"
```

The error appears only when the running command has that flag. Errors from several flags are reported together.

## List values

| Source | List syntax |
| --- | --- |
| Environment, `.env` | One string; for `--transport` it is split on commas and each item trimmed |
| YAML, JSON | A list |

The comma split applies to slice-typed flags only, which among the global flags is `--transport`. The others take the whole string as one value:

- `A2ACLI_SVC_PARAM=X-A=1,X-B=2` sets one parameter `X-A` with the value `1,X-B=2`. Use a YAML or JSON list for several.
- `A2ACLI_CREDENTIAL_HELPER` is one helper spec, commas included.
- A YAML list for a repeatable flag such as `svc-param`, `file-part` or `metadata` applies each item in order.

`credential-helper` in YAML and JSON takes a list of `{scope, command}` objects. See [Credential Helpers](credential-helpers.md).

## Protected settings

Flags annotated with `clicfg.AnnotationNotFromDiscoveredFile` are not read from a `.env` found by the walk-up search:

| Flag | Why |
| --- | --- |
| `--credential-helper` | It runs a command. |
| `--unix-socket` | It redirects every request. |
| `--ca-cert` | It chooses which certificates to trust. |

When the discovered `.env` sets one, `Bind` skips it, uses the next source, and the CLI warns on stderr:

```text
warning: ignoring ca-cert from /home/me/proj/.env; pass --config /home/me/proj/.env to use it
```

The same file passed with `--config` is trusted, because you named it.

Only these three are protected. A discovered `.env` can set every other flag. [Security](security.md) lists what that allows.

## `config show`

`config show` prints the binding of each flag of the `config show` command, which has the global flags only, with its value and source. The JSON form adds `envVar` and, for file sources, `path`.

```text
SETTING            VALUE                     SOURCE
a2a-version        (unset)                   default
agent-card         (unset)                   default
auth               <redacted>                default
ca-cert            (unset)                   default
catalog            (unset)                   default
credential-helper  a2a-serve credential      user-file
endpoint           (unset)                   default
insecure           false                     default
output             text                      default
svc-param          Authorization=<redacted>  local-file
tenant             t1                        local-file
timeout            30s                       default
transport          (unset)                   default
unix-socket        (unset)                   default
verbose            false                     default
```

`auth` is always shown as `<redacted>`, even when unset. For `svc-param` from a file or the environment, the value of each `Authorization` entry is replaced with `<redacted>`. `svc-param` set on the command line shows `(unset)`, since the flag value has no string form. `config show` never prints the per-command flags.

## `plugins-enabled`

`plugins-enabled` is not a flag. It decides whether command plugins are added to the command tree, so `newRootCmd` reads it before cobra parses the command line, with a store loaded without `--config`. It is true when the first source that has it holds a value that Go's `strconv.ParseBool` accepts as true (`1`, `t`, `true`, `TRUE` and so on). The sources are the environment variable `A2ACLI_PLUGINS_ENABLED`, a discovered `.env`, the user `config.yaml` and the global `.env`, in that order.

`plugins-enabled` in a file passed with `--config` has no effect. `a2a plugin set-enabled` writes the key to the user `config.yaml`. `config show` does not list it; `plugin list` prints the current state.

## Environment of transport plugins

A transport plugin's `serve` process gets an environment built by `Store.Environ` (`internal/clicfg/environ.go`):

1. The CLI's own environment, unchanged.
2. Every key from the local file, the `--config` file or the discovered `.env`, that the environment does not set.
3. Every key from the global `.env` that neither of the above sets.

Every key is passed, including keys that are not CLI settings, such as an upstream token for the plugin. List values from YAML or JSON are joined with commas. The user `config.yaml` is not passed. Command plugins and the `info` query of `transport list` get the CLI's own environment only.

`Environ` passes the discovered `.env` as well, protected keys included. See [Security](security.md).

## Where the code is

| File | Content |
| --- | --- |
| `internal/clicfg/loader.go` | Finding and parsing the files |
| `internal/clicfg/store.go` | Lookup in precedence order |
| `internal/clicfg/binder.go` | Binding to flags, list handling, protected settings, redaction |
| `internal/clicfg/environ.go` | Environment for transport plugins |
| `internal/clicfg/userconfig.go` | Writing the user `config.yaml` |
