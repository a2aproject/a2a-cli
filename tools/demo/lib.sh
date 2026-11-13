# Copyright 2026 The A2A Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# Helpers that scenarios use to drive tmux panes while asciinema records them.
# record.sh sources this file; it must stay compatible with bash 3.2 (macOS).

DEMO_SOCKET="${DEMO_SOCKET:-a2a-demo}"
DEMO_SESSION="demo"
DEMO_TYPE_DELAY="${DEMO_TYPE_DELAY:-0.035}"
DEMO_PAUSE_SCALE="${DEMO_PAUSE_SCALE:-1}"
DEMO_TIMEOUT="${DEMO_TIMEOUT:-600}"

t() {
  tmux -L "$DEMO_SOCKET" "$@"
}

log() {
  printf '\033[2m[demo] %s\033[0m\n' "$*" >&2
}

die() {
  printf '[demo] error: %s\n' "$*" >&2
  exit 1
}

# pane_id prints the tmux pane ID of the pane named $1.
pane_id() {
  local id
  id="$(t list-panes -s -t "$DEMO_SESSION" -F '#{@name} #{pane_id}' | awk -v n="$1" '$1 == n { print $2 }')"
  [[ -n "$id" ]] || die "no pane named $1"
  printf '%s' "$id"
}

_pane_shell() {
  printf 'DEMO_PANE=%q exec bash --noprofile --rcfile %q -i' "$1" "$DEMO_WORK/bashrc"
}

_name_pane() {
  t set-option -p -t "$1" @name "$2"
  wait_ready "$2" 30
}

# panes creates the session with its first pane, named $1. Further names
# are split off with split_pane.
panes() {
  t new-session -d -s "$DEMO_SESSION" -x "$DEMO_COLS" -y "$DEMO_ROWS" "$(_pane_shell "$1")"
  t set-option -g status off
  t set-option -g pane-border-status off
  t set-option -g pane-border-format ' #{?@title,#{@title},#{@name}} '
  t set-option -g pane-border-style 'fg=#2e2e35'
  t set-option -g pane-active-border-style 'fg=#63636e'
  _name_pane "$(t display-message -p -t "$DEMO_SESSION" '#{pane_id}')" "$1"
}

# split_pane <new> <from> <above|below|left|right> <percent> adds pane <new>
# next to <from>, taking <percent> of its space.
split_pane() {
  local flags
  case "$3" in
    above) flags="-vb" ;;
    below) flags="-v" ;;
    left) flags="-hb" ;;
    right) flags="-h" ;;
    *) die "split_pane: bad direction $3" ;;
  esac
  local id
  t set-option -g pane-border-status top
  id="$(t split-window "$flags" -l "$4%" -t "$(pane_id "$2")" -P -F '#{pane_id}' "$(_pane_shell "$1")")"
  _name_pane "$id" "$1"
}

# title <pane> <text> sets the label on the pane's border.
title() {
  t set-option -p -t "$(pane_id "$1")" @title "$2"
}

focus() {
  t select-pane -t "$(pane_id "$1")"
}

# wait_ready <pane> [timeout] waits for the next prompt of the pane's shell.
wait_ready() {
  local timeout="${2:-$DEMO_TIMEOUT}"
  t wait-for "demo-ready-$1" &
  local pid=$! waited=0
  while kill -0 "$pid" 2>/dev/null; do
    if ((waited >= timeout * 10)); then
      kill "$pid" 2>/dev/null
      die "pane $1: no prompt after ${timeout}s"
    fi
    sleep 0.1
    waited=$((waited + 1))
  done
  wait "$pid"
}

# type_keys <pane> <text> types text into the pane without pressing Enter.
type_keys() {
  local id text="$2" i c
  id="$(pane_id "$1")"
  for ((i = 0; i < ${#text}; i++)); do
    c="${text:i:1}"
    if [[ "$c" == $'\n' ]]; then
      t send-keys -t "$id" Enter
    elif [[ "$c" == ";" ]]; then
      # tmux takes a lone ; as the end of its command.
      t send-keys -t "$id" -l -- '\;'
    else
      t send-keys -t "$id" -l -- "$c"
    fi
    sleep "$DEMO_TYPE_DELAY"
  done
}

# run <pane> <command> types the command, runs it and waits for the prompt.
run() {
  focus "$1"
  type_keys "$1" "$2"
  sleep 0.3
  t send-keys -t "$(pane_id "$1")" Enter
  wait_ready "$1" "${3:-$DEMO_TIMEOUT}"
  pause 1
}

# paste_run <pane> <command> [timeout] pastes a long command at once, runs it and
# waits for the prompt.
paste_run() {
  focus "$1"
  printf '%s' "$2" | t load-buffer -b demo-paste -
  t paste-buffer -d -b demo-paste -t "$(pane_id "$1")"
  pause 2
  t send-keys -t "$(pane_id "$1")" Enter
  wait_ready "$1" "${3:-$DEMO_TIMEOUT}"
  pause 1
}

# start <pane> <command> types and runs a command that keeps running.
start() {
  focus "$1"
  type_keys "$1" "$2"
  sleep 0.3
  t send-keys -t "$(pane_id "$1")" Enter
}

# answer <pane> <ERE> <reply> waits for a question of a running command and
# types the reply, an empty one to take the default.
answer() {
  wait_text "$1" "$2"
  pause 1
  type_keys "$1" "$3"
  sleep 0.3
  t send-keys -t "$(pane_id "$1")" Enter
}

# say <pane> <text> types a comment line, as narration.
say() {
  run "$1" "# $2"
}

clear_pane() {
  t send-keys -t "$(pane_id "$1")" C-l
  sleep 0.3
}

interrupt() {
  t send-keys -t "$(pane_id "$1")" C-c
  wait_ready "$1" 60
}

pause() {
  sleep "$(awk -v s="$1" -v k="$DEMO_PAUSE_SCALE" 'BEGIN { print s * k }')"
}

screen() {
  t capture-pane -p -J -S -500 -t "$(pane_id "$1")"
}

# wait_text <pane> <ERE> [timeout] waits until the pane shows a match.
wait_text() {
  local timeout="${3:-$DEMO_TIMEOUT}" start
  start="$(date +%s)"
  until screen "$1" | grep -Eq -- "$2"; do
    if (($(date +%s) - start >= timeout)); then
      die "pane $1: no match for /$2/ after ${timeout}s"
    fi
    sleep 0.5
  done
}

# last_match <pane> <ERE> prints the last match of the ERE on the pane.
last_match() {
  local m
  m="$(screen "$1" | grep -Eo -- "$2" | tail -n 1)"
  [[ -n "$m" ]] || die "pane $1: no match for /$2/"
  printf '%s' "$m"
}

# until_ok <timeout> <command...> retries a command off camera until it
# succeeds, for waits that would only bore the viewer.
until_ok() {
  local timeout="$1" start
  shift
  start="$(date +%s)"
  until "$@" >/dev/null 2>&1; do
    if (($(date +%s) - start >= timeout)); then
      die "timed out after ${timeout}s: $*"
    fi
    sleep 1
  done
}

# expect_status <pane> <status> fails the recording unless the pane's last
# a2a send ended in status, such as completed.
expect_status() {
  local got
  got="$(last_match "$1" 'Status: +[a-z-]+')"
  [[ "${got##* }" == "$2" ]] || die "pane $1: the last task is ${got##* }, want $2"
}

# answer_for <prompt> <text> makes the scripted agent answer the prompt with
# text, in fake mode. Call it in the scenario's fakes function.
answer_for() {
  DEMO_ANSWERS=$((DEMO_ANSWERS + 1))
  printf '%s' "$2" >"$DEMO_WORK/fake/answer-$DEMO_ANSWERS.txt"
  _fake_rule "$1" "read $DEMO_WORK/fake/answer-$DEMO_ANSWERS.txt"
}

# behave_as <prompt> <command> makes the scripted agent handle the prompt as
# one of its commands, such as tool, ask, whoami, "slow 3s" or recall, in fake
# mode. Call it in the scenario's fakes function.
behave_as() {
  _fake_rule "$1" "$2"
}

DEMO_ANSWERS=0
_fake_rule() {
  local from to
  from="$(printf '%s' "$1" | sed -e 's/[]\\/.*^$[|]/\\&/g')"
  to="$(printf '%s' "$2" | sed -e 's/[\\/&|]/\\&/g')"
  printf 's|%s|%s|g\n' "$from" "$to" >>"$DEMO_WORK/fake/rules.sed"
}

# lesson_agent starts the lessons' agent in ~/a2a-lessons/agent in the
# background, and points ~/a2a-lessons/.env at it, as lesson 1 leaves them.
lesson_agent() {
  mkdir -p "$HOME/a2a-lessons/agent"
  (cd "$HOME/a2a-lessons/agent" &&
    a2a-serve start opencode --name lesson-agent --model openrouter/~google/gemini-flash-latest \
      --secret OPENROUTER_API_KEY --detach >/dev/null)
  echo "A2ACLI_AGENT_CARD=$(a2a-serve status --dir "$HOME/a2a-lessons/agent" --url=local)" >"$HOME/a2a-lessons/.env"
}

# cast_time prints the time of the last event recorded in the cast.
cast_time() {
  [[ -f "$CAST" ]] || { echo 0; return; }
  tail -n 1 "$CAST" | awk -F'[[,]' '{ print $2 }'
}

# ask_human <message> [<pane> <ERE> [timeout]] tells the person running the
# recording to do something, and waits for <pane> to show <ERE> as the sign
# that it is done.
ask_human() {
  printf '\n\033[1;33m[demo] ACTION NEEDED: %s\033[0m\n' "$1" >&2
  [[ $# -ge 3 ]] || return 0
  wait_text "$2" "$3" "${4:-1800}"
}
