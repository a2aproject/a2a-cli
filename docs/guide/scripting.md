# Scripting

`a2a` prints protocol objects as JSON for scripts, and exits with codes that tell failures apart.

| Flag | Prints |
|---|---|
| `-o text` | Text for people. The default. |
| `-o json` | Indented JSON: one document, or one document per event with `--stream`. |
| `-o jsonl` | One compact JSON object per line. The easiest to pipe. |

## Read a result

`send`, `task get` and each streamed event print an object with one key that names what it holds: `task`, `message`, `statusUpdate` or `artifactUpdate`.

```bash
result="$(a2a send -a localhost:8080 -o json "Name three prime numbers")"
jq -r '.task.status.state' <<<"$result"                       # TASK_STATE_COMPLETED
jq -r '.task.artifacts[].parts[].text // empty' <<<"$result"  # 2, 3 and 5.
```

States are spelled as in the protocol, such as `TASK_STATE_COMPLETED` and `TASK_STATE_INPUT_REQUIRED`.

## Check the task, not only the exit code

A task that ends `failed`, `canceled`, `rejected` or `input-required` is still a successful call, and `a2a` exits 0. Check the state:

```bash
state="$(jq -r '.task.status.state' <<<"$result")"
[[ "$state" == TASK_STATE_COMPLETED ]] || { echo "task ended $state" >&2; exit 1; }
```

## Exit codes

| Exit | Means |
|---|---|
| 0 | The call succeeded. |
| 1 | The agent returned an error, or something else failed. |
| 2 | Wrong flags or arguments. |
| 3 | The agent couldn't be reached. |
| 4 | The agent card is missing or invalid. |
| 5 | `--timeout` expired. |

With `-o json`, an error prints `{"error": {"code": ..., "message": ..., "hint": ...}}` on stdout. [Errors and exit codes](../reference/errors.md) lists the codes.

## Follow a stream

```bash
a2a send -a localhost:8080 --stream -o jsonl "Run the tests" |
  jq -r 'select(.artifactUpdate) | .artifactUpdate.artifact.parts[].text // empty'
```

## Fan out

```bash
for f in src/*.go; do
  a2a send -a localhost:8080 --async -o json --file-part "$f" "Review this file" |
    jq -r '.task.id'
done > tasks.txt

while read -r id; do
  a2a task get -a localhost:8080 "$id" --wait --timeout 10m -o json | jq -r '.task.status.state'
done < tasks.txt
```

[Scripting with jq](../../examples/scripting-with-jq/README.md) is a complete, recorded example, and [Output](../reference/output.md) describes every format.
