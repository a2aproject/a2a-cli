# Lesson 6 — Tailscale, Telegram, and a Grok bot

Last updated: 2026.10.22

Lesson 5 served a coding agent on localhost. This one makes it a personal
agent you can reach from anywhere: on your tailnet, from a Telegram chat, and
with a public URL so a Grok bot can work for it as a webhook node.

## What you'll learn

- How the agent joins your tailnet as its own machine, and why your devices
  need no pairing there
- How to give it a public HTTPS URL with Tailscale Funnel
- How to talk to it, and approve its tool calls, from Telegram
- How to host a Grok bot as a webhook node the agent can delegate to
- How to keep it running as a service

## Prerequisites

- Lesson 5's setup: the `a2a` CLI, Docker, and an API key such as
  `OPENROUTER_API_KEY` exported in your shell.
- A [Tailscale](https://tailscale.com) account, with the machine you work
  from on the tailnet.
- A Telegram account.
- A Grok bot that can be triggered by an HTTP webhook. You need its webhook
  URL, and the token it expects if it checks one.

## 1. Prepare the tailnet

The agent serves HTTPS on its tailnet name, and Funnel publishes the same URL
on the internet. Both need a one-time change in the Tailscale admin console:

1. On the **DNS** page, enable **HTTPS certificates**.
2. In the **Access controls** policy, allow Funnel:

   ```json
   "nodeAttrs": [
     { "target": ["autogroup:member"], "attr": ["funnel"] }
   ]
   ```

## 2. Create a Telegram bot

In Telegram, talk to [@BotFather](https://t.me/BotFather): send `/newbot`,
choose a name and a username, and copy the token it gives you. Each agent
needs its own bot, because only one process can poll a bot token.

```bash
export CODER_BOT_TOKEN=<token from BotFather>
```

## 3. Start the agent

From the agent's project directory:

```bash
mkdir -p ~/a2a-lesson6/coder && cd ~/a2a-lesson6/coder
a2a server opencode --name coder --secret OPENROUTER_API_KEY \
  --private tailscale --public tailscale \
  --telegram env:CODER_BOT_TOKEN
```

`env:CODER_BOT_TOKEN` reads the token from your shell now and stores it in
`.a2a/state/secrets`, so later runs don't need the variable.

On the first run, the agent asks to join your tailnet:

```
tailscale: open https://login.tailscale.com/a/... to add "coder" to your tailnet
```

Open the link and approve the machine. In the admin console, **disable key
expiry** for it, or the agent drops off the tailnet when the key expires. On
a headless machine, pass `--tailscale-authkey env:TS_AUTHKEY` instead of
using the link.

Then it prints its URLs and the Telegram bot:

```
✓ local    http://127.0.0.1:46757
✓ private  https://coder.your-tailnet.ts.net
✓ public   https://coder.your-tailnet.ts.net
✓ telegram @coder_agent_bot
→ link yourself as owner: https://t.me/coder_agent_bot?start=K7Q29XF4   (15 min)
```

The private and public URLs are the same name: on the tailnet it is served
directly, from the internet through Funnel.

## 4. Link Telegram

Open the `t.me` link on your phone and press **Start**. The bot replies:

```
Linked as owner. Write to talk to coder.
```

Now write to it like to any chat:

- Each chat is one conversation (an A2A context); `/new` starts a fresh one
  and `/stop` cancels the running task.
- Answers stream in by editing one message.
- When the agent wants to run something, you get the request with buttons:
  allow once, always allow, or reject. Only you, the owner, can answer.

Try: `Create a file notes.md with three ideas for a weekend project.`

## 5. Call it from your other machines

Your devices on the tailnet are recognized by their Tailscale identity, so
they need no pairing code:

```bash
a2a send -a https://coder.your-tailnet.ts.net "What's in notes.md?"
```

Other people get a code from you, as in lesson 5:
`a2a server pair --name anna`, and then they run `a2a pair add` with it.

## 6. Add the Grok bot as a webhook node

A webhook node is an agent that only runs when woken. The agent server gives
it an A2A address and holds its tasks; when a task arrives, it POSTs a signed
wake to the webhook. The bot answers later with the `a2a` CLI or plain HTTPS.

In the agent's directory, add the node. Header values can be `env:VAR`:

```bash
export GROK_WEBHOOK_TOKEN=<token your Grok webhook expects>
a2a server pair --name grok \
  --webhook <your Grok bot's webhook URL> \
  --webhook-header "Authorization: Bearer env:GROK_WEBHOOK_TOKEN" \
  --description "Grok: web search and current events"
```

```
→ node code 8HD2-PQ7M (15 min, single use); grok is reachable for the agent as "grok"
  node: a2a pair add https://coder.your-tailnet.ts.net 8HD2-PQ7M
  node URL: https://coder.your-tailnet.ts.net/n/grok
```

The commands use the public URL, because Grok calls the agent from the
internet.

## 7. Set up the Grok side

The agent serves everything the bot needs to know at
`https://coder.your-tailnet.ts.net/relay/skill.md`: how to download the
`a2a` CLI from the agent, pair, read a wake, and reply. The download is the
server's own binary, so it runs for the bot when the server runs on Linux;
otherwise the bot needs a Linux build of `a2a`, or it can use the plain HTTPS
API at the end of the skill. Give your Grok bot
instructions along these lines:

```
You are the webhook node "grok" of the agent at https://coder.your-tailnet.ts.net.
Read https://coder.your-tailnet.ts.net/relay/skill.md and follow it.
Your pairing code is 8HD2-PQ7M.
```

On its first run, the bot pairs with `a2a pair add` and gets a node token.
Store that token where the bot keeps secrets, as `A2A_RELAY_TOKEN`: it
includes the agent's URL, so it is the only setting the bot needs on later
wakes. Each wake is a JSON body with `kind`, `taskId`, `text` and
`instructions`, signed with `X-A2A-Serve-Signature`, and the bot answers with:

```bash
a2a relay reply <taskId> --final "Here is what I found: ..."
```

Check that the node is paired:

```bash
a2a server pair list
```

The `grok` row shows `node:grok`. It says `not paired` until the bot has
redeemed its code. If the code expired, run `a2a server pair --name grok`
again for a new one.

## 8. Delegate to Grok from Telegram

In the Telegram chat:

```
Ask grok what the top AI news is today, and summarize it in three bullets.
```

The agent sees `grok` in the `[a2a agents]` line of its prompt and calls it
with `a2a send`, usually with `--async` because a webhook node answers
later. The server wakes the bot, the bot replies, and
the result is posted back into your chat as a new turn, where the agent
summarizes it. A node task with no reply for 10 minutes fails.

From a machine on your tailnet, you can also call the node directly as its
owner:

```bash
a2a send -a https://coder.your-tailnet.ts.net/n/grok "What's new with Tailscale?"
```

## 9. Keep it running

Stop the foreground process with Ctrl-C, then install the agent as a user
service (systemd on Linux, launchd on macOS):

```bash
a2a server --service
a2a server status
a2a server logs -f
```

`a2a server stop --uninstall` removes the service again.

## Troubleshooting

| Symptom | What to check |
| --- | --- |
| `HTTPS certificates are disabled in this tailnet` | Step 1: enable HTTPS on the DNS page. |
| No `public` URL | The `funnel` attribute in the tailnet policy (step 1). |
| The bot never answers | `a2a server logs`: a wake that failed shows the webhook's status, such as `webhook returned 401` for a wrong token. `a2a server pair list` shows whether the node is paired. |
| The node lost its token | `a2a server pair --name grok` issues a new code; pairing replaces the old token. |
| Telegram is silent | Only one process can use a bot token; stop any other process using it. |

## Next steps

The [agent server guide](../../docs/agent-server.md) covers Discord and email,
approvers besides you, and contacts between agents on the same tailnet. The
[architecture](../../docs/agent-server-architecture.md) shows how the pieces
fit together.
