#!/usr/bin/env bash
# Map the demo's pretty domain to a local address by editing the hosts file.
# Works on macOS and Linux (both use /etc/hosts); on macOS it also flushes the
# DNS cache so the change takes effect immediately.
#
#   ./setup-hosts.sh            # add   nimbus.coffee -> 127.0.0.1
#   ./setup-hosts.sh --remove   # remove the entry
#   ./setup-hosts.sh --dry-run  # print the resulting file, change nothing
#
# Overridable via env: DOMAIN, IP, HOSTS_FILE, SUDO (set SUDO= to skip sudo).
set -euo pipefail

DOMAIN="${DOMAIN:-nimbus.coffee}"
IP="${IP:-127.0.0.1}"
HOSTS_FILE="${HOSTS_FILE:-/etc/hosts}"
SUDO="${SUDO-sudo}"
BEGIN="# >>> a2a nimbus demo >>>"
END="# <<< a2a nimbus demo <<<"

action="add"
dry=0
for arg in "$@"; do
  case "$arg" in
    --remove) action="remove" ;;
    --dry-run) dry=1 ;;
    *) echo "unknown argument: $arg" >&2; exit 2 ;;
  esac
done

# Rebuild the file without any existing demo block, then re-add it for "add".
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
awk -v b="$BEGIN" -v e="$END" '
  $0==b {skip=1}
  skip==0 {print}
  $0==e {skip=0}
' "$HOSTS_FILE" > "$tmp"

if [[ "$action" == "add" ]]; then
  {
    echo "$BEGIN"
    echo "$IP	$DOMAIN"
    echo "$END"
  } >> "$tmp"
fi

if [[ "$dry" -eq 1 ]]; then
  echo "--- would write $HOSTS_FILE: ---"
  cat "$tmp"
  exit 0
fi

$SUDO cp "$HOSTS_FILE" "${HOSTS_FILE}.a2a.bak"
$SUDO cp "$tmp" "$HOSTS_FILE"

if [[ "$(uname -s)" == "Darwin" ]]; then
  $SUDO dscacheutil -flushcache || true
  $SUDO killall -HUP mDNSResponder || true
fi

echo "${action}: ${DOMAIN} -> ${IP} in ${HOSTS_FILE} (backup at ${HOSTS_FILE}.a2a.bak)"
