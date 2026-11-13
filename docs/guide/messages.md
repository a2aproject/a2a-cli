# Messages

A message is a list of parts: text, files and structured data. `a2a send` builds one from its arguments, in the order you give them.

| Flag | Adds |
|---|---|
| `"text"` (positional) | A text part, placed first. At most one. |
| `--text-part <text>` | A text part. |
| `--file-part <path>` | A file part with the file's bytes, name and media type. |
| `--file-part <url>` | A file part that points at the URL. `a2a` doesn't fetch it. |
| `--data-part <path\|json>` | A JSON data part, from a file or an inline JSON string. |
| `--media-type <type>` | Sets the media type of the part flag just before it. |

Every part flag can repeat.

```bash
a2a send -a localhost:8080 \
  --text-part "Review this change" \
  --file-part ./diff.patch --media-type text/x-diff \
  --data-part '{"priority": "high"}'
```

## Files

A local path is read and sent inline, with its base name as the file name and a media type guessed from the extension. `http`, `https`, `s3`, `gs`, `ftp` and `ftps` URLs are sent as references. What an agent accepts depends on the agent: its card lists `defaultInputModes`, and an agent may reject a file type it can't handle. [Files](files.md) also covers saving the files an agent sends back.

## Data

`--data-part` takes a path to a JSON file, or the JSON itself when no such file exists:

```bash
a2a send -a localhost:8080 --data-part order.json "Process this order"
a2a send -a localhost:8080 --data-part '{"id": 42, "items": 3}' "Process this order"
```

## Metadata

`--metadata` attaches a JSON object to the request. Repeated flags merge, later keys win:

```bash
a2a send -a localhost:8080 --metadata '{"trace": "abc"}' "hello"
```

## The whole request

`--request-payload` sends a complete `SendMessageRequest`, from a file or inline JSON. It can't be combined with the part flags, `--task-id`, `--context-id`, `--metadata`, `--history` or `--async`. A missing message ID is generated.

```bash
a2a send -a localhost:8080 --request-payload '{"message": {"role": "ROLE_USER", "parts": [{"text": "hello"}]}}'
```

The [send reference](../reference/commands/send.md) lists every flag.
