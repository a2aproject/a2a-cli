# Serve a coding agent harness

```sh
a2a server opencode --workspace ~/agent-home --port 8080   # or --bin <path to an opencode fork>
a2a server claude --workspace ~/agent-home --port 8080
a2a send -a http://127.0.0.1:8080 "What's in this directory?"
```

Both commands take the same server flags as `echo` and `exec` (`--port`,
`--host`, `--name`, `--serve-transport`, `--advertise-address`, ...), plus:

| Flag | Meaning |
| --- | --- |
| `--workspace` | Working directory of the harness. |
| `--state-dir` | Where the A2A context → harness session map is kept, so conversations survive a restart. |
| `--bin` | Harness executable. |
| `--approve ask\|allow\|deny` | Default decision for permission requests. `ask` pauses the task in `input-required` until the caller answers. |
| `--allow-tools`, `--deny-tools` | Tool name globs decided without asking. |
| `--harness-arg` | Extra harness argument, e.g. `--harness-arg --pure` to skip opencode plugins. |

How harness events map to A2A:

| Harness | A2A |
| --- | --- |
| Assistant text | `response` artifact, appended as it streams |
| Tool call started, finished or failed | `working` status update with a text summary and a data part |
| Permission request, or opencode's `question` tool | `input-required` with a text prompt and a data part `{"a2aHarnessQuestion": {...}}` |
| Turn finished | `completed`, or `failed` with the harness error |
| A2A cancel | opencode session abort, or killing the claude process |
| A2A context | opencode or Claude Code session |

A caller answers `input-required` by sending a message with the same task ID:
an option ID or label (`once`, `always`, `reject`), yes/no, free text for
questions, or a data part `{"answer": "once"}`.

- **opencode.** Sessions are created with `ask` rules for the permissions in
  `--ask` (default `bash,edit,webfetch,external_directory`). opencode blocks
  the tool call, the task pauses, and the answer resumes the same turn.
- **Claude Code.** Headless Claude denies tools outside `--allowed-tools` and
  reports them. The executor turns the denials into `input-required`. On
  approval, it resumes the session with the exact calls allowed and asks Claude
  to retry.

A harness started this way loads its user-level config, including MCP servers
and skills.