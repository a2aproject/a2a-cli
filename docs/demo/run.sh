#!/usr/bin/env bash
#
# Re-create the README demo GIF (docs/demo/a2a-demo.gif).
#
# Builds the a2a CLI from this checkout, records demo.sh with asciinema, and
# renders the cast to a GIF with agg. Run it from anywhere:
#
#   docs/demo/run.sh
#
# It needs `go` and `asciinema` on PATH. It will bootstrap `agg` and a small
# Nerd Font (for the agnoster prompt's powerline glyphs) into a cache dir the
# first time, so no manual install is required.
#   asciinema: https://docs.asciinema.org/manual/cli/installation/
#
# Tunables (env vars, with defaults):
#   STYLE  agnoster | robbyrussell   prompt style baked into the recording
#   THEME  kanagawa                  agg color theme (see `agg --help`)
#   COLS   92                        terminal columns
#   ROWS   26                        terminal rows
#   A2A_DEMO_CACHE   ~/.cache/a2a-cli-demo   where agg + fonts are cached
#   A2A_DEMO_FONT_DIR   <dir>        use your own Nerd Font dir instead

set -euo pipefail

ROOT="$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
DEMO_DIR="$ROOT/docs/demo"
CACHE="${A2A_DEMO_CACHE:-$HOME/.cache/a2a-cli-demo}"

STYLE="${STYLE:-agnoster}"
THEME="${THEME:-kanagawa}"
COLS="${COLS:-92}"
ROWS="${ROWS:-26}"

mkdir -p "$CACHE/bin" "$CACHE/fonts"

for tool in go asciinema; do
  command -v "$tool" >/dev/null 2>&1 || { echo "error: '$tool' not found on PATH" >&2; exit 1; }
done

# --- bootstrap agg (prebuilt binary) if missing -----------------------------
if ! command -v agg >/dev/null 2>&1; then
  if [[ ! -x "$CACHE/bin/agg" ]]; then
    case "$(uname -s)/$(uname -m)" in
      Linux/x86_64)  asset="agg-x86_64-unknown-linux-gnu" ;;
      Linux/aarch64) asset="agg-aarch64-unknown-linux-gnu" ;;
      Darwin/x86_64) asset="agg-x86_64-apple-darwin" ;;
      Darwin/arm64)  asset="agg-aarch64-apple-darwin" ;;
      *) echo "error: no prebuilt agg for $(uname -s)/$(uname -m); install agg manually" >&2; exit 1 ;;
    esac
    echo "==> fetching agg ($asset)"
    curl -fsSL -o "$CACHE/bin/agg" \
      "https://github.com/asciinema/agg/releases/latest/download/$asset"
    chmod +x "$CACHE/bin/agg"
  fi
  export PATH="$CACHE/bin:$PATH"
fi

# --- resolve a Nerd Font for the agnoster powerline glyphs -------------------
if [[ -n "${A2A_DEMO_FONT_DIR:-}" ]]; then
  font_args=(--font-dir "$A2A_DEMO_FONT_DIR" --font-family "JetBrainsMono Nerd Font")
else
  if [[ ! -f "$CACHE/fonts/SymbolsNerdFontMono-Regular.ttf" ]]; then
    echo "==> fetching Symbols Nerd Font"
    curl -fsSL -o "$CACHE/fonts/symbols.zip" \
      "https://github.com/ryanoasis/nerd-fonts/releases/latest/download/NerdFontsSymbolsOnly.zip"
    ( cd "$CACHE/fonts" && unzip -o -q symbols.zip 'SymbolsNerdFontMono-Regular.ttf' && rm -f symbols.zip )
  fi
  # DejaVu Sans Mono (present on ~every Linux/macOS) draws the text; the symbols
  # font supplies the powerline/git glyphs.
  font_args=(--font-dir "$CACHE/fonts" --font-family "DejaVu Sans Mono,Symbols Nerd Font Mono")
fi

# --- refuse to record if the demo ports are already in use ------------------
port_busy() { (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null && { exec 3>&- 3<&-; return 0; } || return 1; }
for p in 8080 8081; do
  if port_busy "$p"; then
    echo "error: port $p is in use; free it and retry (the demo starts servers there)" >&2
    exit 1
  fi
done

# --- build the CLI from this checkout ---------------------------------------
BIN_DIR="$(mktemp -d)"
trap 'rm -rf "$BIN_DIR"' EXIT
PKG="github.com/a2aproject/a2a-cli/internal/cli"
VERSION="$(git -C "$ROOT" describe --tags --always 2>/dev/null || echo dev)"
COMMIT="$(git -C "$ROOT" rev-parse --short HEAD 2>/dev/null || echo none)"
DATE="$(date +%Y-%m-%d)"
echo "==> building a2a ($VERSION)"
( cd "$ROOT" && go build \
    -ldflags "-X $PKG.version=$VERSION -X $PKG.commit=$COMMIT -X $PKG.date=$DATE" \
    -o "$BIN_DIR/a2a" . )
export PATH="$BIN_DIR:$PATH"

# The cast is a throwaway intermediate; only the GIF is kept in the repo.
CAST="$BIN_DIR/a2a-demo.cast"
GIF="$DEMO_DIR/a2a-demo.gif"

echo "==> recording (STYLE=$STYLE, ${COLS}x${ROWS})"
STYLE="$STYLE" asciinema rec --cols "$COLS" --rows "$ROWS" --overwrite \
  --title "a2a CLI — build a server from a script and talk to it" \
  --command "bash $DEMO_DIR/demo.sh" \
  "$CAST"

echo "==> rendering GIF (theme=$THEME)"
agg --theme "$THEME" --bold-is-bright --font-size 16 --line-height 1.35 \
  --idle-time-limit 2.5 "${font_args[@]}" \
  "$CAST" "$GIF"

echo "==> done: $GIF"
