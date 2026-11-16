# Recording demos

`tools/demo/record.sh` plays a scenario from `tools/demo/scenarios/` in tmux panes and records it with asciinema. It types each command at human speed, waits for the shell prompt, and reads task IDs from the screen for the next command. `compose.sh` renders the recording as a GIF in the documentation site's colors and font.

Each lesson and example embeds its demo as `demo.gif` next to its README, and the home page uses `docs/assets/demo.gif`. Each run uses its own `HOME` under `/tmp/a2a-demo/you`, so it starts with no configuration and leaves yours alone.

## Set up once

```bash
brew install asciinema tmux jq go    # or your package manager
```

- `a2a` is built from this checkout on every run.
- `a2a-serve` comes from your `PATH`, or is built from a checkout of [a2a-serve](https://github.com/a2aproject/a2a-serve) at `../a2a-serve` (`A2A_SERVE_SRC` changes the path).
- `--gif` fetches [agg](https://github.com/asciinema/agg) and the Geist Mono font into `~/.cache/a2a-cli-demo` the first time.
- asciinema 3 records without a terminal. asciinema 2 works too; `record.sh` runs it under `script` to give it one.

## Record

```bash
tools/demo/record.sh list                                # scenarios
tools/demo/record.sh 01-first-message --fake --no-record # play it without recording, as CI does
tools/demo/record.sh 01-first-message --fake --gif       # record and render tools/demo/out/01-first-message.gif
```

`--fake` answers with a2a-serve's scripted test agent, so a run needs no Docker or model key and gives the same replies every time. The published GIFs are recorded with `--fake`. Without it, the lessons' agent runs opencode with `OPENROUTER_API_KEY`, as the lessons describe.

Other flags: `--speed 2` types and pauses twice as fast, `--size 120x32` sets the terminal, `--gif-speed` the GIF's playback speed, and `--keep` keeps `/tmp/a2a-demo` for debugging. Watch a run live with `tmux -L a2a-demo attach -r`.

Copy a reviewed GIF next to its README:

| Scenario | GIF |
|---|---|
| `home` | `docs/assets/demo.gif` |
| `01-first-message` ... `05-credentials` | `lessons/<scenario>/demo.gif` |
| `scripting-with-jq`, `credential-helper`, `a2a-hello`, `a2a-transport-echo` | `examples/<scenario>/demo.gif` |

Re-render a cast without recording it again with `tools/demo/compose.sh <name>.cast <out.gif>`.

## Write or fix a scenario

A scenario is a bash file that `record.sh` sources. It must run on bash 3.2, the macOS default. It defines:

- `layout`: creates the panes with `panes`, `split_pane` and `title`.
- `scene`: the recorded steps.
- `fakes` (optional): the scripted agent's replies, with `answer_for` and `behave_as`.
- `prepare` (optional): setup before the recording starts, such as `lesson_agent`.
- `teardown` (optional): runs after the agents stop.

Type the commands of the README as written, in its order, so the demo matches the docs. When a README changes, update its scenario: CI plays every scenario with `--fake --no-record` and fails when one no longer works. Check the results that matter with `expect_status` and `wait_text`.

| Helper | Does |
|---|---|
| `panes <name>` | Creates the session with its first pane. |
| `split_pane <new> <from> above\|below\|left\|right <pct>` | Adds a pane next to another. |
| `title <pane> <text>` | Sets the label on the pane border. |
| `run <pane> <cmd> [timeout]` | Types the command, runs it and waits for the prompt. |
| `paste_run <pane> <cmd> [timeout]` | Pastes a long command at once, runs it and waits for the prompt. |
| `start <pane> <cmd>` | Types and runs a command that keeps running, such as `a2a-serve start`. |
| `say <pane> <text>` | Types a `# text` comment as narration. |
| `wait_text <pane> <ERE> [timeout]` | Waits until the pane shows a match. |
| `last_match <pane> <ERE>` | Prints the last match on the pane, to reuse a task ID. |
| `expect_status <pane> <status>` | Fails the run unless the pane's last task printed that status, such as `completed`. |
| `clear_pane`, `interrupt`, `pause <s>` | Ctrl-L, Ctrl-C, a pause scaled by `--speed`. |
| `until_ok <timeout> <cmd...>` | Retries a command off camera. |
| `lesson_agent` | Starts the lessons' agent in `~/a2a-lessons/agent` and writes `~/a2a-lessons/.env`, as lesson 1 leaves them. |
| `answer_for <prompt> <text>` | In `--fake` mode, makes the agent answer the prompt with the text. |
| `behave_as <prompt> <command>` | In `--fake` mode, makes the agent handle the prompt as one of its scripted commands. |

The scripted agent's commands are in `internal/agentsrv/testdata/fakeacp/main.go` of a2a-serve. The useful ones: `tool` reports a tool call, `whoami` asks for permission and stops in `input-required`, `slow 3s` streams dots for three seconds, `stall` runs until canceled, `remember <x>` and `recall` keep a value in a conversation, and `read <file>` answers with a file's contents. Any other prompt gets `Hello from fakeacp.`

## Review before publishing

- Play it: `asciinema play tools/demo/out/<scenario>.cast`, or open the GIF.
- `record.sh` deletes a cast that shows the value of a model key. It can't see other secrets; check for them.

## Troubleshooting

| Symptom | Fix |
|---|---|
| `a demo is already running` | A run is still going, or one crashed. `tmux -L a2a-demo kill-server`. |
| `/tmp/a2a-demo exists` | Stop agents left in it with `a2a-serve stop --dir <dir>`, then remove it. |
| `no prompt after 600s` | The command hung or asked for input. The pane contents print on failure. |
| `no a2a-serve checkout` | Install `a2a-serve`, or set `A2A_SERVE_SRC` to a checkout. `--fake` always needs the checkout. |
