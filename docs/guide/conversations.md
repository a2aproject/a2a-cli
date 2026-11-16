# Conversations

A2A groups messages in two ways. A **task** is one unit of work: it runs, may stop to ask you something, and ends. A **context** is a conversation: a series of tasks the agent treats as related.

| Command | What it does |
|---|---|
| `a2a send --task-id <id> "..."` | Answers a task that waits for input. |
| `a2a send --context-id <id> "..."` | Starts a new task in the same conversation. |
| `a2a chat` | Holds an interactive conversation. |

## Continue a conversation

Every result prints its context ID:

```text
Task:     01a1d530-7e27-79f6-ad7a-9bcf5482d7b0
Context:  01a1d530-7d81-7026-91c8-63d4c55f3481
Status:   completed (2026-11-13T13:54:47Z)
```

Send the next message in the same context, and the agent sees the earlier ones:

```bash
a2a send -a localhost:8080 --context-id 01a1d530-7d81-7026-91c8-63d4c55f3481 "And the tests for it?"
```

## Answer a question

A task in `input-required` waits for you. `a2a` prints the question and the command to answer it:

```text
Task:     01a1d530-540a-791c-a4be-890667492c6a
Context:  01a1d530-540a-79b9-a234-eae4bf4a02f2
Status:   input-required (2026-11-13T13:54:36Z)
  Permission required: git push (git push origin main).

Resume:     a2a send --task-id 01a1d530-540a-791c-a4be-890667492c6a "<reply>"
```

```bash
a2a send -a localhost:8080 --task-id 01a1d530-540a-791c-a4be-890667492c6a "once"
```

A task in `auth-required` needs credentials first; see [Credentials](credentials.md).

## Chat

`a2a chat` keeps the context for you, and sends your next line as the answer when a task asks for input:

```bash
a2a chat -a localhost:8080 --stream
```

```text
> What does this project do?
[status] working
[artifact] A command-line client for A2A agents.
[status] completed
> /exit
contextId: 01a1d531-0a3b-7c41-9d2e-6f1e2a3b4c5d
```

| Key | Does |
|---|---|
| `/exit`, `/quit`, Ctrl-D | Ends the chat and prints the context and task IDs. |
| Ctrl-C during a turn | Stops waiting and asks the agent to cancel the task. A second Ctrl-C stops waiting for the cancel. |
| Ctrl-C at the prompt | Ends the chat. |

Resume a chat with `a2a chat --context-id <id>`, adding `--task-id <id>` when a task waits for input. `chat` needs a terminal; scripts use `send`.
