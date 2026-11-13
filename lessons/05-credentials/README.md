# Lesson 5: Credentials

Your agent served you in every lesson, but it doesn't serve just anyone: a2a-serve checks a token on each request. You never typed one. In this lesson you find out where it came from, see what happens with the wrong one, and learn the ways `a2a` sends credentials to agents.

![Inspecting the credential helper with auth get, sending a wrong token, and checking config show](demo.gif)

## What you'll learn

- How agents read credentials from request headers
- What a credential helper is, and how `a2a` picks one for each agent
- How `--auth` and `--svc-param` send credentials yourself
- How to inspect credentials without printing secrets

## Prerequisites

The agent from [lesson 1](../01-first-message/README.md), and the `.env` in `~/a2a-lessons` that points at it.

## Step 1: Who signed your requests?

```bash
a2a config show | grep credential-helper
```

```text
credential-helper  a2a-serve credential    user-file
```

When you ran `a2a-serve doctor --fix`, it added a **credential helper** to `~/.config/a2a-cli/config.yaml`:

```yaml
credential-helper:
  - command: [a2a-serve, credential]
```

A credential helper is a program `a2a` asks for the headers to send to an agent. Before it calls an agent, `a2a` runs the helper with the agent's URL and adds the headers it returns. `a2a-serve credential` returns the token for agents you started, and for agents you joined with a pairing code.

## Step 2: Look at the headers

`a2a auth get` asks the helper for a URL and shows what it returned. Values stay hidden unless you ask:

```bash
a2a auth get "$(a2a-serve status --dir agent --url=local)"
```

```text
helper: a2a-serve credential
HEADER         VALUE
Authorization  <redacted>
```

`--reveal` prints the values. Treat them like passwords.

## Step 3: Send a wrong token

`--auth` sets the `Authorization` header yourself, and wins over the helper:

```bash
a2a send --auth "Bearer not-a-real-token" "hello"
```

```text
Error: failed to send message: this agent doesn't recognize the token, which may be mistyped, revoked or replaced; ask the agent's owner for a pairing code and run `a2a-serve pair join <agent-url> <code>`
```

```bash
echo "exit code $?"
```

```text
exit code 1
```

The agent refused the request, and `a2a` exited with code 1, the code for errors an agent returns. The card is still readable, because a2a-serve serves it to everyone:

```bash
a2a card get --auth "Bearer not-a-real-token"
```

## Step 4: Other agents, other credentials

Agents you didn't start need their own credentials. Pick the option that fits how the agent authenticates:

| Option | Sends |
|---|---|
| `--auth "Bearer $TOKEN"` | `Authorization: Bearer ...` on every request. |
| `--svc-param "X-API-Key=$KEY"` | Any header, repeatable. |
| A credential helper | The headers a program returns for each agent's URL. |

Helpers suit tokens that expire or differ per agent. List them in `~/.config/a2a-cli/config.yaml`, each with an optional scope that limits it to some hosts:

```yaml
credential-helper:
  - command: [a2a-serve, credential]
  - scope: "*.corp.example.com"
    command: [/usr/local/bin/corp-token]
```

For a request to `review.corp.example.com`, `a2a` runs `corp-token`, the helper with the most specific matching scope. For any other host, it runs `a2a-serve credential`. Helpers follow the [Bazel credential helper protocol](https://github.com/EngFlow/credential-helper-spec), so helpers written for Bazel work too. [A credential helper](../../examples/credential-helper/README.md) shows how to write one in a few lines of shell.

A project's `.env` can't set `--credential-helper`: a cloned repository could otherwise make `a2a` run a program of its choice. Keep tokens out of project files too; use the environment, your own config files or a helper.

## Recap

- Agents read credentials from request headers; `--auth` and `--svc-param` set them.
- A credential helper returns the headers for each agent, and a2a-serve installs one.
- `a2a auth get` shows which helper answers for a URL, without printing secrets.

You've finished the lessons. Stop the agent when you're done:

```bash
a2a-serve stop --dir ~/a2a-lessons/agent
```

## Next

- The [User Guide](../../docs/guide/agent-cards.md) has a short page on each topic.
- [Examples](../../examples/README.md) are recorded recipes to copy.
- [Advanced](../../docs/advanced/architecture.md) explains how `a2a` works inside.
