#!/usr/bin/env bash
# Runs a local "Release Notes" agent on port 9201. It turns the feat and fix
# lines of the change list in each message into release notes, without a model,
# so the example gives the same answer every run. Stop it with Ctrl-C.
set -euo pipefail

exec a2a server --port 9201 --quiet --name "Release Notes" \
  --description "Writes user-facing release notes from a list of commits, in the team's house style" \
  --exec "awk -f '$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)/release-notes.awk'"
