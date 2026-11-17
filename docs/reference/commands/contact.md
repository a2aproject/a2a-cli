# a2a contact

<!-- Generated from the command definitions; do not edit. -->

Manage the contact book: named A2A agents you trust.

The contact book is an AI Catalog at ~/.config/a2a-cli/catalog.json that only a2a writes. It is registered in the AI Catalog store shared with ai-catalog-cli as "a2a-contacts", so other tools see your contacts too.

A contact name can be passed to --agent-card (-a) in place of a card reference, and a contact's identifier takes precedence over the same identifier in other catalogs. The a2a agent plugin hooks tell your coding agent about your contacts at the start of a session.

```
a2a contact <command>
```

Aliases: `contacts`

The [global flags](a2a.md#global-flags) apply too.

## a2a contact add

Resolve &lt;ref&gt; like --agent-card and save the agent under &lt;name&gt;. &lt;ref&gt; may be a host, a card URL, a local file, the identifier of an agent in a registered AI Catalog, or, with --catalog, the identifier of an agent in that catalog.

An agent from a catalog is copied with its identifier, publisher and trust manifest. An agent added by its card gets the identifier urn:ai-catalog:a2a-cli:&lt;name&gt;. The card is fetched first, so an unreachable agent is not saved.

Names use lowercase letters, digits, '-' and '_'.

```
a2a contact add <name> <ref> [flags]
```

**Examples**

```bash
a2a contact add weather https://weather.example.com
a2a contact add fin urn:air:acme.com:agent:finance --note "Expense reports"
a2a contact add fin --catalog https://acme.com/ai-catalog.json urn:air:acme.com:agent:finance
```

**Flags**

| Flag | Default | Description |
|---|---|---|
| `--note string` |  | Note about the agent, shown to your coding agent with the contact |
| `--replace` |  | Overwrite an existing contact with the same name |

## a2a contact get

Show a contact and its agent card. The card is read from the local cache, and fetched when it isn't cached or --refresh is set.

```
a2a contact get <name> [flags]
```

**Flags**

| Flag | Default | Description |
|---|---|---|
| `--refresh` |  | Fetch the agent card even when it is cached |

## a2a contact list

List the contact book. No requests are made.

```
a2a contact list
```

Aliases: `ls`

## a2a contact remove

Remove agents from the contact book

```
a2a contact remove <name>...
```

Aliases: `rm`
