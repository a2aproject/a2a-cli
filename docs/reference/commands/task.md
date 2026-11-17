# a2a task

<!-- Generated from the command definitions; do not edit. -->

Work with tasks

```
a2a task <command>
```

The [global flags](a2a.md#global-flags) apply too.

## a2a task cancel

Cancel a task

```
a2a task cancel <task-id> [flags]
```

**Examples**

```bash
a2a task cancel -a https://agent.example.com <task-id>
```

**Flags**

| Flag | Default | Description |
|---|---|---|
| `--metadata json` |  | Attach request metadata as a JSON object (repeatable) |

## a2a task get

Print a task: its status, artifacts and, with --history, its messages.

--wait polls until the task finishes or needs input. --timeout bounds the
whole wait.

```
a2a task get <task-id> [flags]
```

**Examples**

```bash
a2a task get -a https://agent.example.com <task-id>
a2a task get -a https://agent.example.com <task-id> --history 10 -o json
a2a task get -a https://agent.example.com <task-id> --wait --timeout 5m
```

**Flags**

| Flag | Default | Description |
|---|---|---|
| `--history int` |  | Include up to n history messages |
| `--poll-interval duration` | `2s` | Duration between polls while waiting; only used with --wait. Overall wait budget is --timeout. |
| `--save-fileparts string` |  | A directory for storing raw-bytes message and artifact parts as files. |
| `--wait` |  | Poll until the task reaches a terminal or interrupted (input/auth-required) state |

## a2a task list

List tasks

```
a2a task list [flags]
```

**Examples**

```bash
a2a task list -a https://agent.example.com
a2a task list -a https://agent.example.com --status input-required
a2a task list -a https://agent.example.com --context <context-id> --limit 20
```

**Flags**

| Flag | Default | Description |
|---|---|---|
| `--context string` |  | Filter by context ID |
| `--history int` |  | Include up to n history messages per task |
| `--limit int` |  | Page size |
| `--page-token string` |  | Pagination token |
| `--since string` |  | Only tasks with status updates after this timestamp (RFC 3339) |
| `--status string` |  | Filter by task state |
| `--with-artifacts` |  | Include artifacts in the response |

## a2a task push-config

Manage task push-notification configurations

```
a2a task push-config <command>
```

Aliases: `push`

## a2a task push-config create

Create a push-notification configuration for a task

```
a2a task push-config create <task-id> [flags]
```

**Examples**

```bash
a2a task push-config create -a https://agent.example.com <task-id> --url https://hooks.example.com/a2a
a2a task push-config create -a https://agent.example.com <task-id> --url https://hooks.example.com/a2a \
  --auth-scheme Bearer --auth-credentials "$HOOK_TOKEN"
```

**Flags**

| Flag | Default | Description |
|---|---|---|
| `--auth-credentials string` |  | Optional credentials the agent presents to the webhook |
| `--auth-scheme string` |  | Optional auth scheme the agent uses when calling the webhook (e.g. Bearer) |
| `--id string` |  | Optional client-set configuration ID (allows multiple callbacks) |
| `--token string` |  | Optional token the agent echoes back so the receiver can validate calls |
| `--url string` |  | Webhook callback URL the agent posts updates to (required) |

## a2a task push-config delete

Delete a task's push-notification configuration

```
a2a task push-config delete <task-id> <config-id>
```

**Examples**

```bash
a2a task push-config delete -a https://agent.example.com <task-id> <config-id>
```

## a2a task push-config get

Get a task's push-notification configuration

```
a2a task push-config get <task-id> <config-id>
```

**Examples**

```bash
a2a task push-config get -a https://agent.example.com <task-id> <config-id>
```

## a2a task push-config list

List a task's push-notification configurations

```
a2a task push-config list <task-id> [flags]
```

**Examples**

```bash
a2a task push-config list -a https://agent.example.com <task-id>
```

**Flags**

| Flag | Default | Description |
|---|---|---|
| `--limit int` |  | Page size |
| `--page-token string` |  | Pagination token |

## a2a task subscribe

Stream the events of a running task until it finishes or needs input.

--timeout bounds the time between two events, so a task that keeps
sending events is followed for as long as it runs.

```
a2a task subscribe <task-id> [flags]
```

**Examples**

```bash
a2a task subscribe -a https://agent.example.com <task-id>
a2a task subscribe -a https://agent.example.com <task-id> -o jsonl
```

**Flags**

| Flag | Default | Description |
|---|---|---|
| `--save-fileparts string` |  | A directory for storing raw-bytes message and artifact parts as files. |
