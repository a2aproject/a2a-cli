# Agent server: `a2a server <harness>`

> [!WARNING]
> Alpha. Commands, flags and stored state may change.

`a2a server` runs one coding agent per process, started from its project
directory. The agent runs in Docker, is reachable over A2A, from Telegram,
Discord and email, and joins a private network with your other agents. Each
agent can also have a public URL.

How it works, with diagrams, is in the [architecture](agent-server-architecture.md).
Step-by-step walkthroughs are [lesson 5](../examples/05-agent-server/) (local
agent, pairing, approvals) and [lesson 6](../examples/06-tailscale-telegram-grok/)
(Tailscale, Telegram, a Grok webhook node).

Everything belongs to that agent: its workspace, channels, network identity and
pairings. Several agents on one machine are several directories and processes.
They find each other over the private network the same way agents on different
machines do.

## Quick start

```sh
cd ~/src/app
a2a server opencode --name coder --secret OPENROUTER_API_KEY
```

The first run saves the flags to `.a2a/config.yaml` and starts the agent in
the foreground. Later runs, from the project or a subdirectory, are just
`a2a server`. Run `a2a server` without flags in a new directory and it asks
a few questions instead.

On start, the server saves an owner token for its local URL in your
`~/.config/a2a-cli/credentials.json` and prints how to call it, so from
another terminal on the same machine:

```sh
a2a send -a http://127.0.0.1:<port> "What does this project do?"
```

Anyone else needs an invitation:

```sh
a2a server pair invite anna                    # in ~/src/app: prints a one-time code
a2a pair add <agent-url> <code>                # on anna's side: saves a token for that URL
```

`a2a send`, `a2a task …` and `a2a card get` attach the saved token
automatically.

Harnesses: `opencode` (`opencode acp`) and `claude` (Zed's
`claude-agent-acp`). `a2a server acp --command "<cmd>"` runs any agent that
speaks the [Agent Client Protocol](https://agentclientprotocol.com), and
`--command` also replaces a profile's command, such as
`a2a server opencode --command "cloudcode acp"`.

## The agent directory

```
~/src/app/
├── …project files…        # the workspace
└── .a2a/
    ├── config.yaml        # agent definition, no secrets; can be committed
    ├── .gitignore         # "state/"
    └── state/             # never committed
        ├── server.db      # SQLite: tasks, sessions, links, tokens, contacts, nodes
        ├── secrets        # tokens and API keys, mode 0600
        ├── tsnet/         # Tailscale identity: the same URL after restarts
        ├── home/          # harness home, mounted at /home/agent
        ├── server.log
        └── server.sock    # control socket for management commands
```

Management commands find the agent the way git finds a repository, or take
`--dir <path>`. They talk to the running process over `server.sock`, so
changes apply without a restart.

`--secret VAR`, `--telegram env:VAR` and similar flags read values from your
shell when you run the command and store them in `state/secrets`. A detached
or service process doesn't depend on your shell's environment.

> [!WARNING]
> `config.yaml` decides what runs on your machine: `sandbox: local`,
> `command`, `image`, `mounts`, `env` and `secrets`. Running `a2a server` in a
> cloned repository runs whatever its `config.yaml` says, with access to the
> mounts and secrets it names. Read a `config.yaml` you didn't write before you
> start an agent from it, as you would a Makefile or a CI config.

## Running it

| Command | What it does |
| --- | --- |
| `a2a server` | Starts the agent in the foreground. |
| `a2a server --detach` | Starts it in the background. |
| `a2a server --service` | Installs and starts a systemd (Linux) or launchd (macOS) user service. |
| `a2a server status` | URLs, channels, owner links, task counts. |
| `a2a server logs [-f]` | The server and harness log. |
| `a2a server stop [--uninstall]` | Stops it, detached or as a service; `--uninstall` also removes the service. |

## Sandbox

By default the harness runs in Docker:

- the workspace is mounted at its own path, and more directories with
  `--mount src[:dst][:ro]` (repeatable; the first one is the workspace);
- `.a2a` shows up as an empty read-only directory, so the harness can't read
  the secrets, tokens or config of its own server;
- `state/home` is mounted as `/home/agent`;
- the container runs as your user (as container root under rootless Docker,
  which maps to you), with all capabilities dropped, `no-new-privileges`,
  4 GB of memory, 2 CPUs and 512 processes.

The image is `ghcr.io/a2aproject/harness-<harness>:latest`. If it can't be
pulled, the server builds it from the Dockerfile bundled in the binary
(`internal/harness/images`). `--image` uses another image.

`--sandbox local` runs the harness on the host, for trusted setups or when
Docker isn't available. It runs as you, so it can read your
`~/.config/a2a-cli/credentials.json`, including the owner token of this
machine.

The harness calls other agents with the `a2a` CLI, through the server (see
[Agent to agent](#agent-to-agent)), over a Unix socket that only the harness
reaches: in Docker on Linux it is mounted at `/run/a2a/a2a.sock`, along with
the server's own binary at `/usr/local/bin/a2a`. `A2ACLI_UNIX_SOCKET=a2a=<socket>`
sends requests for the host `a2a` through it, and everything else as usual.
Docker Desktop (macOS) can't mount it, so there the harness connects over TCP
with a key the server writes into the harness's `a2a` credentials for that URL
only. The harness images also install `a2a`, for macOS; a custom `--image`
needs it too. With `--sandbox local` the server's directory is put first on
the harness's `PATH`.

Not in this version: an egress proxy and credential injection. The container
can reach the network, and API keys passed with `--secret` are visible inside it.

## Approvals

The harness is configured so that every tool call that changes something asks
first. A tool call pauses the task in `input-required` with a `hitl` data part:

```json
{"hitl": {"id": "…", "type": "permission", "title": "bash",
          "options": [{"id": "once"}, {"id": "always"}, {"id": "reject"}]}}
```

Answer with a message on the same task: the option ID or label, `approve`,
`deny`, or a data part `{"hitl": {"id": "…", "answer": "once"}}`.

- The **owner** answers approvals. `--approver pair:anna` adds approvers.
- A **member** can talk to the agent, but can't approve. When a member's task
  needs an approval, the owner gets it on their messengers with buttons, and the
  member sees `input-required` until the owner decides.
- `--allow-tools read,search` and `--deny-tools delete` answer by ACP tool kind
  without asking.
- Every question also has a page, `/hitl/<token>`, linked from email approvals.

## Who can call the agent

| Caller | How it is recognized | Principal |
| --- | --- | --- |
| You, on the machine running the agent | The owner token the server saves in your a2a credentials | `user:owner` |
| Messenger account | Linked once with a pairing code | `user:owner` or `pair:<name>` |
| Your machines on the tailnet | Tailscale `WhoIs`: the same user as the agent | `user:owner` |
| Other tailnet users | `network.identities` in `config.yaml`, or the `a2aproject.org/cap/a2a` app capability | `user:<login>` |
| Anyone else, on any listener | Bearer token from pairing | `pair:<name>` |
| [Webhook nodes](#webhook-nodes) | Their node token, as members | `node:<name>` |

```sh
a2a server pair invite grok                 # member: can talk to the agent
a2a server pair invite anna --role owner    # owner: can also approve
a2a server pair list                        # list issued tokens
a2a server pair rm grok                     # revoke its tokens and messenger links
```

A code works once, for 15 minutes. The caller exchanges it with
`a2a pair add <agent-url> <code>`, or sends `link <code>` to the agent's bot.
The agent card and `/pair` are public; every A2A request needs a principal.

The owner token the server saves for this machine is listed as `local`.
`a2a server pair rm local` revokes it, and the next start issues a new one;
the name `local` can't be invited.

## Networks

| Flag | Reachable from | Identity |
| --- | --- | --- |
| *(none)* | localhost and messengers | pairing tokens |
| `--private tailscale` | your tailnet, as `https://<name>.<tailnet>.ts.net` | Tailscale `WhoIs` |
| `--private lan` | the local network, over plain HTTP; peers found with mDNS | pairing tokens |
| `--public tailscale` | the internet through Funnel, at the same URL | pairing tokens |
| `--public ngrok` | the internet, `--ngrok-url` for a reserved domain | pairing tokens |

Each agent joins the tailnet as its own machine with
[tsnet](https://tailscale.com/kb/1244/tsnet), even when the computer already
runs Tailscale. On the first run it prints a login link, or use
`--tailscale-authkey env:TS_AUTHKEY`. Turn off key expiry for the machine in the
admin console. HTTPS must be enabled in the tailnet, and Funnel needs the
`funnel` node attribute in the tailnet policy. `--tailscale-control-url` points
at Headscale.

## Messengers

```sh
a2a server opencode --telegram env:CODER_BOT_TOKEN      # or later:
a2a server channel add telegram --token env:CODER_BOT_TOKEN
a2a server channel add discord --token env:CODER_DISCORD_TOKEN
a2a server channel add email --address coder@example.com \
  --imap imap.example.com:993 --smtp smtp.example.com:465 \
  --password env:MAIL_PASSWORD --trusted-authserv mx.example.com
a2a server channel rm discord
```

A bot token can be polled by only one process, so each agent gets its own bot.
On start, the server prints a link for linking yourself as the owner (for
Telegram, a `t.me` deep link). Unknown senders get a pairing hint; their
attachments aren't downloaded. `channel add` checks a bot token before saving
it. A messenger that fails while running is restarted with backoff, and
`a2a server status` shows its error in the meantime.

- **Telegram:** long polling. Answers are streamed by editing a message about
  once a second; approvals are inline buttons. In groups the bot answers when
  mentioned or replied to.
- **Discord:** gateway WebSocket with the Message Content intent; `/link`,
  `/new` and `/stop` commands; buttons; one thread per conversation in guild
  channels; missed messages are fetched on reconnect.
- **Email:** IMAP IDLE and SMTP. Only mail with `dmarc=pass` for the sender's
  domain or aligned `dkim=pass` in the topmost `Authentication-Results` of the
  trusted server is accepted. Mail with several `From` addresses, auto-replies,
  lists and the agent's own mail are dropped. SMTP needs TLS (port 465 or
  STARTTLS) unless the server is on localhost. Replies stay in the
  thread, and only questions and final results are sent. Approve with a
  one-word reply: `approve`, `always` or `deny`.

A conversation (chat, thread, email thread) is one A2A context. `/new` starts
a new one, and `/stop` cancels the running task.

## Agent to agent

The harness calls contacts and webhook nodes with the `a2a` CLI. The server
proxies the calls, so they use this agent's identity, tailnet and stored
tokens, which never enter the sandbox:

- Every new prompt starts with an `[a2a agents]` line with the agents URL of
  the conversation and the agents the harness can call:
  `[a2a agents] Agents URL for this conversation: http://a2a/k3 (agents: coder, grok; …)`.
  The handle (`k3`) stands for the A2A context, the user the turn is for and
  the hop count, which decide whose behalf calls are made on and where results
  are posted. It isn't a secret: only the harness reaches the socket.
- `<agents URL>/<name>` is an A2A agent, so the harness runs
  `a2a send -a http://a2a/k3/coder "…"`.
- The how-to is in the harness's global instructions, which the server keeps in
  a marked section of `~/.config/opencode/AGENTS.md` and `~/.claude/CLAUDE.md`
  in `state/home`, along with the list of agents. A local harness uses your
  home, so it only gets the `[a2a agents]` line.

```sh
a2a server peers                     # agents found on the private network
a2a server contact add coder         # a peer on the tailnet: no code needed
a2a server contact add grok --url https://grok.example.com --code K7Q2-9XF4
a2a server contact list
a2a server contact rm coder
```

- `a2a send` waits for the answer. With `--async` it returns the task ID at
  once, and the server watches the task. When it finishes, its result is
  posted into the originating conversation as a new turn, so the agent
  continues and tells the user.
- Calls carry `X-A2A-Serve-On-Behalf-Of` and a hop count (at most 3), which
  stops agents from calling each other in a loop. The callee counts hops from
  every caller, but honors `X-A2A-Serve-On-Behalf-Of` only from your own agents
  (the same tailnet login), since the user it names belongs to the caller.
- When a called agent needs an approval, the calling agent relays it to the
  user in the chat they are already in: "coder wants to run git push",
  with buttons. The answer goes back to the remote task, and its result is
  posted into the conversation. The harness itself can't answer approvals.

## Webhook nodes

An agent can host agents that only run when woken, such as a Grok Bot
routine. A node is a pairing with a webhook:

```sh
a2a server pair invite grok --webhook https://<grok routine webhook URL> \
  --webhook-header "Authorization: Bearer env:GROK_WEBHOOK_TOKEN"
# → a message for the agent behind the webhook: its skill URL and a pairing code
a2a server pair invite grok          # a new code, if the node lost its token
a2a server pair list                 # callers and nodes
a2a server pair rm grok
```

Running `pair invite --webhook` again for an existing node updates it, and its
previous token stops working.

The agent calls the node as `grok` (`a2a send -a http://a2a/<handle>/grok`), and it is at
`<agent-url>/n/grok` for the owner. The server keeps its tasks in SQLite and
wakes it with a POST signed with
`X-A2A-Serve-Signature: t=<unix time>,v1=<hex HMAC-SHA256 of "<t>.<body>">`. The node
answers with `a2a relay reply <task> --final "…"`. A task with no event
for 10 minutes fails. The node's instructions are served at
`<agent-url>/relay/skill.md`.

The node token also authenticates A2A calls to the agent, as the member
`node:grok`, so the node can send the agent tasks of its own:
`a2a send -a <agent-url> "…"` attaches the token saved by `a2a pair add`, or
`$A2A_RELAY_TOKEN`. `pair rm grok` revokes both directions.

## HTTP routes

Served on every listener:

| Route | |
| --- | --- |
| `GET /`, `GET /.well-known/agent-card.json` | the agent card |
| `POST /` | A2A JSON-RPC |
| other paths | A2A HTTP+JSON (REST) |
| `POST /pair` | exchange a pairing code for a caller or node token |
| `/hitl/<token>` | answer a question in a browser |
| `/n/<node>/…` | webhook nodes |
| `/relay/api/…` | the node API used by `a2a relay` |
| `GET /skill.md`, `GET /relay/skill.md` | skills for agents calling this one, and for nodes |

## Configuration reference

`config.yaml` holds what the flags set. Secret values are references such as
`secret:TELEGRAM_BOT_TOKEN`.

```yaml
name: coder
harness: opencode
command: [cloudcode, acp]        # optional: replaces the profile's command
sandbox: docker                  # or local
mounts: [/home/u/src/app, /home/u/docs:/docs:ro]
secrets: [OPENROUTER_API_KEY]     # passed to the harness as environment variables
port: 46757                      # localhost port, chosen on the first run
network:
  private: tailscale
  public: ngrok
  ngrok_url: https://coder.ngrok.app
  identities: {anna@example.com: member}
channels:
  telegram: {token: secret:CODER_BOT_TOKEN}
approvals:
  allow: [read, search]
  approvers: [pair:anna]
limits: {memory: 8g, cpus: "4", pids: 1024}
```

## Testing a harness

`internal/agentsrv/conformance_test.go` runs the conformance scenarios
through the whole server against a scripted ACP agent
(`internal/agentsrv/testdata/fakeacp`): text turn, tool progress, approve,
deny, always-allow, cancel mid-tool, follow-up in context, restart and resume,
agent crash, and a queued concurrent task. To try a real harness without
Docker:

```sh
a2a server opencode --sandbox local --command "opencode acp"
```
