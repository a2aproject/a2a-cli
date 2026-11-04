#!/usr/bin/env bash
#
# a2a installer
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/a2aproject/a2a-cli/main/install.sh | bash
#
# Environment variables:
#   A2A_VERSION       Specific version to install (e.g. v0.3.1 or 0.3.1).
#                     Defaults to the latest published release.
#   A2A_INSTALL_DIR   Directory to install the binary into. Defaults to
#                     /usr/local/bin (falls back to $HOME/.local/bin if not
#                     writable and sudo is unusable).
#   A2A_BINARY_NAME   Name of the installed binary. Defaults to a2a.
#
set -euo pipefail

REPO="a2aproject/a2a-cli"
A2A_BINARY_NAME="${A2A_BINARY_NAME:-a2a}"
A2A_INSTALL_DIR="${A2A_INSTALL_DIR:-/usr/local/bin}"

tmpdir=""
trap 'rm -rf "${tmpdir:-}"' EXIT

log()  { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m==>\033[0m %s\n' "$*" >&2; }
err()  { printf '\033[1;31m==>\033[0m %s\n' "$*" >&2; exit 1; }

require() {
  command -v "$1" >/dev/null 2>&1 || err "required command not found: $1"
}

require uname
require tar
require install
require mktemp

if command -v curl >/dev/null 2>&1; then
  DOWNLOADER="curl"
elif command -v wget >/dev/null 2>&1; then
  DOWNLOADER="wget"
else
  err "neither curl nor wget is installed"
fi

if command -v sha256sum >/dev/null 2>&1; then
  SHA256=(sha256sum)
elif command -v shasum >/dev/null 2>&1; then
  SHA256=(shasum -a 256)
else
  err "neither sha256sum nor shasum is installed"
fi

fetch() {
  local url="$1" out="$2"
  if [ "$DOWNLOADER" = "curl" ]; then
    curl -fsSL "$url" -o "$out"
  else
    wget -qO "$out" "$url"
  fi
}

resolve_redirect() {
  local url="$1"
  if [ "$DOWNLOADER" = "curl" ]; then
    curl -fsSLI -o /dev/null -w '%{url_effective}' "$url"
  else
    wget -S --spider "$url" 2>&1 \
      | awk '/^  Location: / { loc = $2 } END { sub(/\r$/, "", loc); if (loc == "") exit 1; print loc }'
  fi
}

# sudo is only usable if it doesn't need a password or can prompt for one.
can_sudo() {
  command -v sudo >/dev/null 2>&1 || return 1
  sudo -n true 2>/dev/null && return 0
  { : </dev/tty; } 2>/dev/null
}

detect_os() {
  local os
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$os" in
    linux)  echo "linux" ;;
    darwin) echo "darwin" ;;
    *) err "unsupported OS: $os (see https://github.com/${REPO}/releases for Windows builds)" ;;
  esac
}

detect_arch() {
  local arch
  arch="$(uname -m)"
  # A shell running under Rosetta reports x86_64 on Apple Silicon.
  if [ "$arch" = "x86_64" ] && [ "$(sysctl -n sysctl.proc_translated 2>/dev/null)" = "1" ]; then
    arch="arm64"
  fi
  case "$arch" in
    x86_64|amd64)  echo "amd64" ;;
    aarch64|arm64) echo "arm64" ;;
    *) err "unsupported architecture: $arch" ;;
  esac
}

resolve_tag() {
  if [ -n "${A2A_VERSION:-}" ]; then
    echo "v${A2A_VERSION#v}"
    return
  fi
  # Follow the releases/latest redirect to avoid GitHub API rate limits.
  local latest_url="https://github.com/${REPO}/releases/latest"
  local url
  url="$(resolve_redirect "$latest_url")" || err "failed to detect latest version from $latest_url"
  case "$url" in
    */releases/tag/*) echo "${url##*/}" ;;
    *) err "failed to detect latest version from $latest_url (resolved to $url)" ;;
  esac
}

verify_checksum() {
  local archive="$1" checksums="$2" name expected actual
  name="$(basename "$archive")"
  expected="$(awk -v n="$name" '$2 == n { print $1 }' "$checksums")"
  [ -n "$expected" ] || err "no checksum found for $name"
  actual="$("${SHA256[@]}" "$archive" | awk '{ print $1 }')"
  [ "$expected" = "$actual" ] || err "checksum mismatch for $name (want $expected, got $actual)"
}

choose_install_dir() {
  local dir="$A2A_INSTALL_DIR"
  if [ -d "$dir" ] && [ -w "$dir" ]; then
    echo "$dir"
    return
  fi
  if [ ! -d "$dir" ] && mkdir -p "$dir" 2>/dev/null; then
    echo "$dir"
    return
  fi
  if can_sudo; then
    echo "$dir"
    return
  fi
  warn "$dir is not writable and sudo is unusable; falling back to \$HOME/.local/bin"
  mkdir -p "$HOME/.local/bin"
  echo "$HOME/.local/bin"
}

install_binary() {
  local src="$1" dest_dir="$2" dest="$2/$A2A_BINARY_NAME"
  if [ -w "$dest_dir" ]; then
    install -m 0755 "$src" "$dest"
    return
  fi
  log "Installing to $dest (requires sudo)"
  sudo mkdir -p "$dest_dir"
  sudo install -m 0755 "$src" "$dest"
}

main() {
  local os arch tag version base_url archive_name dest_dir
  os="$(detect_os)"
  arch="$(detect_arch)"
  tag="$(resolve_tag)"
  version="${tag#v}"
  archive_name="a2a_${version}_${os}_${arch}.tar.gz"
  base_url="https://github.com/${REPO}/releases/download/${tag}"

  log "Installing a2a ${tag} for ${os}/${arch}"
  log "Asset: ${base_url}/${archive_name}"

  tmpdir="$(mktemp -d)"

  fetch "${base_url}/${archive_name}" "${tmpdir}/${archive_name}" \
    || err "failed to download ${base_url}/${archive_name}"
  fetch "${base_url}/checksums.txt" "${tmpdir}/checksums.txt" \
    || err "failed to download ${base_url}/checksums.txt"
  verify_checksum "${tmpdir}/${archive_name}" "${tmpdir}/checksums.txt"

  tar -xzf "${tmpdir}/${archive_name}" -C "$tmpdir" a2a \
    || err "failed to extract a2a from ${archive_name}"

  dest_dir="$(choose_install_dir)"
  install_binary "${tmpdir}/a2a" "$dest_dir"

  log "Installed ${A2A_BINARY_NAME} to ${dest_dir}/${A2A_BINARY_NAME}"

  case ":$PATH:" in
    *":${dest_dir}:"*) ;;
    *) warn "${dest_dir} is not in your PATH; add it to use ${A2A_BINARY_NAME} directly" ;;
  esac

  "${dest_dir}/${A2A_BINARY_NAME}" version || warn "installed binary failed to report its version"
}

main "$@"
