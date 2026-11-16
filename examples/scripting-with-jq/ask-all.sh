#!/usr/bin/env bash
# Sends every line of a questions file to an agent as its own task, waits for
# all of them, and prints each question with the task's final state and answer.
#
#   ./ask-all.sh questions.txt
#
# The agent comes from --agent-card in A2ACLI_AGENT_CARD or a .env file.
set -euo pipefail

questions="${1:?usage: ask-all.sh <questions-file>}"
tasks="$(mktemp)"
trap 'rm -f "$tasks"' EXIT

while IFS= read -r question; do
  [[ -n "$question" ]] || continue
  id="$(a2a send --async -o json "$question" | jq -r '.task.id')"
  printf '%s\t%s\n' "$id" "$question" >>"$tasks"
done <"$questions"

failed=0
while IFS=$'\t' read -r id question; do
  result="$(a2a task get "$id" --wait --timeout 5m -o json)"
  state="$(jq -r '.task.status.state | ascii_downcase | sub("task_state_"; "")' <<<"$result")"
  answer="$(jq -r '[.task.artifacts[]?.parts[]?.text // empty] | join(" ")' <<<"$result")"
  printf '%-10s %s\n           %s\n' "$state" "$question" "$answer"
  [[ "$state" == completed ]] || failed=$((failed + 1))
done <"$tasks"

exit $((failed > 0))
