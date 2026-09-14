# Lesson 1 — build an A2A server from a script

Last updated: 2026.09.08

The first step in learning the `a2a` CLI is to stand up a server you can send messages to and get replies from. The CLI gives you two ready-made server modes, no A2A-specific code required.

The simplest is `--echo`, which sends your message straight back. It is a "ping" for A2A: perfect for a first connection test. The more advanced is `--exec`: point it at any script that reads input and prints output, and it becomes a working A2A server. `--exec` is where the fun is — it turns any program into an agent for demos, testing, and small jobs.

> `--echo` and `--exec` are built for learning, demos, and testing, not for production use.

## What you'll learn

- How to start the simplest server with `--echo`
- How `--exec` wraps an ordinary script as an A2A server
- How to send a one-shot message and read the reply
- How to stream a reply piece by piece

## How `--exec` works

The CLI hands your script the message on **stdin** and turns whatever the script prints on **stdout** into the response. The exit code sets the result: `0` succeeds, non-zero fails. Anything on **stderr** is logged and shown in the failure message.

This example ships two small scripts:

| File | What it does |
|---|---|
| `content-generator.sh` | Uppercases the message and adds a word count. Returns one response. |
| `a2a_unaware_agent.py` | Prints one numbered line per word, with a short delay — handy for streaming. |

## Prerequisites

Install the CLI (see the [repo README](../../README.md)):

```bash
go install github.com/a2aproject/a2a-cli@latest
# go install names the binary `a2a-cli`; rename it to `a2a`
mv "$(command -v a2a-cli)" "$(dirname "$(command -v a2a-cli)")/a2a"
```

## Step 1 — warm up with the echo server

Start the simplest possible server in **terminal A**:

```bash
a2a server --echo --port 8080
```

Send it a message from **terminal B** and get the same text back:

```bash
a2a send -a http://localhost:8080 "hello world from A2A"
```

That is a full A2A round trip. Stop the echo server (Ctrl-C) and move on to `--exec` for something more useful.

## Step 2 — run the scripts on their own

Before the CLI is involved, confirm each script works on a plain pipe:

```bash
echo "1 2 3 4 5 helloworld" | bash content-generator.sh
echo "5 4 3 2 1 helloworld" | python3 a2a_unaware_agent.py
```

## Step 3 — start a server from a script (terminal A)

Wrap one of the scripts in a server:

```bash
# Bash script — returns the whole output as one response
a2a server --exec "bash content-generator.sh" --port 8080

# Python script — streams one piece per line.
# -u keeps output unbuffered so pieces arrive promptly; --chunk splits on newline.
a2a server --exec "python3 -u a2a_unaware_agent.py" --chunk=$'\n' --port 8080
```

Leave the server running.

## Step 4 — send a message (terminal B)

```bash
# Fetch the agent card to confirm the server is up
a2a card get -a http://localhost:8080 -o json

# One-shot response
a2a send -a http://localhost:8080 "hello world from A2A"

# Watch pieces arrive live (pair with the --chunk server above)
a2a send -a http://localhost:8080 --stream "one two three four"
```

## Test

`test.sh` checks both scripts on a plain pipe — no server needed. Because `--exec` only pipes the message to stdin and reads stdout, this exercises the same path the server runs:

```bash
bash test.sh
```

## Next

You have a running agent. In [lesson 2](../02-card-and-send/) you learn the client side properly: reading the agent card, saving it, and setting it once through config so you can drop the `-a` flag from every command.

## Learn more

These scripts scratch the surface. The `a2a` CLI also does agent-card discovery, multi-part messages, async and streaming sends, task management, and echo and proxy server modes. Read the [a2a-cli specification](../../specification/SPEC.md) to explore everything the tool offers.
