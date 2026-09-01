# `--exec` demo — turn a script into an A2A server

Two simple, runnable scripts for demonstration. Each builds a working A2A server from an ordinary program. You write no A2A-specific code; the `a2a` CLI does the work through its `--exec` mode.

With `--exec`, the CLI hands your script the message on **stdin** and turns whatever the script prints on **stdout** into the response. The exit code sets the result: `0` succeeds, non-zero fails, and anything on **stderr** is logged (and shown in the failure message).

| File | What it does |
|---|---|
| `content-generator.sh` | Uppercases the message and adds a word count. Returns one response. |
| `a2a_unaware_agent.py` | Prints one numbered line per word, with a short delay — handy for streaming. |

## Try the scripts on their own

Run either script directly and pipe in a message:

```bash
echo "1 2 3 4 5 helloworld" | bash content-generator.sh
echo "5 4 3 2 1 helloworld" | python3 a2a_unaware_agent.py
```

## Run them as an A2A server

Install the CLI (see the [repo README](../../README.md)):

```bash
go install github.com/a2aproject/a2a-cli@latest
```

### Server Instance

Start a server that wraps one of the scripts (**terminal A**):

```bash
# Bash script — returns the whole output as one response
a2a server --exec "bash content-generator.sh" --port 8080

# Python script — stream one piece per line.
# -u keeps output unbuffered so pieces arrive promptly; --chunk splits on newline.
a2a server --exec "python3 -u a2a_unaware_agent.py" --chunk=$'\n' --port 8080
```

### Client Instance

Send a message with the client (**terminal B**):

```bash
# Know your agent
a2a card get -a http://localhost:8080 -o json

# One-shot response
a2a send -a http://localhost:8080 "hello world from A2A"

# Watch pieces arrive live (pair with the --chunk server above)
a2a send -a http://localhost:8080 --stream "one two three four"
```

## Learn more

These scripts scratch the surface. The `a2a` CLI does much more: agent-card discovery, direct-endpoint connections, and multi-part messages with text, file, and data parts. It also handles async and streaming sends, task management, and echo and proxy server modes. Read the [a2a-cli specification](../../specification/SPEC.md) to explore everything the tool offers.
