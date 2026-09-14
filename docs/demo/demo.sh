#!/usr/bin/env bash
#
# Drives the terminal recording for the README demo GIF.
#
# It stands up two throwaway A2A servers from ordinary scripts (--exec), then
# discovers and talks to them with the a2a CLI. Nothing here is A2A-specific
# code; that is the point of the demo.
#
# The prompt STYLE (agnoster | robbyrussell) is baked into the recording; the
# color palette is chosen at render time with `agg --theme`. Agnoster needs a
# Nerd Font for its powerline glyphs.
#
# Record and render (run from the repo root, with `a2a` on PATH):
#   STYLE=agnoster asciinema rec --cols 92 --rows 26 \
#     --command "bash docs/demo/demo.sh" docs/demo/a2a-demo.cast
#   agg --theme kanagawa --bold-is-bright --idle-time-limit 2.5 \
#     --font-dir <nerd-font-dir> --font-family "JetBrainsMono Nerd Font" \
#     docs/demo/a2a-demo.cast docs/demo/a2a-demo.gif
#
# Requires: a2a on PATH, python3, bash. Run from the repo root.

set -euo pipefail

EX="examples/01-exec-demo"
STYLE="${STYLE:-agnoster}"

esc=$'\033'
r="${esc}[0m"
# Bright/pastel yellow for comments — clearly legible on any dark theme.
c_comment="${esc}[93m"

case "$STYLE" in
  agnoster)
    sep=$'\ue0b0'          # powerline separator (Nerd Font)
    branch=$'\ue0a0'       # git branch glyph (Nerd Font)
    # dir segment (blue bg) -> git segment (green bg), agnoster style.
    PROMPT="${esc}[44;1;37m a2a-cli ${esc}[0;34;42m${sep}${esc}[42;30m ${branch} main ${esc}[0;32m${sep}${r} "
    ;;
  robbyrussell)
    PROMPT="${esc}[1;32m➜${r}  ${esc}[36ma2a-cli${r} ${esc}[34mgit:(${esc}[31mmain${esc}[34m)${r} "
    ;;
  *)
    echo "unknown STYLE: $STYLE" >&2
    exit 1
    ;;
esac

TYPE_DELAY=0.045   # per-keystroke typing speed
AFTER_CMD=2.0      # pause to read command output
AFTER_NOTE=1.4     # pause to read a comment

type_out() {
  printf '%s' "$1" | while IFS= read -r -n1 ch; do
    printf '%s' "$ch"
    sleep "$TYPE_DELAY"
  done
}

# Print a command at the prompt as if typed, run it, then leave a blank line.
run() {
  printf '%s' "$PROMPT"
  type_out "$*"
  printf '\n'
  sleep 0.4
  eval "$*"
  printf '\n'
  sleep "$AFTER_CMD"
}

# Print a comment at the prompt (prompt-prefixed, bright, not executed).
note() {
  printf '%s' "$PROMPT"
  printf '%s' "$c_comment"
  type_out "# $*"
  printf '%s\n' "$r"
  sleep "$AFTER_NOTE"
}

cleanup() {
  [[ -n "${SRV_A:-}" ]] && kill "$SRV_A" 2>/dev/null || true
  [[ -n "${SRV_B:-}" ]] && kill "$SRV_B" 2>/dev/null || true
}
trap cleanup EXIT

clear
note "The a2a CLI: one command surface for any A2A agent."
run "a2a version"

note "Turn an ordinary script into an A2A server -- no A2A code."
note "see cookbook lesson 01 to learn about --echo & --exec options"
a2a server --exec "bash $EX/content-generator.sh" --port 8080 >/tmp/a2a-demo-a.log 2>&1 &
SRV_A=$!
a2a server --exec "python3 -u $EX/a2a_unaware_agent.py" --chunk=$'\n' --port 8081 >/tmp/a2a-demo-b.log 2>&1 &
SRV_B=$!
sleep 2.5

note "Discover what an agent can do -- read its card."
run "a2a card get -a http://localhost:8080"

note "Send a message and wait for the result."
run 'a2a send -a http://localhost:8080 "hello world from A2A"'

note "Stream a reply piece by piece as it arrives."
run 'a2a send -a http://localhost:8081 --stream "one two three four"'

note "That is a full A2A round trip. Learn more: examples/"
sleep 1.5
