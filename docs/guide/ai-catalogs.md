# AI Catalogs

An [AI Catalog](https://ai-catalog.io/) lists AI artifacts, A2A agents among them. `a2a` can register catalogs and resolve `--agent-card` from the agents they list, so you can refer to an agent by its catalog identifier in place of a card URL. Catalog support is experimental.

| Command | What it does |
|---|---|
| `a2a catalog add <name> <url\|path>` | Registers a catalog. A bare host or a URL without a path gets `/.well-known/ai-catalog.json`. |
| `a2a catalog list` | Lists registered catalogs. |
| `a2a catalog search [query]` | Lists the A2A agents whose identifier, name, description or tags contain the query. |
| `a2a catalog update [name]` | Fetches one or every registered catalog again. |
| `a2a catalog remove <name>` | Unregisters a catalog. |

```bash
a2a catalog add corp https://acme.com
a2a catalog search finance
a2a send -a urn:air:acme.com:agent:finance "What were last quarter's expenses?"
```

## Talk to an agent by its identifier

Pass the identifier to `--agent-card`, or to `card get`:

```bash
a2a send -a urn:air:acme.com:agent:finance "..."                                              # across all registered catalogs
a2a send --catalog corp -a urn:air:acme.com:agent:finance "..."                               # in a registered catalog
a2a send --catalog https://acme.com/ai-catalog.json -a urn:air:acme.com:agent:finance "..."   # in an unregistered catalog
a2a send -a https://acme.com/finance-catalog.json "..."                                       # a catalog with a single A2A agent
```

- A `urn:` value of `--agent-card` is an identifier, unless a file with that name exists. With `--catalog`, any value is an identifier.
- Lookups in registered catalogs read only the local store; run `a2a catalog update` to pick up changes.
- An identifier listed by more than one catalog is an error; pick one with `--catalog`.
- When a catalog lists several versions of an agent, the latest is used.
- A URL passed to `--agent-card` that serves a catalog in place of a card resolves only when the catalog lists exactly one A2A agent; otherwise the error lists the agents.

## Search

`search` lists only A2A agents, the entries of type `application/a2a-agent-card+json`. `--catalog` limits it to one catalog, which can be a registered name or the URL of a catalog you haven't registered:

```bash
a2a catalog search --catalog https://acme.com/ai-catalog.json
```

Catalogs nested in a catalog are followed up to four levels deep. Any catalog may reference `https` URLs, and only a local catalog may reference local files.

## Where catalogs are kept

Registered catalogs live in the store shared with [ai-catalog-cli](https://github.com/Agent-Card/ai-catalog-cli): `~/.ai-catalog`, or the directory in `AI_CATALOG_CACHE_DIR`. A catalog registered with either tool is visible to both. The store layout is not part of the AI Catalog specification, so `a2a` warns about a store whose format it doesn't recognize and refuses to change it.

The [catalog reference](../reference/commands/catalog.md) lists every flag.
