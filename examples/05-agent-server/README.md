# Lesson 5 — serve a coding agent

Last updated: 2026.10.22

The first four lessons put a script behind A2A. This one serves a real coding
agent, opencode or Claude Code, from its project directory with
`a2a server <harness>`, and talks to it like any other A2A agent.

## What you'll learn

- How to start a coding agent as an A2A server, and where it keeps its state
- How pairing gives you a token for it
- How the agent asks before a tool call, and how you approve it
- How one agent calls another

## Prerequisites

- The `a2a` CLI installed (see the [repo README](../../README.md)).
- Docker. Without it, add `--sandbox local` to the start commands below; the
  harness then runs on your machine and needs to be installed there.
- An API key for the model, such as `ANTHROPIC_API_KEY`, exported in your shell.

## 1. Create a workspace

The agent works in its project directory. Make a small one, with a hint that
keeps the answers short:

```bash
mkdir -p ~/a2a-lesson5/coder && cd ~/a2a-lesson5/coder
cat > AGENTS.md <<'EOF'
You are reached over A2A. Keep answers to one short sentence.
EOF
```

## 2. Start the agent

In **terminal A**, from the workspace:

```bash
a2a server opencode --name coder --secret ANTHROPIC_API_KEY
```

For Claude Code, use `a2a server claude` instead.

The first run saves the flags to `.a2a/config.yaml` and the key to
`.a2a/state/secrets`, pulls or builds the harness image, and stays in the
foreground. Later runs from this directory are just `a2a server`.

In **terminal B**, check that it runs and note its local URL:

```bash
cd ~/a2a-lesson5/coder
a2a server status
```

```
coder (opencode) running for 12s, pid 41233
  dir      /home/you/a2a-lesson5/coder
  local    http://127.0.0.1:46757
```

## 3. Pair with it

Every A2A request needs a token. Mint a one-time code as the owner, then
exchange it:

```bash
a2a server pair --name me --role owner
```

```
→ code K7Q2-9XF4 (15 min, single use)
  caller: a2a pair add http://127.0.0.1:46757 K7Q2-9XF4
```

Run the printed command:

```bash
a2a pair add http://127.0.0.1:46757 K7Q2-9XF4
```

The token is saved in `~/.config/a2a-cli/credentials.json`, and every later
command for this URL attaches it. Set the agent card once so the commands
below stay short, and give the agent time to think:

```bash
cat > .env <<'EOF'
A2ACLI_AGENT_CARD=http://127.0.0.1:46757
A2ACLI_TIMEOUT=3m
EOF
```

## 4. Ask a question

```bash
a2a send "In one sentence, what can you do?"
```

## 5. Approve a tool call

Ask for something that runs a command:

```bash
a2a send "Use bash to write the output of 'uname -sr' into system.txt."
```

The agent asks first, so the task stops in `input-required`. The output shows
the tool call, the options (`once`, `always`, `reject`) and how to resume:

```
Resume:     a2a send --task-id <task-id> "<reply>"
```

Approve it on the same task:

```bash
a2a send --task-id <task-id> approve
```

The agent runs the command and finishes. `system.txt` is now in the workspace.

## 6. Continue the conversation

The context ID from the previous answer keeps the conversation going:

```bash
a2a send --context-id <context-id> "Which file did you just create?"
```

## 7. Let it call another agent (optional)

Start a second agent in **terminal C**:

```bash
mkdir -p ~/a2a-lesson5/reviewer && cd ~/a2a-lesson5/reviewer
a2a server opencode --name reviewer --secret ANTHROPIC_API_KEY
```

In **terminal B**, let `coder` call it: mint a code on the reviewer's side
and redeem it as a contact on coder's side.

```bash
cd ~/a2a-lesson5/reviewer
a2a server status                  # note reviewer's local URL
a2a server pair --name coder       # prints a code

cd ~/a2a-lesson5/coder
a2a server contact add reviewer --url <reviewer-url> --code <code>
```

Now ask coder to delegate:

```bash
a2a send "Ask the reviewer agent what files it sees in its workspace, and tell me."
```

Coder runs `a2a send -a http://a2a/<handle>/reviewer ...` in its sandbox. Its
server makes the call with the token it just stored, so no credential enters
the sandbox. If reviewer needs an approval, the request reaches you, not coder.

## 8. Clean up

```bash
cd ~/a2a-lesson5/coder && a2a server stop
cd ~/a2a-lesson5/reviewer && a2a server stop
a2a pair rm http://127.0.0.1:46757
```

Everything an agent keeps is in `.a2a/` in its workspace; delete the
directories to start over.

## Next steps

[Lesson 6](../06-tailscale-telegram-grok/) puts the agent on your tailnet and
in Telegram, and adds a Grok bot as a webhook node. The
[agent server guide](../../docs/agent-server.md) covers the rest, and the
[architecture](../../docs/agent-server-architecture.md) explains how the
pieces fit together.
