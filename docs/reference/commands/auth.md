# a2a auth

<!-- Generated from the command definitions; do not edit. -->

Manage the credentials sent to agents

```
a2a auth <command>
```

The [global flags](a2a.md#global-flags) apply too.

## a2a auth get

Print the headers the credential helper returns for an agent.

A credential helper follows the Bazel credential helper protocol:
https://github.com/EngFlow/credential-helper-spec. --credential-helper takes
"[scope=]path", where scope is a host such as example.com, or \*.example.com
for a domain and its subdomains. Config files list {scope, command} entries
under credential-helper. The most specific helper for the agent's host runs.

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
| `--reveal` |  | Print header values instead of redacting them |
