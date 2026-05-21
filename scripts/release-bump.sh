#!/usr/bin/env bash
# release-bump.sh — version-bump, tag, and push a jamsesh release.
#
# Usage:
#   scripts/release-bump.sh [--dry-run] vX.Y.Z
#
# What it does:
#   1. Validates the version argument.
#   2. Pre-flight checks (clean tree, on main, tag not yet taken,
#      all three target files exist).
#   3. Edits in-place:
#        plugins/jamsesh/bin/jamsesh                — readonly JAMSESH_PLUGIN_VERSION="vX.Y.Z"
#        deploy/compose/.env.example                — JAMSESH_VERSION=vX.Y.Z
#        plugins/jamsesh/.claude-plugin/plugin.json — .version = "X.Y.Z"  (no v prefix)
#   4. Stages those three files (explicit paths only — never git add -A).
#   5. Commits  "release-prep: vX.Y.Z"
#   6. Annotated tag "vX.Y.Z"
#   7. Pushes main, then the tag (branch must be reachable before tag fires CI).
#
# GPG signing:
#   Tags are annotated but not GPG-signed by default (keyless cosign in
#   release.yml is the supply-chain trust anchor). To opt in:
#     change: git tag -a "$VERSION" -m "Release $VERSION"
#       to:   git tag -s "$VERSION" -m "Release $VERSION"
#
# Dry-run:
#   Prefix with --dry-run to print every action without touching anything.

set -euo pipefail

# ── helpers ──────────────────────────────────────────────────────────────────

die()  { printf 'release-bump.sh: %s\n' "$*" >&2; exit 1; }
warn() { printf 'release-bump.sh: warning: %s\n' "$*" >&2; }
info() { printf 'release-bump.sh: %s\n' "$*" >&2; }

# run <cmd> [args…]
#   Executes the command normally, or (in dry-run) just echoes it.
run() {
  if [[ -n "${DRY_RUN:-}" ]]; then
    printf '[dry-run] %s\n' "$*" >&2
  else
    "$@"
  fi
}

# show_sed_diff <file> <old_pattern> <new_value>
#   In dry-run, prints a unified diff of what the sed edit would produce.
show_sed_diff() {
  local file="$1" pattern="$2" replacement="$3"
  diff -u "$file" <(sed "s/${pattern}/${replacement}/" "$file") \
    --label "a/$file" --label "b/$file" || true
}

# sed_inplace <file> <old_pattern> <new_value>
#   Portable in-place edit using a temp file (works on Linux and macOS).
#   Preserves the original file's mode — without the explicit chmod the temp
#   file inherits the default umask (typically 0644) and the mv strips the
#   executable bit on plugins/jamsesh/bin/jamsesh.
sed_inplace() {
  local file="$1" pattern="$2" replacement="$3"
  local tmp="${file}.tmp"
  local mode
  mode="$(stat -c '%a' "$file" 2>/dev/null || stat -f '%Lp' "$file")"
  sed "s/${pattern}/${replacement}/" "$file" > "$tmp" \
    && chmod "$mode" "$tmp" \
    && mv "$tmp" "$file"
}

# ── argument parsing ──────────────────────────────────────────────────────────

DRY_RUN=""
if [[ "${1:-}" == "--dry-run" ]]; then
  DRY_RUN=1
  shift
fi

VERSION="${1:-}"

if [[ -z "$VERSION" ]]; then
  printf 'Usage: scripts/release-bump.sh [--dry-run] vX.Y.Z\n' >&2
  exit 1
fi

if ! [[ "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  die "invalid version '$VERSION' — expected vX.Y.Z (e.g. v1.2.3)"
fi

# Bare version without the leading 'v', used for plugin.json.
BARE_VERSION="${VERSION#v}"

# ── file paths ────────────────────────────────────────────────────────────────

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

BIN_JAMSESH="$REPO_ROOT/plugins/jamsesh/bin/jamsesh"
ENV_EXAMPLE="$REPO_ROOT/deploy/compose/.env.example"
PLUGIN_JSON="$REPO_ROOT/plugins/jamsesh/.claude-plugin/plugin.json"

# ── pre-flight checks ─────────────────────────────────────────────────────────

info "pre-flight checks…"

# 1. Required tools
command -v jq >/dev/null 2>&1 || die "jq is required but not installed — brew install jq / apt-get install jq"

# 2. Target files exist
[[ -f "$BIN_JAMSESH"  ]] || die "file not found: $BIN_JAMSESH"
[[ -f "$ENV_EXAMPLE"  ]] || die "file not found: $ENV_EXAMPLE"
[[ -f "$PLUGIN_JSON"  ]] || die "file not found: $PLUGIN_JSON"

# 3. Expected version-pin lines exist in each file
grep -qE '^readonly JAMSESH_PLUGIN_VERSION=' "$BIN_JAMSESH" \
  || die "$BIN_JAMSESH: expected 'readonly JAMSESH_PLUGIN_VERSION=...' line not found"
grep -qE '^JAMSESH_VERSION=' "$ENV_EXAMPLE" \
  || die "$ENV_EXAMPLE: expected 'JAMSESH_VERSION=...' line not found"
jq -e '.version' "$PLUGIN_JSON" >/dev/null 2>&1 \
  || die "$PLUGIN_JSON: expected .version field not found"

# 4. Clean working tree (unstaged and staged)
if ! git -C "$REPO_ROOT" diff --quiet; then
  die "working tree is dirty — commit or stash your changes, then retry"
fi
if ! git -C "$REPO_ROOT" diff --cached --quiet; then
  die "index is dirty — commit or reset your staged changes, then retry"
fi

# 5. On main branch
CURRENT_BRANCH="$(git -C "$REPO_ROOT" branch --show-current)"
if [[ "$CURRENT_BRANCH" != "main" ]]; then
  die "not on main (currently on '$CURRENT_BRANCH') — switch to main before releasing"
fi

# 6. Tag not already local
if git -C "$REPO_ROOT" tag --list "$VERSION" | grep -q "^${VERSION}$"; then
  die "tag $VERSION already exists locally — delete it with 'git tag -d $VERSION' if this is a retry"
fi

# 7. Tag not already on origin
if git -C "$REPO_ROOT" ls-remote --tags origin "refs/tags/$VERSION" | grep -q "refs/tags/${VERSION}$"; then
  die "tag $VERSION already exists on origin — this version has already been released"
fi

info "pre-flight OK"

# ── idempotence warnings ──────────────────────────────────────────────────────

CURRENT_PLUGIN_VER="$(grep -E '^readonly JAMSESH_PLUGIN_VERSION=' "$BIN_JAMSESH" \
  | sed 's/^readonly JAMSESH_PLUGIN_VERSION="\(.*\)"/\1/')"
CURRENT_ENV_VER="$(grep -E '^JAMSESH_VERSION=' "$ENV_EXAMPLE" \
  | sed 's/^JAMSESH_VERSION=//')"
CURRENT_JSON_VER="$(jq -r '.version' "$PLUGIN_JSON")"

if [[ "$CURRENT_PLUGIN_VER" == "$VERSION" ]]; then
  warn "$BIN_JAMSESH already pins $VERSION — will overwrite (idempotent)"
fi
if [[ "$CURRENT_ENV_VER" == "$VERSION" ]]; then
  warn "$ENV_EXAMPLE already pins $VERSION — will overwrite (idempotent)"
fi
if [[ "$CURRENT_JSON_VER" == "$BARE_VERSION" ]]; then
  warn "$PLUGIN_JSON already has version $BARE_VERSION — will overwrite (idempotent)"
fi

# ── dry-run: show diffs and exit ──────────────────────────────────────────────

if [[ -n "${DRY_RUN:-}" ]]; then
  info "dry-run mode — showing what would change, touching nothing"
  printf '\n--- %s ---\n' "$BIN_JAMSESH"
  show_sed_diff "$BIN_JAMSESH" \
    '^readonly JAMSESH_PLUGIN_VERSION=.*' \
    "readonly JAMSESH_PLUGIN_VERSION=\"${VERSION}\""

  printf '\n--- %s ---\n' "$ENV_EXAMPLE"
  show_sed_diff "$ENV_EXAMPLE" \
    '^JAMSESH_VERSION=.*' \
    "JAMSESH_VERSION=${VERSION}"

  printf '\n--- %s ---\n' "$PLUGIN_JSON"
  diff -u "$PLUGIN_JSON" \
    <(jq --ascii-output --arg v "$BARE_VERSION" '.version = $v' "$PLUGIN_JSON") \
    --label "a/$PLUGIN_JSON" --label "b/$PLUGIN_JSON" || true

  printf '\n[dry-run] git add %s %s %s\n' "$BIN_JAMSESH" "$ENV_EXAMPLE" "$PLUGIN_JSON" >&2
  printf '[dry-run] git commit -m "release-prep: %s"\n' "$VERSION" >&2
  printf '[dry-run] git tag -a %s -m "Release %s"\n' "$VERSION" "$VERSION" >&2
  printf '[dry-run] git push origin main\n' >&2
  printf '[dry-run] git push origin %s\n' "$VERSION" >&2
  info "dry-run complete — nothing changed"
  exit 0
fi

# ── apply edits ───────────────────────────────────────────────────────────────

info "bumping $BIN_JAMSESH → $VERSION"
sed_inplace "$BIN_JAMSESH" \
  '^readonly JAMSESH_PLUGIN_VERSION=.*' \
  "readonly JAMSESH_PLUGIN_VERSION=\"${VERSION}\""

info "bumping $ENV_EXAMPLE → $VERSION"
sed_inplace "$ENV_EXAMPLE" \
  '^JAMSESH_VERSION=.*' \
  "JAMSESH_VERSION=${VERSION}"

info "bumping $PLUGIN_JSON → $BARE_VERSION"
jq --ascii-output --arg v "$BARE_VERSION" '.version = $v' "$PLUGIN_JSON" \
  > "${PLUGIN_JSON}.tmp" && mv "${PLUGIN_JSON}.tmp" "$PLUGIN_JSON"

# ── stage, commit, tag, push ──────────────────────────────────────────────────

info "staging files…"
run git -C "$REPO_ROOT" add \
  "$BIN_JAMSESH" \
  "$ENV_EXAMPLE" \
  "$PLUGIN_JSON"

info "committing…"
run git -C "$REPO_ROOT" commit -m "release-prep: ${VERSION}"

info "tagging…"
run git -C "$REPO_ROOT" tag -a "$VERSION" -m "Release ${VERSION}"

info "pushing main…"
run git -C "$REPO_ROOT" push origin main

info "pushing tag $VERSION…"
run git -C "$REPO_ROOT" push origin "$VERSION"

info "done — $VERSION is tagged and pushed; watch CI with: gh run watch"
