# Agent Cards

Every A2A agent publishes an agent card: a JSON document with its name, description, skills, and the interfaces it accepts. `a2a` reads the card to decide how to connect.

| Command | What it does |
|---|---|
| `a2a card get <ref>` | Prints the card. |
| `a2a card get <ref> -o json` | Prints the raw card. |
| `a2a card get <ref> --extended` | Asks the agent for the card it shows authenticated callers. |
| `a2a <command> -a <ref>` | Uses the card to pick the agent's interface for any command. |

## Card references

`--agent-card` (`-a`), and the argument of `card get`, take three forms:

| You pass | `a2a` reads |
|---|---|
| `agent.example.com` | `https://agent.example.com/.well-known/agent-card.json` |
| `localhost:8080` or `127.0.0.1:8080` | `http://localhost:8080/.well-known/agent-card.json` |
| `https://agent.example.com/cards/a.json` | That URL, since it has a path |
| `./card.json` or any existing file | The file |
| `urn:air:acme.com:agent:finance` | The card of that agent in a registered [AI Catalog](ai-catalogs.md) |

```bash
a2a card get localhost:8080
```

```text
Name:         demo
Description:  demo is an acp coding agent reached through `a2a-serve`. ...
Version:      1.0.0
Interfaces:
  JSONRPC (A2A v1.0)     http://127.0.0.1:43229
  HTTP+JSON (A2A v1.0)   http://127.0.0.1:43229
Streaming:    true
Skills:
  coding               Coding agent
```

`Interfaces` lists the URLs and protocol bindings the agent accepts. `Streaming` says whether it can stream replies.

## Save a card

```bash
a2a card get localhost:8080 -o json > agent-card.json
a2a send -a ./agent-card.json "hello"
```

A saved card skips the network round trip, and pins the interfaces you tested against.

## Skip the card

When you know the interface URL and its protocol, connect to it directly with `--endpoint` (`-e`) and exactly one `--transport`:

```bash
a2a send -e http://127.0.0.1:43229 --transport jsonrpc "hello"
```

[Transports](transports.md) covers the protocol choice, and [Discovery](../advanced/discovery.md) how `a2a` picks an interface from the card.
