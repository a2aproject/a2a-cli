#!/usr/bin/env bash
# An A2A-unaware agent. The a2a CLI (server --exec) feeds the incoming
# message text on stdin and turns whatever we print on stdout into the
# response artifact. Exit 0 => completed, non-zero => failed.
set -euo pipefail

# Read the whole message from stdin.
message="$(cat)"

if [[ -z "${message// }" ]]; then
  echo "error: empty message" >&2   # stderr is logged; shows up in the failure status
  exit 1
fi

# Do the "work". Here: shout it back with a word count.
words=$(echo "$message" | wc -w | tr -d ' ')
echo "You said (${words} words): ${message^^}"
