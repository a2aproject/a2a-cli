# a2a auth

<!-- Generated from the command definitions; do not edit. -->

Manage the credentials sent to agents

```
a2a auth <command>
```

The [global flags](a2a.md#global-flags) apply too.

## a2a auth get

Print the credentials sent to an agent: the headers the credential helper
returns for it, and the OAuth grants stored for it.

A credential helper follows the Bazel credential helper protocol:
https://github.com/EngFlow/credential-helper-spec. --credential-helper takes
"[scope=]path", where scope is a host such as example.com, or \*.example.com
for a domain and its subdomains. Config files list {scope, command} entries
under credential-helper. The most specific helper for the agent's host runs.

OAuth grants are obtained with a2a auth login or when an agent requires
them; see a2a auth login --help. The agent's grants are the one for exactly
the URL you pass, those for the interface URLs in its card, and, when the URL
has a path, those below it; when the card can't be read, only the one for
exactly the URL. Other grants that may apply, such as one for the whole host
that other agents may share, are listed apart. --reveal prints header values
and access tokens.

```
a2a auth get <agent-url> [flags]
```

**Examples**

```bash
a2a auth get https://agent.example.com
a2a auth get https://agent.example.com --reveal
```

**Flags**

| Flag | Default | Description |
|---|---|---|
| `--reveal` |  | Print header values and access tokens instead of redacting them |

## a2a auth list

List stored OAuth grants

```
a2a auth list
```

## a2a auth login

Sign in to an agent and store the OAuth grant, without sending a message.

a2a learns where to sign in from the agent card's oauth2 or openIdConnect
security scheme, or from the Bearer challenge of a 401 response and the
agent's protected resource metadata. It prints a URL to open and waits for the
redirect to 127.0.0.1; with --device it prints a URL and a code to enter on any
device. The grant is stored in ~/.config/a2a-cli/oauth/grants.json (or the OS
keychain with --oauth-store keychain) and refreshed when it expires.

Other commands leave OAuth off unless you turn it on (--oauth off, the
default), so agents that take other credentials work as before. With --oauth
auto, or oauth: auto in ~/.config/a2a-cli/config.yaml, they use stored grants
and sign in the same way when an agent requires it and a terminal is attached.
--oauth cached only uses stored grants, so scripts and sandboxed agents can run
after an a2a auth login. No OAuth runs when --auth, --svc-param or a credential
helper supplies the Authorization header. Other credentials you pass (any
--svc-param, a credential helper for the agent's host, --unix-socket) are sent
first: OAuth then signs in only when the agent answers 401. a2a server --proxy
never uses OAuth for the upstream agent.

--oauth-clients configures one client per authorization server: on the command
line or in A2ACLI_OAUTH_CLIENTS as JSON, {"authority":"&lt;issuer&gt;","client_id":"&lt;id
or CIMD URL&gt;","scopes":[...]}, and in config files as a list of such objects.
"omit_resource":true leaves the RFC 8707 resource parameter out, for servers
that reject it.
When none matches a server that supports Client ID Metadata Documents, a2a uses
https://a2a-protocol.org/oauth/a2a-cli.json.

```
a2a auth login [agent-ref] [flags]
```

**Examples**

```bash
a2a auth login https://agent.example.com
a2a auth login https://agent.example.com --device
a2a auth login https://agent.example.com --scope a2a.invoke
a2a auth login -e https://agent.example.com/a2a --transport jsonrpc
```

**Flags**

| Flag | Default | Description |
|---|---|---|
| `--device` |  | Use the device authorization flow: print a URL and a code to enter on any device |
| `--scope stringArray` |  | Scope to request instead of the ones the agent names (repeatable) |

## a2a auth logout

Revoke and delete stored OAuth grants for an agent, or all of them.

The agent's grants are the one for exactly the URL you pass, those for the
interface URLs in its agent card, and, when the URL has a path, those below
it. When the card can't be read, only the grant for exactly the URL is
removed. Other grants that may apply, such as one for the whole host that
other agents may share, are kept; a note names each with the command that
removes it, and --all removes every grant.

```
a2a auth logout [agent-url] [flags]
```

**Examples**

```bash
a2a auth logout https://agent.example.com
a2a auth logout --all
```

**Flags**

| Flag | Default | Description |
|---|---|---|
| `--all` |  | Remove every stored grant |
