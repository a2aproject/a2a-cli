# a2a chat

<!-- Generated from the command definitions; do not edit. -->

Start an interactive chat session with an agent. Chat reuses the same context ID across turns and continues tasks that require input.

Controls:

```
/exit, /quit, Ctrl-D   Exit and print the context and task IDs to resume later
Ctrl-C                 Cancel an active turn (and its agent task), or exit at the prompt
```

Chat requires a terminal; use send to send a single message. Use chat --context-id and --task-id to resume a conversation.

```
a2a chat [message] [flags]
```

**Examples**

```bash
a2a chat -a https://agent.example.com
a2a chat -a https://agent.example.com --stream "Hi, what can you do?"
a2a chat -a https://agent.example.com --context-id <context-id>
```

**Flags**

| Flag | Default | Description |
|---|---|---|
| `--context-id string` |  | Context ID to continue an existing conversation |
| `--poll-interval duration` | `2s` | Interval between GetTask requests when polling |
| `--stream` |  | Stream responses (falls back to polling if unsupported) |
| `--task-id string` |  | Task ID to continue an existing task |

The [global flags](a2a.md#global-flags) apply too.
