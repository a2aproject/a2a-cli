#!/usr/bin/env bash
# Smoke-test the demo scripts without starting a server.
# --exec just pipes stdin -> stdout and checks the exit code, so testing the
# scripts on a plain pipe tests exactly what the server would run.
set -uo pipefail
cd "$(dirname "$0")"

fails=0
check() { # check <name> <expected-exit> <expected-substring> -- output actual-exit
  local name=$1 want_code=$2 want_text=$3 got_code=$5 out=$4
  if [[ "$got_code" != "$want_code" ]]; then
    echo "FAIL: $name — exit $got_code, want $want_code"; ((fails++)); return
  fi
  if [[ -n "$want_text" && "$out" != *"$want_text"* ]]; then
    echo "FAIL: $name — output missing '$want_text'"; ((fails++)); return
  fi
  echo "ok:   $name"
}

# content-generator.sh: uppercases and counts words, exit 0.
out=$(echo "hello world" | bash content-generator.sh); code=$?
check "bash: happy path" 0 "HELLO WORLD" "$out" "$code"

# content-generator.sh: empty input fails with exit 1.
out=$(echo "" | bash content-generator.sh 2>/dev/null); code=$?
check "bash: empty input fails" 1 "" "$out" "$code"

# a2a_unaware_agent.py: one numbered line per word, exit 0.
out=$(echo "one two" | python3 a2a_unaware_agent.py); code=$?
check "python: happy path" 0 "1. one" "$out" "$code"

# a2a_unaware_agent.py: empty input fails with exit 1.
out=$(echo "" | python3 a2a_unaware_agent.py 2>/dev/null); code=$?
check "python: empty input fails" 1 "" "$out" "$code"

echo
if ((fails)); then echo "$fails test(s) failed"; exit 1; fi
echo "all tests passed"
