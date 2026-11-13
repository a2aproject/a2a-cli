# Scripting with jq

This example sends a list of questions to an agent as separate tasks, so the agent works on them at once, then waits for each and prints its state and answer. It uses `--async` to start tasks, `task get --wait` to collect them, `-o json` with `jq` to read results, and the exit code to report failures.

![Running ask-all.sh: three tasks started, collected and printed with their states](demo.gif)

## Prerequisites

- `a2a` and [jq](https://jqlang.org).
- An agent. The [lessons](../../lessons/README.md#what-you-need) start one with a2a-serve; set its URL:

  ```bash
  export A2ACLI_AGENT_CARD="$(a2a-serve status --dir ~/a2a-lessons/agent --url=local)"
  ```

## Run it

```bash
./ask-all.sh questions.txt
```

```text
completed  Which Go version does this project use?
           Go 1.26, from go.mod.
completed  Name the largest file in the workspace.
           go.sum, at 31 KB.
completed  Is there a test that takes longer than a second?
           No; the slowest, TestStream, takes 0.4s.
```

The script exits 1 when any task ends in a state other than `completed`.

## How it works

[`ask-all.sh`](ask-all.sh) starts one task per line, keeping each task's ID:

```bash
id="$(a2a send --async -o json "$question" | jq -r '.task.id')"
```

`--async` returns as soon as the agent accepts the task, and `-o json` prints it as `{"task": {...}}`, so `.task.id` is the ID. Then it waits for each task and reads its state and the text of its artifacts:

```bash
result="$(a2a task get "$id" --wait --timeout 5m -o json)"
state="$(jq -r '.task.status.state | ascii_downcase | sub("task_state_"; "")' <<<"$result")"
answer="$(jq -r '[.task.artifacts[]?.parts[]?.text // empty] | join(" ")' <<<"$result")"
```

States arrive as protocol names such as `TASK_STATE_COMPLETED`; the `jq` filter shortens them. A task can end `failed` or `input-required` while `a2a` exits 0, because the call itself succeeded, so the script checks the state.

## Adapt it

- Send files with each question: add `--file-part "$file"` to the `send`.
- Stream progress for one task: `a2a send --stream -o jsonl "..." | jq -r 'select(.artifactUpdate) | .artifactUpdate.artifact.parts[].text // empty'`.
- Tell failures apart: exit code 3 means the agent was unreachable, 5 a timeout. See [Errors and exit codes](../../docs/reference/errors.md).

[Scripting](../../docs/guide/scripting.md) covers the patterns, and [Output](../../docs/reference/output.md) the JSON shapes.
