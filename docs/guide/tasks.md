# Tasks

Every message you send starts or continues a task. The `task` commands look at tasks an agent keeps, follow them, and cancel them.

| Command | What it does |
|---|---|
| `a2a task get <id>` | Prints a task: status, artifacts, and with `--history n` its last messages. |
| `a2a task get <id> --wait` | Polls every `--poll-interval` until the task finishes or asks for input. |
| `a2a task list` | Lists tasks, with filters. |
| `a2a task subscribe <id>` | Streams the events of a running task. |
| `a2a task cancel <id>` | Asks the agent to cancel a task. |

## States

| State | Meaning |
|---|---|
| `submitted`, `working` | Accepted, or running. |
| `input-required` | Waiting for your answer. See [Conversations](conversations.md). |
| `auth-required` | Waiting for credentials. |
| `completed`, `failed`, `canceled`, `rejected` | Finished. |

## List and filter

```bash
a2a task list -a localhost:8080
a2a task list -a localhost:8080 --status input-required
a2a task list -a localhost:8080 --context <context-id> --since 2026-11-13T00:00:00Z --limit 20
```

```text
ID                                    STATUS          CONTEXT
01a1d530-540a-791c-a4be-890667492c6a  input-required  01a1d530-540a-79b9-a234-eae4bf4a02f2
01a1d530-53cf-74b7-a36c-af9ea1df605e  working         01a1d530-53cf-75ae-86d7-a936d61bd45e
01a1d530-5355-76ef-bac5-dbeda9bdf0d8  completed       01a1d530-5355-778d-9be0-4f020a4b0e84
```

When there are more, the output ends with `Next page token: ...`; pass it back with `--page-token`. `--with-artifacts` and `--history n` add each task's artifacts and messages to `-o json`.

## Cancel

```bash
a2a task cancel -a localhost:8080 01a1d530-53cf-74b7-a36c-af9ea1df605e
```

A finished task can't be canceled; the agent returns an error.

## What agents support

Agents decide which of these they serve and to whom. An agent may keep tasks only while they run, show each caller only their own tasks, or refuse `task list` to callers it doesn't know. An unsupported call fails with the agent's error and exit code 1.

The [task reference](../reference/commands/task.md) lists every flag.
