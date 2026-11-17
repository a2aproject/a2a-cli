# Lesson 4: Conversations

Every message so far started something new: a fresh task in a fresh conversation. Real work goes back and forth. In this lesson you continue a conversation, answer an agent that stops to ask you something, and hold a conversation with `a2a chat`.

![Continuing a conversation with --context-id, approving a tool call with --task-id, and chatting](demo.gif)

## What you'll learn

- The difference between a task and a context
- How to send a follow-up in the same conversation
- What `input-required` means, and how to answer it
- How `a2a chat` does both for you

## Prerequisites

The agent from [lesson 1](../01-first-message/README.md), and the `.env` in `~/a2a-lessons` that points at it.

## Step 1: Tasks and contexts

A **task** is one piece of work. A **context** is the conversation it belongs to. Every result shows both:

```bash
a2a send "Remember this: the release is called Juniper."
```

```text
Task:     01a1d530-7d81-71e2-8d4c-1f0b2c3d4e5f
Context:  01a1d530-7d81-7026-91c8-63d4c55f3481
Status:   completed (2026-11-13T13:54:47Z)
Artifacts:
  [response] Noted: the release is called Juniper.
```

A new `send` starts a new context, and the agent doesn't see the earlier one. Ask without the context and it won't know:

```bash
a2a send "What is the release called?"
```

## Step 2: Continue the conversation

Send the follow-up with the context ID, and the agent answers with the earlier messages in view:

```bash
a2a send --context-id 01a1d530-7d81-7026-91c8-63d4c55f3481 "What is the release called?"
```

```text
Task:     01a1d530-7e27-79f6-ad7a-9bcf5482d7b0
Context:  01a1d530-7d81-7026-91c8-63d4c55f3481
Status:   completed (2026-11-13T13:54:47Z)
Artifacts:
  [response] Juniper.
```

The context is the same, and the task is new: each message you send in a context starts its own task.

## Step 3: Answer a question

Sometimes the agent can't finish without you. a2a-serve asks before the agent runs a command, and the task stops in `input-required`:

```bash
a2a send "Run whoami and tell me the user name."
```

```text
Task:     01a1d530-540a-791c-a4be-890667492c6a
Context:  01a1d530-540a-79b9-a234-eae4bf4a02f2
Status:   input-required (2026-11-13T13:54:36Z)
  Permission required: bash (whoami).
Reply with one of: once, always (allows bash whoami * in this conversation), reject. ...

Resume:     a2a send --task-id 01a1d530-540a-791c-a4be-890667492c6a "<reply>"
```

The status message is the agent's question, and `Resume` is the command to answer it. This time you continue the **task**, so the answer goes to the question:

```bash
a2a send --task-id 01a1d530-540a-791c-a4be-890667492c6a once
```

```text
Task:     01a1d530-540a-791c-a4be-890667492c6a
Context:  01a1d530-540a-79b9-a234-eae4bf4a02f2
Status:   completed (2026-11-13T13:54:48Z)
Artifacts:
  [response] The user name is you.
```

`once` allows this one command. `reject` refuses it, and the agent carries on without it.

Agents can also stop in `auth-required`, when they need credentials you haven't given. Lesson 5 covers credentials.

## Step 4: Chat

Tracking IDs by hand gets old. `a2a chat` keeps the context between your messages, and sends your next line as the answer when the agent asks something:

```bash
a2a chat --stream
```

```text
> Remember this: my favorite color is teal.
[status] working
[artifact] Got it.
[status] completed
> What is my favorite color?
[status] working
[artifact] Teal.
[status] completed
> /exit
contextId: 01a1d531-0a3b-7c41-9d2e-6f1e2a3b4c5d
```

`/exit`, `/quit` or Ctrl-D end the chat and print the IDs. Ctrl-C while the agent works cancels the task and returns to the prompt. To pick the conversation up later, pass the printed ID back:

```bash
a2a chat --context-id 01a1d531-0a3b-7c41-9d2e-6f1e2a3b4c5d
```

`chat` needs a terminal. In scripts, use `send` with `--context-id` and `--task-id`.

## Recap

- A context is a conversation; a task is one piece of work in it.
- `--context-id` starts a new task in an existing conversation.
- `--task-id` continues a task, such as one waiting in `input-required`.
- `a2a chat` handles both for you.

## Next

[Lesson 5](../05-credentials/README.md) looks at how `a2a` proved who you were in every request so far.
