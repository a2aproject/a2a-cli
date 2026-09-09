# Lesson 3 — configure the CLI

Last updated: 2026.09.08

In lesson 2 you set the agent card in a `.env` file. That is one of three ways to give the `a2a` CLI a setting. This lesson covers all three. It also shows how `a2a config show` reveals which one won, and lists every setting you can configure.

## What you'll learn

- The three ways to pass a setting: a CLI flag, a session environment variable, and a `.env` file
- Which one to reach for, and why flags are best for agentic tools
- How `a2a config show` reports the effective value and where it came from
- Every setting you can configure, with its environment variable and default

## Prerequisites

The `a2a` CLI installed (see the [repo README](../../README.md)). For the `send` step, start a throwaway echo server in **terminal A**:

```bash
a2a server --echo --port 8090
```

An echo server sends your message straight back, so you can see requests land. Do everything below in **terminal B**.

## The three ways to set a value

### (a) Pass it on the command line

Set the value right where you run the command:

```bash
a2a card get -a http://localhost:8090
a2a send -a http://localhost:8090 -o json "hello"
```

**Recommended for agentic tools and scripts.** The command carries its own settings, so anyone reading it sees exactly which agent the request goes to. Nothing is hidden in the environment or a file.

### (b) Set a session environment variable

Export a setting once and every command in that shell session picks it up. It lasts until you close the session:

```bash
export A2ACLI_AGENT_CARD=http://localhost:8090

a2a card get       # no -a needed
a2a send "hello"   # runs as a task; the reply is in the artifacts
```

Good for a focused session against one agent, without editing any file.

### (c) Put it in a `.env` file

For a setting you want every time you work in a folder, write it to `.env`:

```bash
echo "A2ACLI_AGENT_CARD=http://localhost:8090" > .env

a2a card get       # reads .env from the working directory
```

The CLI reads `.env` from the working directory automatically. Point at a different file with `--config ./other.env`.

## See what won: `a2a config show`

Settings can come from several places at once. `config show` prints the effective value and the source it resolved from:

```bash
a2a config show
```

```text
SETTING      VALUE                    SOURCE
agent-card   http://localhost:8090    env
output       text                     default
timeout      30s                      default
...
```

Add `-o json` for a machine-readable version. Credential settings such as `auth` are shown as `<redacted>`.

## Precedence

When the same setting is given in more than one place, the CLI uses the first match in this order:

1. a command-line flag
2. a session environment variable
3. the local `.env` (the file named by `--config`, or the nearest `.env` above the working directory)
4. the global `.env` at `~/.config/a2a-cli/.env`
5. the built-in default

## All settings you can configure

Set any of these as a flag, an environment variable, or a `.env` entry. The table lists each one with its variable name and default. The variable name is always the flag name in capitals, with a `A2ACLI_` prefix.

| Setting | Short | Environment variable | Default | Purpose |
|---|---|---|---|---|
| `--agent-card` | `-a` | `A2ACLI_AGENT_CARD` | (unset) | Agent Card reference: host, card URL, or file path |
| `--endpoint` | `-e` | `A2ACLI_ENDPOINT` | (unset) | Direct interface URL; skips card resolution |
| `--transport` | | `A2ACLI_TRANSPORT` | (card order) | Transport preference: `rest`, `jsonrpc`, `grpc` |
| `--a2a-version` | | `A2ACLI_A2A_VERSION` | (unset) | A2A protocol version to advertise to the server |
| `--output` | `-o` | `A2ACLI_OUTPUT` | `text` | Output format: `text`, `json`, or `jsonl` |
| `--svc-param` | | `A2ACLI_SVC_PARAM` | (unset) | Service parameter, `key=value` |
| `--auth` | | `A2ACLI_AUTH` | (unset) | Authorization credentials (redacted in `config show`) |
| `--tenant` | | `A2ACLI_TENANT` | (unset) | Tenant identifier, sent on every request |
| `--timeout` | | `A2ACLI_TIMEOUT` | `30s` | Request timeout |
| `--verbose` | `-v` | `A2ACLI_VERBOSE` | `false` | Verbose output to stderr |
| `--insecure` | | `A2ACLI_INSECURE` | `false` | Use plaintext gRPC credentials |

`--stream`, `--config`, `--help`, and `--version` must be passed as flags; they are never read from the environment or a `.env` file.

> Store secrets such as `--auth` only in files you keep private.

## Run the whole lesson

`run.sh` starts an echo server, then walks through the three methods and `config show`:

```bash
bash run.sh
```

## Next

Lesson 4 goes deeper into [messages and tasks](../04-messages-and-tasks/). You will build multi-part messages, stream replies, and send async.

## Learn more

Read the [a2a-cli specification](../../specification/SPEC.md) for the full set of commands and flags.
