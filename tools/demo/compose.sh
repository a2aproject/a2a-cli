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
# Renders a cast as a GIF in the documentation site's colors and font. Fetches
# agg and Geist Mono into ~/.cache/a2a-cli-demo the first time.
#
#   tools/demo/compose.sh <name>.cast <out.gif> [--speed N]

set -euo pipefail

usage() {
  echo "usage: tools/demo/compose.sh <name>.cast <out.gif> [--speed N]" >&2
  exit 2
}

[[ $# -ge 2 ]] || usage
CAST="$1"
OUT="$2"
shift 2
SPEED=1.5
while [[ $# -gt 0 ]]; do
  case "$1" in
    --speed) SPEED="$2"; shift 2 ;;
    *) usage ;;
  esac
done

CACHE="${A2A_DEMO_CACHE:-${DEMO_REAL_HOME:-$HOME}/.cache/a2a-cli-demo}"
mkdir -p "$CACHE/bin" "$CACHE/fonts"

AGG="$(command -v agg || true)"
if [[ -z "$AGG" ]]; then
  AGG="$CACHE/bin/agg"
  if [[ ! -x "$AGG" ]]; then
    case "$(uname -s)/$(uname -m)" in
      Linux/x86_64) asset="agg-x86_64-unknown-linux-gnu" ;;
      Linux/aarch64) asset="agg-aarch64-unknown-linux-gnu" ;;
      Darwin/x86_64) asset="agg-x86_64-apple-darwin" ;;
      Darwin/arm64) asset="agg-aarch64-apple-darwin" ;;
      *) echo "error: no prebuilt agg for $(uname -s)/$(uname -m); install agg" >&2; exit 1 ;;
    esac
    echo "[demo] fetching agg ($asset)" >&2
    curl -fsSL -o "$AGG" "https://github.com/asciinema/agg/releases/latest/download/$asset"
    chmod +x "$AGG"
  fi
fi

if ! ls "$CACHE"/fonts/GeistMono-*.ttf >/dev/null 2>&1; then
  echo "[demo] fetching Geist Mono" >&2
  css="$(curl -fsSL "https://fonts.googleapis.com/css2?family=Geist+Mono:wght@400;700")"
  i=0
  for url in $(grep -o 'https://[^)]*\.ttf' <<<"$css"); do
    i=$((i + 1))
    curl -fsSL -o "$CACHE/fonts/GeistMono-$i.ttf" "$url"
  done
fi

# Background, foreground, then the 16 ANSI colors, from the site's palette.
THEME="09090b,e4e4e7,18181c,f87171,6ee7b7,e5c07b,93c5fd,c4b5fd,7dd3c0,a1a1aa,63636e,fca5a5,a7f3d0,f5d896,bfdbfe,ddd6fe,a5f3fc,e4e4e7"

"$AGG" --theme "$THEME" --font-dir "$CACHE/fonts" --font-family "Geist Mono" \
  --font-size 15 --line-height 1.5 --speed "$SPEED" --idle-time-limit 2 --last-frame-duration 4 \
  "$CAST" "$OUT"
