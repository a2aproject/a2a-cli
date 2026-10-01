#!/usr/bin/env bash
#
# Drives the SPLIT-SCREEN terminal recording for the README demo GIF.
#
# Two tmux panes, side by side:
#   left  -- stand up an A2A server from an ordinary script (--exec)
#   right -- discover and talk to it with the a2a CLI
#
# The point of the split is that you watch one side become a server while the
# other side drives it as a client. Nothing here is A2A-specific code.
#
# It runs on a PRIVATE tmux socket (-L / -f /dev/null) so it ignores your
# ~/.tmux.conf and inherits the a2a binary that run.sh puts on PATH. The pane
# size is taken from the recording terminal, so the session matches the GIF.
#
# Record and render it through the shared runner (run from the repo root, with
# `a2a` on PATH):
#
#   DEMO=docs/demo/demo2.sh GIF=docs/demo/demo2.gif COLS=150 ROWS=28 \
#     TRIM_TAIL=1 docs/demo/run.sh
#
# TRIM_TAIL=1 drops the final tmux-teardown frame so the GIF rests on the
# finished demo (needs ImageMagick 'convert').
#
# Requires: a2a on PATH, tmux, python3, bash. TRIM_TAIL also needs convert.

set -euo pipefail

# Drive the demo from the repo root so the relative example-script path below
# resolves regardless of where the recording is launched from, and so the
# agent card's "Wraps command:" line stays clean (no absolute path) in the GIF.
cd "$(dirname "${BASH_SOURCE[0]}")/../.."

EX="examples/01-exec-demo"
SID="a2a-demo2-$$"
SOCK="a2a-demo2-$$"
TM=(tmux -f /dev/null -L "$SOCK")

# Match the tmux window to the terminal asciinema is recording into.
COLS="${COLS:-$(tput cols 2>/dev/null || echo 150)}"
ROWS="${ROWS:-$(tput lines 2>/dev/null || echo 28)}"

LEFT="$SID.0"
RIGHT="$SID.1"

TYPE_DELAY=0.035   # per-keystroke typing speed
AFTER_CMD=2.2      # pause to read command output
AFTER_NOTE=1.2     # pause to read a comment

# The server command typed into the left pane. The pane is cd-ed into the
# example dir below, so the script name (and the card's "Wraps command:" line)
# stays short; the default port 8080 is omitted for the same reason. $'\n' must
# reach the pane's shell literally, so keep it backslash-escaped.
SERVER_CMD="a2a server --exec \"python3 -u a2a_unaware_agent.py\" --chunk=\$'\\n'"

cleanup() { "${TM[@]}" kill-server 2>/dev/null || true; }
trap cleanup EXIT

# Type a string into a pane one character at a time, then press Enter.
type_in() {
  local pane="$1" text="$2" i ch
  for (( i = 0; i < ${#text}; i++ )); do
    ch="${text:i:1}"
    "${TM[@]}" send-keys -t "$pane" -l "$ch"
    sleep "$TYPE_DELAY"
  done
  "${TM[@]}" send-keys -t "$pane" Enter
}

# Type a prompt comment (a no-op in the shell) into a pane, then pause.
note_in() {
  type_in "$1" "# $2"
  sleep "$AFTER_NOTE"
}

# --- build the session (detached) -------------------------------------------
"${TM[@]}" new-session -d -s "$SID" -x "$COLS" -y "$ROWS" bash --norc --noprofile
"${TM[@]}" split-window -h -t "$LEFT" bash --norc --noprofile

"${TM[@]}" set -g status off
"${TM[@]}" set -g pane-border-status top
"${TM[@]}" set -g pane-border-format " #{pane_title} "
"${TM[@]}" set -g pane-border-style "fg=colour244"
"${TM[@]}" set -g pane-active-border-style "fg=colour244"
"${TM[@]}" set -g default-terminal "tmux-256color"

"${TM[@]}" select-pane -t "$LEFT" -T "A2A server — built from a script"
"${TM[@]}" select-pane -t "$RIGHT" -T "a2a CLI — the client"

# Clean, role-coloured prompts, and cd the server pane into the example dir so
# the typed command stays short. Done before attach so this setup is not shown.
"${TM[@]}" send-keys -t "$LEFT" -l "PS1='\[\e[1;32m\]server\[\e[0m\] \$ '" \; send-keys -t "$LEFT" Enter
"${TM[@]}" send-keys -t "$RIGHT" -l "PS1='\[\e[1;36m\]client\[\e[0m\] \$ '" \; send-keys -t "$RIGHT" Enter
"${TM[@]}" send-keys -t "$LEFT" -l "cd $EX" \; send-keys -t "$LEFT" Enter
"${TM[@]}" send-keys -t "$LEFT" -l "clear" \; send-keys -t "$LEFT" Enter
"${TM[@]}" send-keys -t "$RIGHT" -l "clear" \; send-keys -t "$RIGHT" Enter
sleep 0.6

# --- drive the two panes while attached -------------------------------------
drive() {
  sleep 1.2

  note_in "$LEFT" "Turn an ordinary script into an A2A server -- no A2A code."
  type_in "$LEFT" "$SERVER_CMD"
  sleep 3.0

  note_in "$RIGHT" "Discover what the agent can do -- read its card."
  type_in "$RIGHT" "a2a card get -a http://localhost:8080"
  sleep "$AFTER_CMD"

  note_in "$RIGHT" "Send a message and get one reply back."
  type_in "$RIGHT" 'a2a send -a http://localhost:8080 "hello world from A2A"'
  sleep 3.0

  note_in "$RIGHT" "Stream a reply piece by piece as it arrives."
  type_in "$RIGHT" 'a2a send -a http://localhost:8080 --stream "one two three four"'
  sleep 4.0

  note_in "$RIGHT" "A full A2A round trip. Learn more: examples/"
  sleep 2.5

  "${TM[@]}" kill-session -t "$SID"
}

drive &
"${TM[@]}" attach -t "$SID"
