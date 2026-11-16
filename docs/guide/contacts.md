# Contacts

The contact book stores named agents you trust, so you can refer to an agent by a short name in place of a card URL or identifier.

| Command | What it does |
|---|---|
| `a2a contact add <name> <ref>` | Saves the agent `<ref>` resolves to, where `<ref>` is any `--agent-card` reference. |
| `a2a contact list` | Lists contacts. No requests are made. |
| `a2a contact get <name>` | Prints the contact and its card. `--refresh` fetches the card again. |
| `a2a contact remove <name>...` | Removes contacts. |

```bash
a2a contact add weather https://weather.example.com
a2a contact add fin urn:air:acme.com:agent:finance --note "Expense reports"
a2a contact add travel --catalog https://acme.com/ai-catalog.json urn:air:acme.com:agent:travel
a2a send -a fin "Where is report R-2041?"
```

## Talk to a contact

A contact name can be passed to `--agent-card` (`-a`) or `card get` in place of a card reference.

- Names use lowercase letters, digits, `-` and `_`, so they never look like a URL, path or identifier. A name can match a single-label host such as `agentbox`; the contact takes precedence, and `-a https://agentbox` reaches the host.
- A contact's identifier takes precedence over the same identifier in registered [AI Catalogs](ai-catalogs.md), so `-a <identifier>` resolves to the contact.
- If the contact book can't be read, `-a` prints a warning and resolves the reference as if there were no contacts.

## Adding contacts

- `contact add` fetches the card first and fails if it can't, using the same credentials, `--svc-param` headers and `--ca-cert` as `card get`.
- An agent from a catalog is copied with its identifier, publisher and trust manifest, so its signature can still be verified. A relative card URL is made absolute so the contact book resolves it in any tool; a trust manifest whose subject signs the relative URL no longer matches the copy. An agent added by its card gets the identifier `urn:ai-catalog:a2a-cli:<name>`.
- Two contacts can't share an identifier, even with different versions. Pass `--replace` to overwrite a contact of the same name.
- `--note` adds a note that your coding agent sees with the contact.

Fetched cards are cached under the user cache directory (`~/.cache/a2a-cli` on Linux) for 24 hours. An older card is fetched again by `contact get` and `-a <contact>`.

## Where contacts are kept

The contact book is an AI Catalog at `~/.config/a2a-cli/catalog.json` that only `a2a` writes. It is registered in the AI Catalog store shared with [ai-catalog-cli](https://github.com/Agent-Card/ai-catalog-cli) as `a2a-contacts`, so `ai-catalog search` and `a2a catalog search` see your contacts too. If the store can't be updated, the contact is still saved and `a2a` prints a warning.

The [Agent Plugin](coding-agents.md#contacts-and-agent-discovery) tells your coding agent about your contacts at the start of a session. The [contact reference](../reference/commands/contact.md) lists every flag.
