#!/usr/bin/env bash
# Starts the three local agents this example uses, on ports 9101-9103. Each one
# answers with a fixed sentence built from the message. Stop them with Ctrl-C.
set -euo pipefail

a2a-serve start exec --port 9101 --no-traffic-log --name Weather --description "Current weather for any city" \
  'echo "Sunny and 21°C in $(cat)."' &
a2a-serve start exec --port 9102 --no-traffic-log --name "Travel Desk" --description "Books flights and hotels within Acme's travel policy" \
  'echo "Booked: $(cat). Confirmation TD-4821."' &
a2a-serve start exec --port 9103 --no-traffic-log --name Expenses --description "Files expense reports and answers questions about reimbursements" \
  'echo "Filed: $(cat). Report EX-311."' &

trap 'kill $(jobs -p) 2>/dev/null' EXIT
echo "agents on http://127.0.0.1:9101 (Weather), :9102 (Travel Desk), :9103 (Expenses)"
wait
