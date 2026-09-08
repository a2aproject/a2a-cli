# Lesson 2 — discover and talk to an agent

In lesson 1 you started a server. Now learn the client side: read an agent's card, save it, set it once through config, and send a message.

## What you'll learn

- What an **agent card** is and how to fetch it, plain and as JSON
- How to export a card to a file
- How to set the agent through a `.env` file so you can drop the `-a` flag
- How to send a simple text message

## Prerequisites

The `a2a` CLI installed (see the [repo README](../../README.md)). This lesson uses the built-in **echo** server, so there is nothing to write and you do not need to have finished lesson 1 first.

## Start the agent (terminal A)

The echo server sends your message straight back — a simple partner for learning the client:

```bash
a2a server --echo --port 8090 --name "Echo Agent"
```

Leave it running. Do everything below in **terminal B**.

## Step 1 — get the agent card

Every A2A agent publishes an **agent card** that describes who it is and how to reach it. Fetch it:

```bash
a2a card get http://localhost:8090
```

```text
Echo Agent
  URL: http://localhost:8090
  Version: 1.0.0
```

Add `-o json` for the raw card — useful for scripts and for saving it:

```bash
a2a card get http://localhost:8090 -o json
```

## Step 2 — export the card

Save the card to a file so you can inspect it or serve it later:

```bash
a2a card get http://localhost:8090 -o json > agent-card.json
```

## Step 3 — set the card through config

Typing `-a http://localhost:8090` on every command gets old. Put it in a `.env` file instead:

```bash
cp .env.example .env
```

This `.env` contains the following:

```dotenv
A2ACLI_AGENT_CARD=http://localhost:8090
```

The CLI reads `.env` from the working directory automatically, so you can now drop `-a`:

```bash
a2a card get          # uses A2ACLI_AGENT_CARD from .env
a2a config show       # confirm the value and where it resolved from
```

```text
SETTING      VALUE                  SOURCE
agent-card   http://localhost:8090  local-file
...
```

The `local-file` source means the value came from a `.env` file in the working directory.

## Step 4 — send a message

The echo agent sends your text right back. Every `send` runs as a task, so the CLI prints the task, its status, and the reply in the artifacts:

```bash
a2a send "hello world from A2A"
```

```text
Task:     01a08152-ae99-73ae-98a3-58b82e14bde0
Context:  01a08152-ae99-74bc-bfc7-d6ce8b2c74d2
Status:   completed (2026-09-08T14:01:14Z)
Artifacts:
  [01a08152-ae99-75cd-891b-cceb14223d58] hello world from A2A
History:
  [user] hello world from A2A
```

## Run the whole lesson

`run.sh` does every step above — start the agent, read and export the card, set config, and send a message — then stops the agent:

```bash
bash run.sh
```

## Next

Lesson 3 shows the three ways to [configure the CLI](../03-config/) and lists every setting you can change.

## Learn more

Read the [a2a-cli specification](../../specification/SPEC.md) for the full set of commands and flags.
