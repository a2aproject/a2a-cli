# A2A relay (concept prototype)

> [!WARNING]
> This is a prototype for connecting agents to the A2A ecosystem. Commands,
> flags, wire formats and stored state may change or be removed without
> notice. Don't rely on it for anything you can't afford to lose.

Many capable agents can't be A2A servers on their own. Cloud agents such as
Grok Bot run on vendor computers, can't open ports, and run only when a
webhook wakes them. Coding agents such as opencode and Claude Code have no A2A
interface at all. This prototype adds two commands to `a2a server`:

**`a2a server relay`**: a relay that gives an agent without a public address
a stable public A2A URL.

## How the relay works

Each agent on the relay is a *node* with its own URL, `https://<relay>/n/<node>`,
and its own agent card. Callers always see a standard A2A agent at that URL,
whichever way the relay reaches the node. The relay authenticates callers
against the node's grants in both modes.

### Webhook mode

For agents that run only when something wakes them, such as Grok Bot
routines or serverless functions. The relay acts as the A2A server while the
agent sleeps.

```
 caller ── A2A ──▶ relay /n/<node> ── webhook wake (JSON) ──▶ agent (node)
   ▲                  │  ▲                                        │
   └─ task updates ───┘  └──── a2a server relay reply <task> ─────┘
```

1. A caller sends `SendMessage` to `https://<relay>/n/<node>`. The relay
   creates the task in `submitted` and POSTs a JSON *wake* to the node's
   webhook.
2. The agent wakes up and runs `a2a server relay reply <task> ...` (or POSTs
   to the relay API) to report `working`, ask a question with
   `input-required`, or finish with a result and artifacts.
3. The caller sees each reply as an A2A task update, through streaming, push
   notifications or `GetTask`. If the agent asked a question, the caller's
   answer wakes the agent again under the same task ID.

The relay holds all task state, so the agent needs no memory between wakes.
Replies arrive out of band, so callers should expect asynchronous tasks.

### Connected mode (planned)

For agents that can stay running but have no public address, such as
`a2a server opencode` on a laptop or home server, or any A2A server behind a
NAT. The relay only adds a public address and grants. The agent remains the
A2A server.

```
 caller ── A2A ──▶ relay /n/<node> ══ outbound WebSocket ══ local A2A server (node)
   ▲                                  (opened by the node)          │
   └──────────── responses and streams, in band ◀───────────────────┘
```

The relay serves the node's own agent card with its URL rewritten to the
public one. While the node is disconnected, calls fail with "agent offline".
Queuing them until the node reconnects is an open question. See
[Node modes](design.md#node-modes) in the design doc.

### Roles

| Role | Who | Does |
| --- | --- | --- |
| Operator | Whoever runs the relay. Holds the admin token. | `relay serve`, or deploys it. |
| Owner | The person the agent works for. Usually the operator too. | `relay attach`: registers the node (and its webhook, in webhook mode) and gets a pairing code. |
| Node | The agent behind the relay. | `relay pair` once. Then, in webhook mode, `relay reply` on every wake; in connected mode, it keeps its socket open and answers requests directly. |
| Caller | A person, script or agent sending A2A tasks to the node. | Uses any A2A client with a grant token. |

The relay serves an Agent Skills `SKILL.md` at `https://<relay>/skill.md`
that teaches an agent everything a node does, and serves its own binary at
`https://<relay>/download/a2a`. Onboarding an agent takes one sentence:
"install the skill at `<url>` and pair with code `<code>`".

## Try it locally

```sh
deploy/relay/local-demo.sh
```

This starts a relay and a stand-in webhook agent on localhost. It then
attaches, pairs, grants a caller, and sends a message through the relay. The
stand-in agent answers each wake with `a2a server relay reply`, the same way a
real agent following the skill does.

## Deploy a relay

The relay has to be reachable from the internet so that the agent's vendor
can deliver its replies and callers can reach it. On Google Cloud Run:

```sh
PROJECT=<project> deploy/relay/cloudrun/deploy.sh
```

The script prints the relay URL and the admin token. See
[deploy/relay](../../deploy/relay/README.md) for prerequisites, options and
other hosts.

## Pair in webhook mode

These steps work for any agent that a webhook wakes and that can run shell
commands or make HTTPS calls. Grok Bot is the example.

**1. Set up the webhook on the agent side.** Ask a Grok Bot to create a webhook-triggered routine and give you webhook URL and auth header link.

**2. Attach the webhook to the relay** from your machine, with the admin token
printed by the deploy script:

```sh
export A2A_RELAY_URL=https://<relay> A2A_RELAY_ADMIN_TOKEN=<admin token>

a2a server relay attach https://<grok routine webhook URL> \
  --name grok-bot --description "My Grok Bot" \
  --webhook-header <grok webhook auth header>
```

The command prints the agent's A2A URL, a pairing code (single use, valid for 15 minutes) and a message for the agent. It also saves the relay URL and admin token to
`~/.config/a2a-cli/relay.json`.

**3. Tell Grok Bot to pair**, in a chat:

> Install the skill at https://\<relay\>/skill.md and pair with the relay
> using code K7QF-M2XD. Save the node token it prints as a secret named
> A2A_RELAY_TOKEN.

Grok Bot downloads the CLI from the relay and runs
`./a2a server relay pair K7QF-M2XD --relay https://<relay>`. If the code
expires or the token is lost, issue a new code with
`a2a server relay attach --node <node-id>`.

**4. Tell the routine how to handle wakes.** The routine's prompt should say
something like:

> You are woken by an A2A relay. The request body is a JSON wake with kind,
> taskId, sender, text and instructions fields. Follow the a2a-relay skill:
> first run `./a2a server relay inbox` to catch up, then handle each task.
> Check who the sender is before acting. Finish every task with
> `./a2a server relay reply <taskId> --final "<answer>"`, `--input-required
> "<question>"` or `--fail "<reason>"`. Report progress on long jobs with
> `--status working --text "<progress>"`.

**5. Check the wiring** from your machine:

```sh
a2a server relay ping --node <node-id>     # the relay sends a test wake; the routine should run
a2a server relay status --node <node-id>   # Paired: true
```

**6. Let callers in.** Nodes are private by default. Issue a token per caller:

```sh
a2a server relay grant alice --node <node-id>   # prints a caller token, shown once
```

The caller then uses any A2A client. Tasks are asynchronous because the agent
may take a while to wake up, so send without waiting and poll, stream, or
register a push notification config:

```sh
export A2ACLI_AUTH="Bearer <caller token>"

a2a send -a https://<relay>/n/<node-id> --stream "Hi, how are you?"

# or async

a2a send -a https://<relay>/n/<node-id> --async "Summarize today's news"
a2a task get -a https://<relay>/n/<node-id> <task-id> --wait --timeout 1h
```

`relay grant node:<id>` lets another agent on the same relay call this one,
and `relay update --public` lets anyone call. Revoke a grant with `relay revoke <name>`.


## Commands

| Command | Who | What |
| --- | --- | --- |
| `relay serve` | Operator | Run a relay. `$PORT`, `A2A_RELAY_ADMIN_TOKEN`, `A2A_RELAY_STATE_DIR` and `A2A_RELAY_PUBLIC_URL` set the defaults. |
| `relay attach <webhook>` | Owner | Attach a node. Prints its URL and a pairing code. `--node <id>` re-pairs. |
| `relay pair <code>` | Node | Exchange the code for a node token. |
| `relay reply <task>` | Node | `--final`, `--input-required`, `--fail` or `--status`, with `--artifact <file>`. Retries are safe. |
| `relay task <task>` | Node | Show a task with history and artifacts. |
| `relay inbox [--all]` | Node | Wakes not delivered yet, or tasks still waiting for a reply. |
| `relay send <peer> <text>` | Node | Message another agent. The result arrives as a wake. |
| `relay grant`, `revoke`, `grants` | Node | Manage who can call the node. |
| `relay ping`, `status`, `update` | Node | Test the webhook, show the node, or change its webhook, name or access. |
| `relay skill [--install <dir>]` | Anyone | Print the a2a-relay skill. |

Node commands find the relay and token in `--relay`/`--token`,
`A2A_RELAY_URL`/`A2A_RELAY_TOKEN`, or `~/.config/a2a-cli/relay.json`. The owner
can run any node command with `--node <id>` and the admin token. Every command
takes `-o json`.

## Status and limitations

- Tasks, inboxes and idempotency keys are in memory, so a relay restart loses
  in-flight tasks. That is also why the relay runs as a single, always-on
  instance. Attached nodes, tokens and grants persist in the state directory.
- Wakes authenticate with a static header and aren't signed. Webhook headers
  are stored in plaintext in `nodes.json`.
- The relay doesn't restrict webhook or `send` targets, and it doesn't
  rate-limit callers.
- A task fails when its node posts nothing for `--task-timeout` (default 1h).
  There is no shorter deadline for a wake that is never picked up.
