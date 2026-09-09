#!/usr/bin/env python3
"""An A2A-unaware agent.

`a2a server --exec` pipes the incoming message text to stdin and turns
stdout into the response artifact. Exit 0 => completed, non-zero => failed.
stderr is logged by the CLI (and attached to the failure status on error).

Run streaming chunks with:  a2a server --exec "python3 -u a2a_unaware_agent.py" --chunk=$'\n'
Use `python3 -u` so stdout is unbuffered and chunks stream promptly.
"""

import sys
import time


def main() -> int:
    message = sys.stdin.read().strip()
    if not message:
        print("error: empty message", file=sys.stderr)
        return 1

    # Do the "work". Here: stream one line per word so --chunk can split on \n.
    for i, word in enumerate(message.split(), start=1):
        print(f"{i}. {word}")
        sys.stdout.flush()
        time.sleep(0.3)  # visible streaming when run with --chunk=$'\n'
    return 0


if __name__ == "__main__":
    sys.exit(main())
