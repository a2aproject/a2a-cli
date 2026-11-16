# a2a catalog

<!-- Generated from the command definitions; do not edit. -->

Register AI Catalogs and find the A2A agents they list.

Catalogs are kept in the store shared with ai-catalog-cli (~/.ai-catalog, or $AI_CATALOG_CACHE_DIR), so a catalog registered with either tool is visible to both. Pass an agent's identifier to --agent-card to talk to it.

These commands are experimental: the store layout is not part of the AI Catalog specification.

```
a2a catalog <command>
```

Aliases: `catalogs`

The [global flags](a2a.md#global-flags) apply too.

## a2a catalog add

Fetch the AI Catalog at a URL or local path, and the catalogs nested in it, and register it under a name. A URL without a path, or a bare host, gets the well-known path /.well-known/ai-catalog.json.

```
a2a catalog add <name> <url|path>
```

## a2a catalog list

List registered AI Catalogs

```
a2a catalog list
```

## a2a catalog remove

Unregister an AI Catalog

```
a2a catalog remove <name|source>
```

Aliases: `rm`

## a2a catalog search

List the A2A agents (entries of type application/a2a-agent-card+json) whose identifier, name, description or tags contain the query, ignoring case. Without a query every agent is listed.

Searches the registered catalogs, or only the one given with --catalog, which may also be the URL of a catalog that is not registered.

```
a2a catalog search [query]
```

**Examples**

```bash
a2a catalog search finance
a2a catalog search --catalog corp
a2a send -a <identifier> "hello"
```

## a2a catalog update

Fetch a registered AI Catalog from its source again, or every registered catalog when no name is given.

```
a2a catalog update [name]
```
