# Files

Messages and artifacts can carry files, as bytes or as URLs. `a2a` sends local files inline, and can save the files an agent returns.

## Send a file

```bash
a2a send -a localhost:8080 --text-part "Summarize this" --file-part ./report.pdf
a2a send -a localhost:8080 --file-part https://example.com/spec.pdf --media-type application/pdf
```

A local file goes with its bytes, base name and a media type guessed from its extension; `--media-type` right after it overrides the guess. A URL goes as a reference, and the agent fetches it if it can. See [Messages](messages.md).

## Save returned files

Text output shows a file as `file: name (type, size)` or with its URL. `--save-fileparts <dir>` also writes every file that arrives as bytes into a directory:

```bash
a2a send -a localhost:8080 --save-fileparts ./out "Draw the architecture as a PNG"
a2a task get -a localhost:8080 <task-id> --save-fileparts ./out
a2a task subscribe -a localhost:8080 <task-id> --save-fileparts ./out
```

`--verbose` prints each saved path. The directory is created when the first file arrives.

| Rule | |
|---|---|
| Name | The part's file name, reduced to its base name. Without one, the artifact's name or ID, or `status-msg` for a file in a status message. |
| Extension | Added from the media type when the name has none, for common types such as `.png`, `.pdf` and `.json`. |
| Streaming | Chunks appended to an artifact are appended to its file. |
| Same name | A file from another artifact with the same name gets a ` (1)`, ` (2)` suffix. An existing file from an earlier run is overwritten. |

Files the agent sends as URLs aren't downloaded; `-o json` shows their URLs.
