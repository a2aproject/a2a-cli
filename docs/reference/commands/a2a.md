# a2a

<!-- Generated from the command definitions; do not edit. -->

Discover, message, and manage A2A agents from your terminal.

Pick the agent with --agent-card (a host, a card URL or a card file) or
--endpoint with --transport. Every flag can also come from an A2ACLI_\*
environment variable or a config file; a2a config show prints where each
value came from.

## Commands

| Command | Description |
|---|---|
| [`auth`](auth.md) | Manage the credentials sent to agents |
| [`card`](card.md) | Work with agent cards |
| [`catalog`](catalog.md) | Work with AI Catalogs of agents (experimental) |
| [`chat`](chat.md) | Chat interactively with an agent |
| [`completion`](completion.md) | Generate the autocompletion script for the specified shell |
| [`config`](config.md) | Inspect configuration |
| [`contact`](contact.md) | Manage the contact book of trusted agents |
| [`plugin`](plugin.md) | Work with command plugins |
| [`send`](send.md) | Send a message to an agent |
| [`skill`](skill.md) | Print the a2a-cli agent skill |
| [`task`](task.md) | Work with tasks |
| [`transport`](transport.md) | Work with transport plugins |
| [`version`](version.md) | Print version information |

## Global flags

Every command takes these.

| Flag | Default | Description |
|---|---|---|
| `--a2a-version string` |  | Controls which a2a-protocol version client will advertise to the server. |
| `-a, --agent-card host\|url\|path` |  | Agent Card reference: host/origin, full card URL, local file path, an AI Catalog entry identifier, or a contact name |
| `--auth string` |  | Shorthand for --svc-param Authorization=&lt;creds&gt; |
| `--ca-cert string` |  | Trust only the CA certificates in this PEM file for TLS connections |
| `--catalog string` |  | AI Catalog to resolve --agent-card in: a registered catalog name, or a catalog URL (experimental) |
| `--config string` |  | Load configuration from an explicit file (.yaml, .json, or .env) in place of the local .env |
| `--credential-helper helper` |  | Credential helper returning the headers for an agent: [scope=]path (repeatable; see a2a auth get --help) |
| `-e, --endpoint string` |  | Agent interface URL for a direct connection; skips card resolution and requires a single --transport flag |
| `--insecure` |  | Use insecure (plaintext) gRPC transport credentials |
| `--oauth string` | `off` | OAuth for agent calls: off (default), auto (sign in when an agent requires it), or cached (use stored grants only); see a2a auth login --help |
| `--oauth-clients json` |  | OAuth client registration as JSON: {"authority":"&lt;issuer&gt;","client_id":"&lt;id or CIMD URL&gt;","scopes":[...],"omit_resource":false} (repeatable; a JSON array is also accepted) |
| `--oauth-default-client-id string` | `https://a2a-protocol.org/oauth/a2a-cli.json` | OAuth client ID used with authorization servers that support Client ID Metadata Documents when no oauth-clients entry matches; empty disables it |
| `--oauth-store string` | `file` | Where OAuth grants are stored: file (~/.config/a2a-cli/oauth/grants.json) or keychain (tokens in the OS keychain) |
| `-o, --output string` | `text` | Output format: text, json (indented), or jsonl (one compact JSON object per line) |
| `--svc-param key=value` |  | Service parameter key=value (repeatable) |
| `--tenant string` |  | Tenant identifier |
| `--timeout duration` | `30s` | Request timeout |
| `--transport stringArray` |  | Transport preference: rest, jsonrpc, grpc, or an installed plugin name (repeatable, highest preference first) |
| `--unix-socket string` |  | Send every HTTP request and gRPC connection through the Unix socket at this path |
| `-v, --verbose` |  | Verbose output to stderr |
