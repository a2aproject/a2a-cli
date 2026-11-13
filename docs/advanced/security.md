# Security

The `a2a` CLI runs on your machine with your permissions, sends your credentials to agents, and can run other programs. This page lists its trust boundaries: what it protects, how, and what it leaves to you. The [Configuration Layers](configuration.md), [Credential Helpers](credential-helpers.md) and plugin pages explain the mechanisms in more detail.

## Cloned repositories

The CLI reads the nearest `.env` found by walking up from the working directory, so running `a2a` inside a repository you cloned applies the repository's `.env`. Three settings are not read from that file:

| Setting | What a hostile `.env` could do with it |
| --- | --- |
| `credential-helper` | Run any program, the next time you call an agent |
| `unix-socket` | Send every request, with your credentials, to a socket it chose |
| `ca-cert` | Make the CLI trust a certificate it chose, and accept a server impersonating the agent |

The CLI ignores them in a discovered `.env` and prints a warning. The same file passed with `--config` is trusted, because you named it.

All other settings are read from a discovered `.env`. In particular it can:

- Set `agent-card` or `endpoint`, so an `a2a send` without `-a` goes to an agent the file chose, with the headers of any unscoped credential helper you configured.
- Set `svc-param` or `auth`, which adds headers to your requests.
- Set `file-part`, which attaches a local file of its choice to every `send`.
- Set `insecure`, which turns off TLS for gRPC.
- Set `transport`, which can select a transport plugin you installed.
- Set `plugins-enabled`, which turns on command plugins you installed.

Run `a2a config show` in an unfamiliar directory to see what applies; the source column names the file. A discovered `.env` cannot install a plugin or run a program that is not already on your `PATH`.

## Plugins

Command plugins (`a2a-<name>`) and transport plugins (`a2a-transport-<name>`) are programs on your `PATH`. The CLI does not sandbox them. They run as you, see your files, and inherit your environment.

| | Command plugins | Transport plugins |
| --- | --- | --- |
| When it runs | You run `a2a <name>`; `info` runs for root help and `plugin list` | A card lists the binding and the client selects it, or `--endpoint` with `--transport <name>`; `info` runs for `transport list` |
| Opt-in | `plugins-enabled` must be true | None; any card or `--transport` value can name it |
| Environment | The CLI's process environment | The CLI's environment plus every key from the local and global `.env` files and the `--config` file |

The second consequence is worth stating plainly: a transport plugin receives every value in your `.env` files, secrets included, and so does the plugin of any binding a card lists. A program named `a2a-transport-jsonrpc`, `a2a-transport-grpc` or `a2a-transport-http+json` on `PATH` replaces the built-in transport for that binding. Treat a plugin like any program you give your credentials to.

The loopback connection between the CLI and a transport plugin is protected against other local processes: a per-launch token on every call, checked in constant time, and TLS with a certificate created per launch and pinned by the CLI. The devkit does both; a plugin written without it may serve plaintext.

## Credentials on the wire

- `--auth`, `--svc-param` and credential helper headers go to the card request and every A2A call of the command. The card request goes to the `--agent-card` host. A2A calls go to the interface URL in the card, which can be on another host.
- Credential helpers are chosen by the host of each URL, so a scoped helper's token goes only to hosts in its scope. An unscoped helper runs for every host.
- Agents on loopback addresses default to `http://`, and the card's interface URLs can be `http://`. The CLI sends credentials over plain HTTP when the URL says so, without a warning.
- `--insecure` disables TLS for gRPC. When a credential is set, from `--auth`, `--svc-param Authorization=...` or a helper's `Authorization` header, the CLI warns once:

  ```text
  warning: --insecure disables TLS for gRPC; a credential sent over gRPC may be exposed.
  ```

- `--ca-cert` replaces the system trust store with the certificates in the file, for the card request and for the REST, JSON-RPC and gRPC transports.
- `--unix-socket` sends every HTTP request and gRPC connection to the socket, whatever the URL's host. TLS still applies to `https` URLs, with the URL's host as the server name.

## What the CLI hides

- **Errors.** When an error contains a URL from a failed HTTP request, `clierr.RedactURL` masks the password in `user:password@` as `***` and drops the query string. Card reference validation errors are redacted the same way, and so are credential helper failures. Other URLs in error messages, such as one inside an agent's error text, are printed as received.
- **`config show`.** `auth` is always `<redacted>`. In `svc-param` values from files and the environment, each `Authorization` value is `<redacted>`. Other service parameters, such as an `X-Api-Key`, are printed.
- **`auth get`.** Header values are `<redacted>` unless you pass `--reveal`.
- **Push config text output.** `--auth-credentials` is shown as `<redacted>` in text output; JSON output includes it.
- **`-v`.** Verbose logs print helper names and URLs, not header values. The URLs are printed as given, without redaction, so a password in a URL appears in the log.

## Files the CLI writes

- `a2a plugin set-enabled` writes `~/.config/a2a-cli/config.yaml` with mode `0600` and creates the directory with mode `0755`. It rewrites the whole file, so comments in it are lost.
- `--save-fileparts <dir>` writes the raw file parts of agent responses. `internal/fileparts/saver.go` reduces each file name the agent sends to its base name, so `../../.bashrc` is written as `.bashrc` inside the directory. Names `.`, `..` and `/` are replaced with a name derived from the artifact. Files are created with mode `0644`. Within one command, a name used by another artifact gets a ` (n)` suffix; a file left in the directory from an earlier run is overwritten.

## Not protected

- The content of agent replies. Text output prints agent text as is, including terminal control characters.
- Which interface an agent's card points to. A card can list any URL; the CLI connects to it and sends your credentials there.
- Card signatures. The CLI does not verify them.
- Plugins and credential helpers. They are trusted programs.
- Secrets in configuration files. The CLI reads `.env`, YAML and JSON files with whatever permissions they have; keep files with credentials private.
