#!/usr/bin/env bash
# Validate — and optionally archive — the Claude Code plugin and its marketplace.
#
# The plugin is installable straight from the git repo once the manifests are
# committed (see "Install" output below), so this script's main job is to catch a
# broken manifest before you push. With --zip it also builds a distributable
# archive for the marketplace `archive` source type.
#
#   ./scripts/package-claude-plugin.sh          # validate manifests
#   ./scripts/package-claude-plugin.sh --zip    # validate, then build dist/*.zip
set -euo pipefail

cd "$(dirname "$0")/.."   # repo root
REPO_ROOT="$(pwd)"
PLUGIN_DIR="agent-plugin"
MARKETPLACE="${REPO_ROOT}/.claude-plugin/marketplace.json"
PLUGIN_MANIFEST="${REPO_ROOT}/${PLUGIN_DIR}/.claude-plugin/plugin.json"
HOOKS="${REPO_ROOT}/${PLUGIN_DIR}/hooks/hooks.json"

want_zip=0
[[ "${1:-}" == "--zip" ]] && want_zip=1

fail() { echo "FAIL: $*" >&2; exit 1; }
ok() { echo "  ok: $*"; }

command -v jq >/dev/null 2>&1 || fail "jq is required"

echo "== validate JSON =="
for f in "$MARKETPLACE" "$PLUGIN_MANIFEST" "$HOOKS"; do
  [[ -f "$f" ]] || fail "missing $f"
  jq empty "$f" 2>/dev/null || fail "invalid JSON: $f"
  ok "${f#"$REPO_ROOT"/}"
done

echo "== check required fields =="
[[ "$(jq -r '.name' "$MARKETPLACE")" != "null" ]] || fail "marketplace.name missing"
[[ "$(jq -r '.owner.name' "$MARKETPLACE")" != "null" ]] || fail "marketplace.owner.name missing"
[[ "$(jq -r '.plugins | length' "$MARKETPLACE")" -ge 1 ]] || fail "marketplace.plugins is empty"
PLUGIN_NAME="$(jq -r '.name' "$PLUGIN_MANIFEST")"
[[ "$PLUGIN_NAME" != "null" ]] || fail "plugin.name missing"
ok "marketplace '$(jq -r '.name' "$MARKETPLACE")' lists plugin '$PLUGIN_NAME'"

echo "== check the marketplace source path resolves =="
SRC="$(jq -r '.plugins[0].source' "$MARKETPLACE")"
[[ -d "${REPO_ROOT}/${SRC#./}" ]] || fail "plugin source path does not exist: $SRC"
[[ -f "${REPO_ROOT}/${SRC#./}/.claude-plugin/plugin.json" ]] || fail "no plugin.json under $SRC"
ok "source '$SRC' -> ${SRC#./}/.claude-plugin/plugin.json"

echo "== check bundled components =="
[[ -d "${REPO_ROOT}/${PLUGIN_DIR}/skills" ]] && ok "skills/ present ($(find "${REPO_ROOT}/${PLUGIN_DIR}/skills" -name SKILL.md | wc -l | tr -d ' ') skill(s))"
jq -e '.hooks.UserPromptSubmit and .hooks.PostToolUse' "$HOOKS" >/dev/null && ok "hooks.json declares UserPromptSubmit + PostToolUse"

# Prefer the official validator when present; it is authoritative.
if command -v claude >/dev/null 2>&1; then
  echo "== claude plugin validate =="
  claude plugin validate "${REPO_ROOT}/${PLUGIN_DIR}" --strict || fail "claude plugin validate failed"
else
  echo "note: 'claude' CLI not found; skipped 'claude plugin validate' (JSON checks above still ran)."
fi

if [[ "$want_zip" -eq 1 ]]; then
  echo "== build archive =="
  command -v zip >/dev/null 2>&1 || fail "zip is required for --zip"
  VERSION="$(jq -r '.version // "0.0.0"' "$PLUGIN_MANIFEST")"
  OUT="${REPO_ROOT}/dist/${PLUGIN_NAME}-${VERSION}.zip"
  mkdir -p "${REPO_ROOT}/dist"
  rm -f "$OUT"
  # Zip the plugin's contents so .claude-plugin/ sits at the archive root.
  ( cd "${REPO_ROOT}/${PLUGIN_DIR}" && zip -rq "$OUT" . -x '.DS_Store' )
  ok "wrote ${OUT#"$REPO_ROOT"/}"
  if command -v shasum >/dev/null 2>&1; then
    echo "  sha256: $(shasum -a 256 "$OUT" | awk '{print $1}')"
  elif command -v sha256sum >/dev/null 2>&1; then
    echo "  sha256: $(sha256sum "$OUT" | awk '{print $1}')"
  fi
fi

MKT_NAME="$(jq -r '.name' "$MARKETPLACE")"
cat <<EOF

All checks passed.

Install from the published repo:
  /plugin marketplace add a2aproject/a2a-cli
  /plugin install ${PLUGIN_NAME}@${MKT_NAME}

Test locally from this checkout:
  /plugin marketplace add ${REPO_ROOT}
  /plugin install ${PLUGIN_NAME}@${MKT_NAME}

The plugin drives the 'a2a' binary — install it first (see README).
EOF
