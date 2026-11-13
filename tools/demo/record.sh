#!/usr/bin/env bash
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
# Records a demo scenario from tools/demo/scenarios as an asciinema cast.
# See tools/demo/README.md.

set -euo pipefail

DEMO_TOOLS="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$DEMO_TOOLS/../.." && pwd)"

usage() {
  cat <<'EOF'
usage: tools/demo/record.sh <scenario>|list [flags]

  --fake          answer with a2a-serve's scripted test agent: no Docker or model key
  --out DIR       where to write the recordings (default tools/demo/out)
  --size COLSxROWS terminal size (default 120x32)
  --speed N       type and pause N times faster (default 1)
  --gif           also render a GIF (fetches agg and the font on first use)
  --gif-speed N   play the GIF N times faster (default 1.5)
  --keep          keep the work directory
  --no-record     play the scenario without recording it, as CI does (no asciinema needed)

a2a is built from this checkout. a2a-serve comes from PATH, or is built from
A2A_SERVE_SRC (default ../a2a-serve) when it isn't on PATH. --fake builds the
scripted test agent from A2A_SERVE_SRC.
EOF
}

[[ $# -ge 1 ]] || { usage >&2; exit 2; }
DEMO_NAME="$1"
shift

if [[ "$DEMO_NAME" == list ]]; then
  for f in "$DEMO_TOOLS"/scenarios/*.sh; do
    printf '%-24s %s\n' "$(basename "$f" .sh)" "$(sed -n 's/^# summary: //p' "$f")"
  done
  exit 0
fi

SCENARIO="$DEMO_TOOLS/scenarios/$DEMO_NAME.sh"
[[ -f "$SCENARIO" ]] || { echo "error: no scenario $SCENARIO (try: record.sh list)" >&2; exit 2; }

DEMO_FAKE=""
DEMO_OUT="$DEMO_TOOLS/out"
SIZE="120x32"
SPEED=1
GIF=""
GIF_SPEED=1.5
KEEP=""
NO_RECORD=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --fake) DEMO_FAKE=1; shift ;;
    --out) DEMO_OUT="$2"; shift 2 ;;
    --size) SIZE="$2"; shift 2 ;;
    --speed) SPEED="$2"; shift 2 ;;
    --gif) GIF=1; shift ;;
    --gif-speed) GIF_SPEED="$2"; shift 2 ;;
    --keep) KEEP=1; shift ;;
    --no-record) NO_RECORD=1; shift ;;
    -h | --help) usage; exit 0 ;;
    *) echo "error: unknown flag $1" >&2; usage >&2; exit 2 ;;
  esac
done
DEMO_COLS="${SIZE%x*}"
DEMO_ROWS="${SIZE#*x}"
DEMO_TYPE_DELAY="$(awk -v s="$SPEED" 'BEGIN { print 0.035 / s }')"
DEMO_PAUSE_SCALE="$(awk -v s="$SPEED" 'BEGIN { print 1 / s }')"

if [[ -n "$NO_RECORD" && -n "$GIF" ]]; then
  echo "error: --gif needs a recording; drop --no-record" >&2
  exit 2
fi
tools=(tmux go jq)
[[ -n "$NO_RECORD" ]] || tools+=(asciinema)
for tool in "${tools[@]}"; do
  command -v "$tool" >/dev/null || { echo "error: $tool is not installed" >&2; exit 1; }
done

mkdir -p "$DEMO_OUT"
DEMO_OUT="$(cd "$DEMO_OUT" && pwd)"
CAST="$DEMO_OUT/$DEMO_NAME.cast"

# A short, stable path: it shows in the recording, and macOS limits Unix
# socket paths to 104 bytes.
DEMO_WORK="${DEMO_WORK:-/tmp/a2a-demo}"
if [[ -e "$DEMO_WORK" ]]; then
  echo "error: $DEMO_WORK exists; stop its agents and remove it, or set DEMO_WORK" >&2
  exit 1
fi
if tmux -L a2a-demo has-session 2>/dev/null; then
  echo "error: a demo is already running (tmux -L a2a-demo attach -r); stop it with tmux -L a2a-demo kill-server" >&2
  exit 1
fi
REAL_HOME="$HOME"
mkdir -p "$DEMO_WORK/you" "$DEMO_WORK/bin" "$DEMO_WORK/fake/bin"

echo "[demo] building a2a" >&2
(cd "$REPO_ROOT" && go build -o "$DEMO_WORK/bin/a2a" .)

A2A_SERVE_SRC="${A2A_SERVE_SRC:-$REPO_ROOT/../a2a-serve}"
need_src() {
  [[ -f "$A2A_SERVE_SRC/go.mod" ]] || { echo "error: no a2a-serve checkout at $A2A_SERVE_SRC; set A2A_SERVE_SRC" >&2; exit 1; }
}
serve_bin="$(command -v a2a-serve || true)"
if [[ -z "$serve_bin" ]]; then
  need_src
  echo "[demo] building a2a-serve from $A2A_SERVE_SRC" >&2
  (cd "$A2A_SERVE_SRC" && CGO_ENABLED=0 go build -o "$DEMO_WORK/fake/bin/a2a-serve" .)
  serve_bin="$DEMO_WORK/fake/bin/a2a-serve"
  ln -s "$serve_bin" "$DEMO_WORK/bin/a2a-serve"
fi
if [[ -n "$DEMO_FAKE" ]]; then
  need_src
  echo "[demo] building the scripted test agent from $A2A_SERVE_SRC" >&2
  (cd "$A2A_SERVE_SRC" && go build -o "$DEMO_WORK/fake/fakeacp" ./internal/agentsrv/testdata/fakeacp)
  # The agent rewrites the scenario's prompts into the scripted agent's
  # commands (see answer_for and behave_as in lib.sh), and announces that it
  # accepts files, as opencode and Claude Code do.
  cat >"$DEMO_WORK/fake/agent" <<EOF
#!/bin/sh
touch "$DEMO_WORK/fake/rules.sed"
sed -u -f "$DEMO_WORK/fake/rules.sed" | "$DEMO_WORK/fake/fakeacp" "\$@" |
  sed -u 's/"agentCapabilities":{"loadSession":true}/"agentCapabilities":{"loadSession":true,"promptCapabilities":{"embeddedContext":true}}/'
EOF
  # a2a-serve start <opencode|claude> runs the scripted agent instead, so the
  # recording shows the commands the lessons give.
  rm -f "$DEMO_WORK/bin/a2a-serve"
  cat >"$DEMO_WORK/bin/a2a-serve" <<EOF
#!/usr/bin/env bash
if [[ "\${1:-}" == start && ( "\${2:-}" == opencode || "\${2:-}" == claude ) ]]; then
  args=(start "\$2" --sandbox local --command "$DEMO_WORK/fake/agent")
  shift 2
  while [[ \$# -gt 0 ]]; do
    case "\$1" in
      --model | --secret) shift 2 ;;
      *) args+=("\$1"); shift ;;
    esac
  done
  set -- "\${args[@]}"
fi
exec "$serve_bin" "\$@"
EOF
  chmod +x "$DEMO_WORK/fake/agent" "$DEMO_WORK/bin/a2a-serve"
fi
export PATH="$DEMO_WORK/bin:$PATH"
# The panes see the demo's binaries, ~/bin and the tools the scenarios use,
# so no a2a-* program on the real PATH shows up as a plugin.
mkdir -p "$DEMO_WORK/tools"
for tool in go jq curl; do
  ln -s "$(command -v "$tool")" "$DEMO_WORK/tools/$tool"
done
DEMO_PANE_PATH="$DEMO_WORK/bin:$DEMO_WORK/you/bin:$DEMO_WORK/tools:/usr/local/bin:/usr/bin:/bin"

# The scenario runs with its own HOME, so it starts with no configuration and
# leaves the real one alone.
if [[ -z "${DOCKER_HOST:-}" ]] && command -v docker >/dev/null; then
  DOCKER_HOST="$(docker context inspect --format '{{.Endpoints.docker.Host}}' 2>/dev/null || true)"
  [[ -n "$DOCKER_HOST" ]] && export DOCKER_HOST
fi
[[ -d "$REAL_HOME/.docker" ]] && export DOCKER_CONFIG="$REAL_HOME/.docker"
GOPATH="$(go env GOPATH)" GOMODCACHE="$(go env GOMODCACHE)" GOCACHE="$(go env GOCACHE)"
export GOPATH GOMODCACHE GOCACHE
export DEMO_REAL_HOME="$REAL_HOME" DEMO_PAUSE_SCALE DEMO_WORK REPO_ROOT
export HOME="$DEMO_WORK/you"
export BASH_SILENCE_DEPRECATION_WARNING=1
unset TMUX

# shellcheck source=lib.sh
source "$DEMO_TOOLS/lib.sh"

cat >"$DEMO_WORK/bashrc" <<EOF
PS1='\[\e[38;2;99;99;110m\]\w\[\e[0m\] \[\e[38;2;110;231;183m\]\$\[\e[0m\] '
PS2='> '
PATH="$DEMO_PANE_PATH"
HISTFILE=
PROMPT_COMMAND='tmux -L $DEMO_SOCKET wait-for -S "demo-ready-\$DEMO_PANE"'
cd ~
EOF

ASCIINEMA_PID=""
cleanup() {
  local rc=$?
  set +e
  if [[ $rc -ne 0 ]]; then
    for name in $(t list-panes -s -t "$DEMO_SESSION" -F '#{@name}' 2>/dev/null); do
      echo "--- pane $name ---" >&2
      screen "$name" | tail -n 25 >&2 || true
    done
  fi
  t kill-server 2>/dev/null
  [[ -n "$ASCIINEMA_PID" ]] && wait "$ASCIINEMA_PID" 2>/dev/null
  find "$HOME" -path '*/.a2a/config.yaml' 2>/dev/null | while read -r cfg; do
    a2a-serve stop --dir "$(dirname "$(dirname "$cfg")")" >/dev/null 2>&1 ||
      echo "[demo] could not stop the agent in $(dirname "$(dirname "$cfg")")" >&2
  done
  declare -F teardown >/dev/null && teardown
  if [[ -n "$KEEP" ]]; then
    echo "[demo] kept $DEMO_WORK" >&2
  else
    chmod -R u+w "$DEMO_WORK" 2>/dev/null
    rm -rf "$DEMO_WORK"
  fi
  exit $rc
}
trap cleanup EXIT

# doctor --fix configures a2a for a2a-serve. It exits non-zero without Docker,
# which --fake runs don't need, so check the settings it writes instead.
a2a-serve doctor --fix >"$DEMO_WORK/doctor.log" 2>&1 || true
grep -q '^credential-helper:' "$HOME/.config/a2a-cli/config.yaml" 2>/dev/null ||
  { cat "$DEMO_WORK/doctor.log" >&2; die "a2a-serve doctor --fix did not configure a2a"; }

# shellcheck source=/dev/null
source "$SCENARIO"
if [[ -n "$DEMO_FAKE" ]] && declare -F fakes >/dev/null; then
  fakes
fi
if declare -F prepare >/dev/null; then
  log "preparing (off camera)"
  prepare
fi
layout

if [[ -n "$NO_RECORD" ]]; then
  log "playing $DEMO_NAME without recording (watch: tmux -L $DEMO_SOCKET attach -r)"
  scene
  t kill-server
  exit 0
fi

# asciinema 3 records without a terminal; asciinema 2 needs one, which
# script(1) provides.
attach="tmux -L $DEMO_SOCKET attach -t $DEMO_SESSION"
if [[ "$(asciinema --version | sed -E 's/^asciinema ([0-9]+).*/\1/')" -ge 3 ]]; then
  TERM=xterm-256color asciinema rec --headless --window-size "${DEMO_COLS}x${DEMO_ROWS}" -f asciicast-v2 \
    --overwrite -q -i 2 -t "a2a: $DEMO_NAME" -c "$attach" "$CAST" </dev/null >/dev/null 2>&1 &
else
  rec="stty cols $DEMO_COLS rows $DEMO_ROWS; exec asciinema rec --overwrite -q -i 2 -t 'a2a: $DEMO_NAME' -c '$attach' '$CAST'"
  if [[ "$(uname)" == Darwin ]]; then
    TERM=xterm-256color script -q /dev/null sh -c "$rec" </dev/null >/dev/null 2>&1 &
  else
    TERM=xterm-256color script -qfec "$rec" /dev/null </dev/null >/dev/null 2>&1 &
  fi
fi
ASCIINEMA_PID=$!
until_ok 30 sh -c "tmux -L $DEMO_SOCKET list-clients -t $DEMO_SESSION | grep -q ."
log "recording $CAST (watch: tmux -L $DEMO_SOCKET attach -r)"
pause 1

scene

pause 2
# Cut the cast before tmux clears the screen on exit.
cutoff="$(cast_time)"
t kill-server
wait "$ASCIINEMA_PID" || true
ASCIINEMA_PID=""
awk -v cut="$cutoff" 'NR == 1 { print; next } { split($0, f, /[[,]/) } f[2] + 0 <= cut + 0' "$CAST" >"$CAST.tmp"
mv "$CAST.tmp" "$CAST"

for var in OPENROUTER_API_KEY ANTHROPIC_API_KEY OPENAI_API_KEY GEMINI_API_KEY ${DEMO_SECRET_VARS:-}; do
  value="${!var:-}"
  if [[ ${#value} -ge 8 ]] && grep -qF -- "$value" "$CAST"; then
    rm -f "$CAST"
    die "the recording showed the value of \$$var; deleted $CAST"
  fi
done

if [[ -n "$GIF" ]]; then
  "$DEMO_TOOLS/compose.sh" "$CAST" "${CAST%.cast}.gif" --speed "$GIF_SPEED"
fi
ls -1 "$DEMO_OUT/$DEMO_NAME".* 2>/dev/null || true
