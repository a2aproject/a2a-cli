# a2a card

<!-- Generated from the command definitions; do not edit. -->

Work with agent cards

```
a2a card <command>
```

The [global flags](a2a.md#global-flags) apply too.

## a2a card get

Fetch an agent card and print it.

The reference is a host (the CLI fetches /.well-known/agent-card.json from
it, over http for loopback hosts and https otherwise), a full card URL, or
a local file. --extended asks the agent for the card it shows to
authenticated callers.

```
a2a card get [ref] [flags]
```

**Examples**

```bash
a2a card get https://agent.example.com
a2a card get localhost:8080 -o json
a2a card get ./agent-card.json
a2a card get https://agent.example.com --extended --auth "Bearer $TOKEN"
```

**Flags**

| Flag | Default | Description |
|---|---|---|
| `--extended` |  | Fetch the extended agent card |
