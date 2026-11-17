# Examples

Ready-made recipes, each recorded from start to end. Copy them and adapt them. The [lessons](../lessons/README.md) explain the basics they build on.

| Example | Shows |
|---|---|
| [Scripting with jq](scripting-with-jq/README.md) | Fan work out to an agent with `--async`, collect results with `task get --wait`, read them with `jq`. |
| [A credential helper](credential-helper/README.md) | A 20-line helper that gives each agent its own token, and how to scope it. |
| [A command plugin](a2a-hello/README.md) | Add `a2a hello` with a small Go program. |
| [A transport plugin](a2a-transport-echo/README.md) | Add `--transport echo`: a custom transport behind a loopback A2A server. |
| [AI Catalog interop](ai-catalog-interop/README.md) | A contact found by `ai-catalog search`, and a catalog registered with `ai-catalog` used by `a2a`. |
| [OpenCode calls a contact](opencode-contacts/README.md) | opencode learns your contacts from the a2a-cli plugin and delegates to one. |
| [Agent discovery](agent-discovery/README.md) | opencode finds the agent of a site it reads, asks first, then delegates to it. |
