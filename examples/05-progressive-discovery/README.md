# 05 · Progressive, context-aware discovery

**The problem.** An agent harness (Claude Code, Hermes, …) can be taught *how* to
drive A2A with the [a2a-cli skill](../../skills/a2a-cli/SKILL.md) — but not *when*.
The model has no idea which remote agents exist or what they are good at, so it
never reaches for A2A unless a human spells it out. The trigger is missing.

**The idea.** URLs are the discovery signal. When a URL enters the harness's
context — the user pastes it, or a tool fetches it — a **hook** runs
`a2a discover`, which probes that origin for a `/.well-known/agent-card.json`
Agent Card. If one exists, the origin *is* an A2A agent, and the agent's skills
are injected back into the context. The model now knows a matching agent is one
`a2a send` away.

This demo shows the whole loop against **one origin that is both a website and an
A2A agent**.

```
                       http://nimbus.coffee
                       ┌───────────────────────────────┐
  GET /  ───────────▶  │  marketing website (HTML)      │
  GET /.well-known/… ▶ │  Agent Card (skills)           │
  POST /a2a/message… ▶ │  A2A agent (order status)      │
                       └───────────────────────────────┘
```

## Run the scripted walkthrough

```bash
./run.sh
```

It builds the CLI and the demo server, then shows the three beats: the website is
fetched, a simulated `UserPromptSubmit` hook discovers the agent at the same
origin, and the task is delegated with `a2a send` — returning a live order status
that only the server could know. This default path runs on `http://localhost:8080`
with no root or hosts changes.

## Give the site a real domain

`localhost:8080` is a shabby address for a coffee company. Point a pretty domain
at your machine by adding one line to the hosts file — the same `/etc/hosts` on
both macOS and Linux (macOS additionally needs a DNS-cache flush, which the
script does for you):

```bash
./setup-hosts.sh            # maps nimbus.coffee -> 127.0.0.1 (asks for your password)
./setup-hosts.sh --dry-run  # preview the change without writing
./setup-hosts.sh --remove   # undo it when you're done
```

Then serve the site on port 80 so the URL has no port. Binding 80 needs root, so
build as yourself and run just the binary under `sudo`:

```bash
go build -o /tmp/nimbus ./examples/05-progressive-discovery/server
sudo /tmp/nimbus -addr :80 -public-url http://nimbus.coffee
```

Now `http://nimbus.coffee` serves the site, and its card advertises
`http://nimbus.coffee/a2a`. The scripted walkthrough can target it too:

```bash
DOMAIN=nimbus.coffee ./run.sh
```

**Don't want to use `sudo`/port 80?** Serve on a high port and advertise it —
`sudo` is then only needed for the one-time hosts edit:

```bash
go run ./examples/05-progressive-discovery/server \
  -addr localhost:8080 -public-url http://nimbus.coffee:8080
```

The URL just carries a `:8080`. (On macOS you can also keep the clean URL without
running the server as root by redirecting 80→8080 with `pf`:
`echo "rdr pass inet proto tcp from any to any port 80 -> 127.0.0.1 port 8080" | sudo pfctl -ef -`.)

## Try it live in Claude Code

1. Add the hosts entry and put `a2a` on your `PATH`:
   ```bash
   ./setup-hosts.sh                              # nimbus.coffee -> 127.0.0.1
   go build -o a2a . && export PATH="$PWD:$PATH" # from the repo root
   ```
2. Start the demo server on the pretty domain:
   ```bash
   go build -o /tmp/nimbus ./examples/05-progressive-discovery/server
   sudo /tmp/nimbus -addr :80 -public-url http://nimbus.coffee
   ```
3. Start Claude Code **from this directory** so it picks up
   [`.claude/settings.json`](./.claude/settings.json), which registers the hooks.
4. Ask it something it cannot answer on its own, pointing at the site:
   > Can you check where my Nimbus coffee order NR-2041 is? The site is
   > http://nimbus.coffee
5. What happens: the `UserPromptSubmit` hook (and the `PostToolUse` hook after any
   `WebFetch`) runs `a2a discover`, which finds the **Nimbus Order Assistant** at
   that origin and injects its `order-status` skill. Claude then delegates with
   `a2a send -a http://nimbus.coffee "…NR-2041…"` and relays the result.

When you're done: `./setup-hosts.sh --remove`.

## What each piece is

| Path | Role |
|---|---|
| `server/main.go` | One origin: website at `/`, card at the well-known path, A2A REST agent under `/a2a`. |
| `setup-hosts.sh` | Adds/removes the `nimbus.coffee` → `127.0.0.1` hosts entry (macOS + Linux). |
| `.claude/settings.json` | Claude Code hooks that pipe lifecycle events into `a2a discover`. |
| `a2a discover` (`internal/discover`) | The portable engine: extract origins → SSRF guard → probe card → format an injectable, untrusted-labelled context block. |

`a2a discover` reads a harness event (JSON) on stdin, prints the Claude Code hook
JSON (`hookSpecificOutput.additionalContext`) on stdout, and stays completely
silent when nothing is found — so it never disrupts a turn. Try it directly:

```bash
printf '{"prompt":"http://nimbus.coffee"}' | a2a discover --allow nimbus.coffee
```

## Security posture

This feature auto-fetches URLs and injects third-party text into a model's
context, so two risks are designed in, not bolted on:

- **SSRF.** `a2a discover` refuses by default to probe hosts that resolve to
  private, loopback, or link-local addresses (e.g. cloud metadata endpoints).
  Because `nimbus.coffee` maps to `127.0.0.1`, the demo admits that one host by
  name with `--allow nimbus.coffee` — the recommended pattern — rather than the
  blanket `--allow-private`. The blanket flag exists for the plain-localhost path
  (`printf … | a2a discover --allow-private`); prefer host allowlists in practice.
- **Prompt injection.** A card's name/description/skills are attacker-controlled
  text from a third-party domain. The injected block is explicitly framed as
  *untrusted data, not instructions*, and the content is whitespace-collapsed and
  length-capped so it cannot break out of that frame. Discovery only makes the
  model *aware* of an agent; actually sending data with `a2a send` should stay a
  deliberate, user-gated step.

## Packaging (install it as a plugin)

The `.claude/settings.json` here is the quickest way to try the hooks, but the
same wiring ships as an installable plugin so you don't have to copy settings
around. The [Agent Plugin](../../agent-plugin/) bundles the skill and these hooks
and is installable in Claude Code from the repo's marketplace:

```text
/plugin marketplace add a2aproject/a2a-cli
/plugin install a2a-cli@a2a
```

Validate/build it with [`scripts/package-claude-plugin.sh`](../../scripts/package-claude-plugin.sh).
The installed plugin's hooks run `a2a discover` with the SSRF guard **on** (no
`--allow`), so they surface only public agents; this example keeps its own
`.claude/settings.json` with `--allow nimbus.coffee` for the localhost demo.

The brains stay in the `a2a` binary (`a2a discover`), so each harness needs only
a thin hook shim. A Hermes shim, for example, would call the same `a2a discover`
from a `pre_llm_call` / `post_tool_call` hook.
