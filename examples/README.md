# a2a-cli examples

A short, hands-on course in the `a2a` CLI. Each lesson builds on the last. Start at lesson 1.

## Lessons

### 1. Run a simple A2A server — [`exec-demo/`](exec-demo/)

Turn an ordinary script into a working A2A server with `--exec`. Send it a message and read the response. No A2A-specific code required.

### 2. Discover and talk to an agent

Using the Python script from lesson 1, learn the client side:

- `card get` — fetch an agent card, plain and with `-o json`
- set the agent card through config so you can drop the `-a` flag, and export the card
- `send` a simple text message

### 3. Messages and tasks

Go deeper into the `message` and `task` features: multi-part messages, streaming and async sends, and managing tasks (`task get`, `task list`, `task cancel`, `task subscribe`).

## Learn more

Read the [a2a-cli specification](../specification/SPEC.md) for everything the tool offers.
