# AI Catalog Interop

`a2a` shares its catalog store with [ai-catalog-cli](https://github.com/Agent-Card/ai-catalog-cli), the reference CLI for [AI Catalogs](https://ai-catalog.io/). This example goes both ways: a contact saved with `a2a` shows up in `ai-catalog search`, and a catalog registered with `ai-catalog` is searchable and callable from `a2a`.

![Adding a contact found by ai-catalog, then registering a catalog with ai-catalog and calling its agent from a2a](demo.gif)

## Prerequisites

- `a2a`.
- `ai-catalog`, from its [releases](https://github.com/Agent-Card/ai-catalog-cli/releases) or `brew install ai-catalog` after `brew tap Agent-Card/ai-catalog-cli https://github.com/Agent-Card/ai-catalog-cli`.
- No model or API key. [`start-agents.sh`](start-agents.sh) runs three small local agents with `a2a server --exec`, on ports 9101 to 9103.

Run the commands in this directory. Start the agents in a second terminal:

```bash
./start-agents.sh
```

## A contact, seen by ai-catalog

Save the weather agent as a contact:

```bash
a2a contact add weather localhost:9101 --note "Forecasts for trip planning"
```

```text
Added contact weather: Weather (urn:ai-catalog:a2a-cli:weather)
```

The contact book is a catalog of its own, registered in the shared store as `a2a-contacts`, so `ai-catalog` lists and searches it:

```bash
ai-catalog catalog list
ai-catalog search weather
```

```text
NAME          URL                                              ENTRIES  LAST UPDATED          HASH
--------------------------------------------------------------------------------------------------------
a2a-contacts  file:///home/you/.config/a2a-cli/catalog.json        1  2026-11-16T09:58:03   da99b5df
```

```text
IDENTIFIER                      NAME     DESCRIPTION                   TYPE                             CATALOG
------------------------------------------------------------------------------------------------------------------------------
urn:ai-catalog:a2a-cli:weather  Weather  Current weather for any city  application/a2a-agent-card+json  file:///home/you/.config/a2…
```

## A catalog, used by a2a

[`acme-catalog.json`](acme-catalog.json) lists two of Acme's agents. Register it with `ai-catalog`:

```bash
ai-catalog catalog add acme ./acme-catalog.json
```

```text
Fetching catalog from ./acme-catalog.json...
✓ "acme" added (2 entries)
```

`a2a` reads the same store, so it sees the catalog and finds its agents:

```bash
a2a catalog list
a2a catalog search travel
```

```text
NAME          SOURCE                                                   ENTRIES  LAST UPDATED
a2a-contacts  file:///home/you/.config/a2a-cli/catalog.json            1        2026-11-16 09:58:03
acme          file:///home/you/ai-catalog-interop/acme-catalog.json    2        2026-11-16 09:58:03
```

```text
IDENTIFIER                           NAME         VERSION  CATALOG  DESCRIPTION
urn:air:acme.example:agent:travel    Travel Desk  -        acme     Books flights and hotels within Acme's travel policy
urn:air:acme.example:agent:expenses  Expenses     -        acme     Files expense reports and answers questions about reimburse…
```

Talk to an agent by its identifier:

```bash
a2a send -a urn:air:acme.example:agent:travel "Lisbon, 3 nights from Dec 2"
```

```text
Status:   completed (2026-11-16T09:57:38Z)
Artifacts:
  [01a1e3ca-7391-71d0-8a3d-dc63f6766e82] Booked: Lisbon, 3 nights from Dec 2. Confirmation TD-4821.
```

Save it as a contact to give it a short name. The contact keeps the catalog identifier:

```bash
a2a contact add travel urn:air:acme.example:agent:travel
a2a send -a travel "Porto, 1 night from Dec 5"
a2a contact list
```

```text
NAME     AGENT        IDENTIFIER                         NOTE
travel   Travel Desk  urn:air:acme.example:agent:travel  -
weather  Weather      urn:ai-catalog:a2a-cli:weather     Forecasts for trip planning
```

## How it works

Both tools keep registered catalogs in `~/.ai-catalog`, or the directory in `AI_CATALOG_CACHE_DIR`. `a2a catalog add` and `ai-catalog catalog add` write the same entries, so each tool sees the other's catalogs. The contact book at `~/.config/a2a-cli/catalog.json` is an ordinary AI Catalog that only `a2a` writes; `a2a contact add` registers it in the store the first time.

[AI Catalogs](../../docs/guide/ai-catalogs.md) and [Contacts](../../docs/guide/contacts.md) describe both features.

## Clean up

```bash
a2a contact remove weather travel
ai-catalog catalog remove acme
```
