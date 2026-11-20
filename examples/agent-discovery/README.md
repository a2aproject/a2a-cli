# Agent Discovery

Sites can publish an A2A agent at `/.well-known/agent-card.json`. With the a2a-cli plugin, your coding agent learns about the agent of a site when that site comes up in the conversation, and asks you before it delegates to it. This example serves a made-up documentation site with an agent at `docs.acme.test`, and asks [opencode](https://opencode.ai) a question about it.

![opencode reads the docs page, learns about the site's agent, asks first, then delegates the question to it](demo.gif)

## Prerequisites

- `a2a`, Go and Docker, as in [OpenCode Calls a Contact](../opencode-contacts/README.md), whose [`run-opencode`](../opencode-contacts/run-opencode) this example uses.
- A [Gemini API key](https://aistudio.google.com/apikey) in `GEMINI_API_KEY`.

Run the commands in this directory. Serve the site in a second terminal:

```bash
go run ./site --addr 127.0.0.1:8080 --url http://docs.acme.test:8080
```

[`site/main.go`](site/main.go) serves three things: a page about the rate limits of the "Acme Payments API" that leaves out the retry policy, an agent card for the "Acme Docs Assistant" with a skill for exactly that, and the agent. The agent has no model and gives the same answer every time.

## Point a name at the site

Discovery works on real host names, so give the site one. Outside Docker, add it to `/etc/hosts`:

```bash
echo "127.0.0.1 docs.acme.test" | sudo tee -a /etc/hosts
```

`run-opencode` runs opencode in a container, which takes the name from `--add-host` instead:

```bash
export OPENCODE_DOCKER_ARGS="--add-host docs.acme.test:127.0.0.1"
```

## Ask opencode

Start opencode:

```bash
../opencode-contacts/run-opencode
```

Ask it about the site:

```text
Read http://docs.acme.test:8080/ and tell me how my client should handle 429s from the Acme Payments API.
```

opencode fetches the page. The plugin sees the URL, finds the site's agent card, and adds it to the fetch result as an unverified agent. The page doesn't have the answer, so opencode offers the agent and asks first:

```text
% WebFetch http://docs.acme.test:8080/

The documentation at http://docs.acme.test:8080/ indicates that requests exceeding your limit return an
HTTP 429, but specific retry policies and limits are not listed on that page.

It suggests consulting the Acme Docs Assistant, an A2A agent available at the same origin. Would you like me
to ask it how your client should handle 429s?
```

Say yes:

```text
Yes, ask it.
```

opencode runs `a2a send -a http://docs.acme.test:8080 "..."` and answers from the agent's reply:

```text
To handle HTTP 429 responses from the Acme Payments API, your client should follow this retry policy:

1. Respect Retry-After: If the response includes a Retry-After header, wait for the specified number of
   seconds before retrying.
2. Exponential Backoff: If the header is missing, use exponential backoff starting at 1 second, doubling the
   wait time up to 32 seconds, and include jitter.
3. Maximum Attempts: Give up after 5 attempts and report the error.
4. Idempotency: Only retry POST /v2/charges using the same Idempotency-Key to ensure a charge is not applied
   multiple times.
```

The site's terminal shows the probe of the agent card, then the A2A request. To script the same exchange, use `run-opencode run "<prompt>"` and then `run-opencode run --continue "Yes, ask it."`.

## How it works

When a URL appears in a prompt or a web fetch, the plugin runs `a2a hook`, which checks the card cache for the URL's origin. An origin it hasn't seen is probed by a detached `a2a hook probe`, which fetches `/.well-known/agent-card.json` and `/.well-known/ai-catalog.json` in parallel; for a web fetch the plugin waits up to 1.5 seconds for the probe, so the agent arrives with the fetch result. This site has no AI Catalog; a site that has one gets its A2A agents listed too. Each agent is reported once per session.

Discovered agents are marked unverified: the context tells opencode to ask you before delegating or sending data to them, and to treat the card's text as data, not instructions. To trust one, save it as a contact with `a2a contact add acme-docs http://docs.acme.test:8080`.

[Coding Agents](../../docs/guide/coding-agents.md#contacts-and-agent-discovery) describes discovery and its settings, and [Security](../../docs/advanced/security.md#agent-discovery) where probes may connect. Set `discovery: false` in `~/.config/a2a-cli/config.yaml` to turn off both probes.

## Clean up

Stop the site with Ctrl-C, remove the `/etc/hosts` line if you added it, and remove the volumes `run-opencode` created:

```bash
docker volume rm a2a-example-opencode-data a2a-example-opencode-cache
```
