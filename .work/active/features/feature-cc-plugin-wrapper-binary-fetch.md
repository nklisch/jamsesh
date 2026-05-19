---
id: feature-cc-plugin-wrapper-binary-fetch
kind: feature
stage: implementing
tags: [infra, plugin]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-18
updated: 2026-05-18
---

# CC plugin: wrapper-script binary fetch (replaces mirror-repo pattern)

## Brief

Replace the mirror-repo plugin distribution (`nklisch/jamsesh-cc-plugin`,
since deleted) with an inline approach: the plugin manifest at the
**main repo root** is what CC users install from, and `bin/jamsesh`
becomes a small wrapper script that fetches the matching per-arch
binary from the corresponding GitHub release asset on first run,
verifies its sha256 against `checksums.txt`, caches under
`${CLAUDE_PLUGIN_DATA}/bin/`, and execs the cached binary.

This removes operational complexity that has cost real bug-fix cycles:
no separate mirror repo, no `MARKETPLACE_DEPLOY_KEY` secret, no
one-time bootstrap, no `publish plugin to marketplace repo` CI job
(which has been failing red on every release). It also closes the
"jamsesh-cc-plugin repo doesn't exist" failure mode permanently.

The pattern is the same one `gh extension install` and `kubectl krew`
use: a small wrapper that downloads the right binary on demand from
release assets. Claude Code does not officially document or bless this
pattern, but its `bin/` directory contract (files in `bin/` get added
to Bash tool PATH; invoked as bare commands; `${CLAUDE_PLUGIN_DATA}`
exposed to subprocesses) is exactly the surface this needs.

## Strategic decisions

Locked at scope so feature-design inherits the framing.

- **Wrapper language: bash.** POSIX-ish shell. Windows users need Git Bash.
- **Verification: sha256 mandatory, cosign optional.** sha256 against
  `checksums.txt` works for every user. Cosign verify-blob runs additionally
  if cosign is on PATH.
- **Version pinning: `JAMSESH_PLUGIN_VERSION` constant in the wrapper.** Bumped
  before each release tag is pushed. `JAMSESH_PLUGIN_VERSION_OVERRIDE` env
  for advanced override.
- **Cache: `${CLAUDE_PLUGIN_DATA}/bin/jamsesh-vX.Y.Z-<os>-<arch>`.** Multiple
  versions side-by-side for rollback.
- **Mirror repo cleanup: already done.** User deleted
  `nklisch/jamsesh-cc-plugin` manually before this feature was scoped.

## Mockups

N/A — wrapper script + workflow + docs only, no UI surface.

## Architectural choice

**Single bash script at `bin/jamsesh`.** Considered alternatives:

- POSIX `sh` (more portable but harder to read; no real-world Alpine
  plugin user we know of) — rejected, not worth the complexity.
- Go cross-compiled wrapper — defeats the point (would need its own
  per-arch binary just to fetch the real per-arch binary) — rejected.
- Multi-file bash with helpers — overkill for ~80 lines of logic —
  rejected; keep it as one self-contained script.

Bash is the right fit: one file, no build step, exactly what tools like
`gh extension install` use.

## Implementation Units

### Unit 1: `bin/jamsesh` wrapper script

**File**: `bin/jamsesh`
**Story**: `feature-cc-plugin-wrapper-binary-fetch-script`

```bash
#!/usr/bin/env bash
# jamsesh — Claude Code plugin entrypoint.
#
# On first invocation, downloads the per-arch jamsesh binary matching
# JAMSESH_PLUGIN_VERSION from the corresponding GitHub release asset,
# verifies it (sha256 + optional cosign), caches under
# ${CLAUDE_PLUGIN_DATA}/bin/, and execs it. Subsequent invocations exec
# directly from cache.
#
# Override env vars (advanced):
#   JAMSESH_BIN_OVERRIDE             absolute path to an already-built binary
#   JAMSESH_PLUGIN_VERSION_OVERRIDE  pin to a different version than the constant
#   JAMSESH_PLUGIN_OWNER             override the upstream owner (default: nklisch)
#   JAMSESH_PLUGIN_VERBOSE           set non-empty to log progress to stderr

set -euo pipefail

# Bumped on every release tag (see docs/RELEASING.md). The CI workflow
# asserts this matches GITHUB_REF_NAME before the release proceeds.
readonly JAMSESH_PLUGIN_VERSION="v0.1.0"

log() {
  if [[ -n "${JAMSESH_PLUGIN_VERBOSE:-}" ]]; then
    printf 'jamsesh-wrapper: %s\n' "$*" >&2
  fi
}

die() { printf 'jamsesh-wrapper: %s\n' "$*" >&2; exit 1; }

# ── 1. Dev override ──────────────────────────────────────────────────────────
if [[ -n "${JAMSESH_BIN_OVERRIDE:-}" ]]; then
  [[ -x "${JAMSESH_BIN_OVERRIDE}" ]] || die "JAMSESH_BIN_OVERRIDE=${JAMSESH_BIN_OVERRIDE} is not executable"
  exec "${JAMSESH_BIN_OVERRIDE}" "$@"
fi

# ── 2. Resolve version + owner ───────────────────────────────────────────────
version="${JAMSESH_PLUGIN_VERSION_OVERRIDE:-${JAMSESH_PLUGIN_VERSION}}"
owner="${JAMSESH_PLUGIN_OWNER:-nklisch}"

# ── 3. Detect OS / arch ──────────────────────────────────────────────────────
uname_s=$(uname -s); uname_m=$(uname -m)
case "${uname_s}" in
  Linux)             os=linux;   ext='' ;;
  Darwin)            os=darwin;  ext='' ;;
  MINGW*|MSYS*|CYGWIN*) os=windows; ext='.exe' ;;
  *) die "unsupported OS: ${uname_s}" ;;
esac
case "${uname_m}" in
  x86_64|amd64)      arch=amd64 ;;
  arm64|aarch64)     arch=arm64 ;;
  *) die "unsupported arch: ${uname_m}" ;;
esac
[[ "${os}" == "windows" && "${arch}" == "arm64" ]] && die "no jamsesh release for windows-arm64"

binary_name="jamsesh-${os}-${arch}${ext}"

# ── 4. Cache hit? ────────────────────────────────────────────────────────────
cache_dir="${CLAUDE_PLUGIN_DATA:-${HOME}/.cache/jamsesh}/bin"
cached="${cache_dir}/jamsesh-${version}-${os}-${arch}${ext}"
if [[ -x "${cached}" ]]; then
  exec "${cached}" "$@"
fi

# ── 5. Cache miss — download, verify, install ────────────────────────────────
mkdir -p "${cache_dir}"
log "fetching ${binary_name} ${version} from github.com/${owner}/jamsesh"

tmpdir=$(mktemp -d)
trap 'rm -rf "${tmpdir}"' EXIT

release_url="https://github.com/${owner}/jamsesh/releases/download/${version}"

curl -fsSL "${release_url}/${binary_name}"        -o "${tmpdir}/${binary_name}"      || die "download failed: ${release_url}/${binary_name}"
curl -fsSL "${release_url}/checksums.txt"          -o "${tmpdir}/checksums.txt"       || die "download failed: ${release_url}/checksums.txt"

# sha256 verify
sha256_of() {
  if command -v sha256sum >/dev/null; then sha256sum "$1" | awk '{print $1}'
  elif command -v shasum    >/dev/null; then shasum -a 256 "$1" | awk '{print $1}'
  else die "neither sha256sum nor shasum found on PATH"
  fi
}
expected=$(awk -v f="${binary_name}" '$2 == f || $2 == "*"f { print $1; exit }' "${tmpdir}/checksums.txt")
[[ -n "${expected}" ]] || die "${binary_name} not present in checksums.txt"
actual=$(sha256_of "${tmpdir}/${binary_name}")
[[ "${expected}" == "${actual}" ]] || die "sha256 mismatch for ${binary_name}: want ${expected}, got ${actual}"
log "sha256 ok"

# Optional cosign verify
if command -v cosign >/dev/null; then
  if curl -fsSL "${release_url}/${binary_name}.sigstore.json" -o "${tmpdir}/${binary_name}.sigstore.json"; then
    cosign verify-blob \
      --bundle "${tmpdir}/${binary_name}.sigstore.json" \
      --certificate-identity-regexp "https://github.com/${owner}/jamsesh/.github/workflows/release.yml@refs/tags/${version}" \
      --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
      "${tmpdir}/${binary_name}" >/dev/null 2>&1 || die "cosign verification failed for ${binary_name}"
    log "cosign verified"
  else
    log "no sigstore bundle published for ${binary_name} (sha256 already validated; continuing)"
  fi
fi

# Atomic install — rename within the same fs as the cache dir
chmod +x "${tmpdir}/${binary_name}"
mv "${tmpdir}/${binary_name}" "${cached}"
log "installed at ${cached}"

exec "${cached}" "$@"
```

**Implementation Notes**:
- `set -euo pipefail` is mandatory. The wrapper's job is to be predictable; silent failures here mean the plugin silently degrades.
- `${CLAUDE_PLUGIN_DATA}` falls back to `${HOME}/.cache/jamsesh` for users
  invoking the wrapper outside of CC (e.g. testing on the CLI directly).
- The `mktemp -d` + `mv` pattern keeps the cache dir clean even on
  interrupted downloads. Renames are atomic within the same filesystem;
  using a tmpdir under `/tmp` may NOT be same-fs as the cache dir, so
  the right pattern is `tmpdir` adjacent to the final cache dir. Refine
  during implementation: `tmpdir="${cache_dir}/.tmp.$$"` then `mv` is
  guaranteed atomic.
- Concurrent invocations race; both download; both verify; both rename.
  Last-write wins on the same path — no corruption. Acceptable; no flock.
- Args pass through via `"$@"`. stdin/stdout/stderr stream through `exec`
  unchanged. Hooks reading stdin (e.g. PreToolUse) work without
  modification.
- The wrapper is invoked from FOUR call sites in the plugin:
  1. `.mcp.json` → `["bin/jamsesh", "mcp-headers"]`
  2. `hooks/hooks.json` → 6 hooks (`bin/jamsesh hook <name>`)
  3. Skill slash-commands (TBD per skill)
  4. Manual invocation from a shell on PATH
  All take args after `bin/jamsesh`; `exec ... "$@"` handles every shape.
- The checksums.txt line format varies by `sha256sum` flavor — GNU emits
  `<hash>  <file>`, BSD emits `<hash>  *<file>` (asterisk for binary
  mode). The awk pattern handles both via `$2 == f || $2 == "*"f`.

**Acceptance Criteria**:
- [ ] First invocation: downloads binary + checksums.txt; verifies sha256;
      caches at `${CLAUDE_PLUGIN_DATA}/bin/jamsesh-vX.Y.Z-<os>-<arch>`;
      execs.
- [ ] Subsequent invocations: no network, exec directly from cache.
- [ ] sha256 mismatch: hard fail with `sha256 mismatch for <file>: want X, got Y`.
- [ ] cosign on PATH: verifies sigstore bundle; hard fail if bundle exists and verify fails.
- [ ] cosign on PATH, bundle missing: log "no sigstore bundle published";
      continue (sha256 already passed).
- [ ] cosign absent: skips cosign check silently, relies on sha256.
- [ ] `JAMSESH_BIN_OVERRIDE=/usr/local/bin/jamsesh ./bin/jamsesh ...` execs that path with args.
- [ ] Args pass through: `./bin/jamsesh hook session-start <<<'payload'` reaches the
      real binary as those exact args + stdin.
- [ ] Unsupported OS/arch: clear error message (e.g. "unsupported OS: FreeBSD").
- [ ] Concurrent invocations don't corrupt the cache.
- [ ] Verbose mode (`JAMSESH_PLUGIN_VERBOSE=1`) logs progress to stderr;
      default mode is silent on success.
- [ ] Works on Linux (amd64/arm64), macOS (amd64/arm64), Windows via Git Bash
      (amd64). Smoke test at least one platform locally.

---

### Unit 2: `release.yml` — delete the marketplace job + add version-assertion guard

**File**: `.github/workflows/release.yml`
**Story**: `feature-cc-plugin-wrapper-binary-fetch-release-workflow`
**Depends on**: `feature-cc-plugin-wrapper-binary-fetch-script`

Two edits:

1. **Delete the entire `marketplace:` job** (lines 265–331 in the current
   file). The job assembled a tree of plugin files + per-arch binaries
   and pushed to `nklisch/jamsesh-cc-plugin`. No replacement needed —
   binaries already go to GitHub release assets via the existing
   `sign-and-release` job, and the wrapper fetches them on demand.

2. **Add a version-assertion step** to the `sign-and-release` job, near
   the top after `checkout`. Fails the release if `bin/jamsesh`'s
   `JAMSESH_PLUGIN_VERSION` constant doesn't match the pushed tag:

   ```yaml
   - name: assert bin/jamsesh version matches tag
     run: |
       set -euo pipefail
       expected="${GITHUB_REF_NAME}"
       actual=$(grep '^readonly JAMSESH_PLUGIN_VERSION=' bin/jamsesh \
                | sed -E 's/.*="([^"]+)".*/\1/')
       if [[ "${actual}" != "${expected}" ]]; then
         echo "::error::bin/jamsesh JAMSESH_PLUGIN_VERSION='${actual}' but tag is '${expected}'."
         echo "::error::Bump the constant in bin/jamsesh before pushing the tag (see docs/RELEASING.md)."
         exit 1
       fi
   ```

**Implementation Notes**:
- Place the assertion AFTER `actions/checkout@v4` but BEFORE the
  `actions/download-artifact` step. It's cheap (a grep) and should fail
  fast so a forgotten bump doesn't waste artifact-download time.
- Don't add the assertion to the `build` matrix job — that runs N
  times in parallel; doing the check once in `sign-and-release` is
  enough. Build never depends on the version-constant value.
- Don't try to auto-bump from CI. The constant change is content the
  tag itself is built from; auto-bumping mid-flow is confusing and
  risks a divergent commit-vs-tag state.

**Acceptance Criteria**:
- [ ] `marketplace:` job deleted entirely. `jobs:` map has: `build`,
      `sign-and-release`, `docker`. That's it.
- [ ] `sign-and-release` job has a new `assert bin/jamsesh version matches tag` step.
- [ ] On a release where the constant matches: build succeeds, no
      mention of marketplace anywhere in the workflow.
- [ ] On a release where the constant is stale (e.g. wrapper says
      `v0.1.0` but tag is `v0.1.1`): release fails fast at the
      assertion step with a clear error.
- [ ] `release.yml` valid YAML; `gh workflow view release.yml` parses cleanly.

---

### Unit 3: Documentation alignment

**Story**: `feature-cc-plugin-wrapper-binary-fetch-docs`
**Depends on**: `feature-cc-plugin-wrapper-binary-fetch-script`
**Files**:
- `docs/RELEASING.md`
- `docs/SECURITY.md`
- `README.md` (optional — add a "Install the plugin" section pointing at
  `nklisch/jamsesh` as the marketplace source)

**`docs/RELEASING.md` deltas:**

- §"Overview" step 8 (lines ~24–27): `Publishes the Claude Code plugin to the
  `<owner>/jamsesh-cc-plugin` marketplace repository` → reword to: "Plugin
  install: users install from `nklisch/jamsesh` directly. The `bin/jamsesh`
  wrapper script in the plugin fetches the matching binary from the release's
  GitHub assets on first run, verifies via sha256 + optional cosign, and
  caches under `${CLAUDE_PLUGIN_DATA}/bin/`."

- §"Cutting a release" add a new step **before** step 4 (Push the tag),
  positioned alongside the existing "Bump the compose template" step:
  
  ```markdown
  3. **Bump `bin/jamsesh` JAMSESH_PLUGIN_VERSION.** The plugin wrapper
     pins to a specific release tag. Bump before pushing:
     
     ```bash
     sed -i 's/^readonly JAMSESH_PLUGIN_VERSION=.*/readonly JAMSESH_PLUGIN_VERSION="v0.X.0"/' bin/jamsesh
     git add bin/jamsesh
     git commit -m "release-prep: bump plugin wrapper to v0.X.0"
     ```
     
     The release workflow asserts this matches the pushed tag and fails fast
     if it doesn't.
  ```
  
  Renumber subsequent steps.

- §"One-time bootstrap: marketplace plugin repo" (lines ~81–156): **delete the
  entire section**. Not needed anymore.

- §"Verifying release signatures (for users)" (lines ~158–175): unchanged. The
  cosign verify-blob example is still useful for direct binary downloaders;
  the wrapper does it automatically for plugin users when cosign is installed.

**`docs/SECURITY.md` deltas** (lines 199–214 "Supply chain and integrity"):

- Line 201–202: `The `jamsesh` binary is built reproducibly from public
  source and distributed via the marketplace repo with cryptographic
  checksums.` → `The `jamsesh` binary is built reproducibly from public
  source and distributed as GitHub release assets with cryptographic
  checksums. The plugin's `bin/jamsesh` wrapper verifies sha256 against
  the signed `checksums.txt` before exec, and additionally validates the
  cosign sigstore bundle when `cosign` is on the user's PATH.`

- Line 204–208: tighten — `Signatures are verified at install time by both
  the marketplace and the self-host install flows` → `Signatures are
  verified at fetch time by the plugin wrapper (`bin/jamsesh`) and at
  install time by the self-host install flows`.

**`README.md` (optional addition)** — if there's no current "Install the plugin"
section, add one between "Operator quickstart" and "License":

```markdown
## Install the Claude Code plugin

In Claude Code:

\`\`\`
/marketplace add nklisch/jamsesh
/plugins install jamsesh
\`\`\`

The plugin's `bin/jamsesh` wrapper downloads the matching native binary on
first use from this repo's GitHub release assets, caches it under
`${CLAUDE_PLUGIN_DATA}/bin/`, and execs. Subsequent runs skip the
download. The wrapper verifies sha256 against the release's
`checksums.txt`; if `cosign` is on your PATH it additionally verifies
the Sigstore bundle.
```

(Confirm the exact CC marketplace install command shape before writing —
the doc-research agent's response mentioned `marketplace.json` format but
not the specific user-facing command. If unclear during implementation,
note in the docs that we'll firm up the exact commands after manual
verification.)

**Implementation Notes**:
- Don't add migration-style prose ("previously the plugin shipped via..."
  etc). Per rolling-foundation, the docs describe the system as it is
  NOW. Git history is the audit trail.
- Renumbering in RELEASING.md must be careful — the file already has a
  step 2 from the compose-template feature ("Bump the compose template's
  `JAMSESH_VERSION` pin"). The wrapper-bump can be its own step 3, and
  subsequent steps shift accordingly.
- The compose-template bump step and the wrapper bump step are
  independent — both should happen before tag-push. Order between them
  doesn't matter.

**Acceptance Criteria**:
- [ ] `docs/RELEASING.md` no longer contains "marketplace repo",
      "MARKETPLACE_DEPLOY_KEY", or "jamsesh-cc-plugin" anywhere.
- [ ] `docs/RELEASING.md` "Cutting a release" has both the compose-template
      bump AND the bin/jamsesh bump as sequential numbered steps.
- [ ] `docs/SECURITY.md` line 201–208 reworded to describe GitHub-release
      asset distribution and wrapper-script verification.
- [ ] All cross-references resolve (no broken relative paths).
- [ ] `README.md` either updated with a plugin-install section OR an
      issue/note filed if the exact CC marketplace commands need
      verification first (don't ship wrong commands).

---

## Implementation Order

1. **script** — write `bin/jamsesh`. Foundational; nothing else can land
   without the file existing and the version constant being well-defined.
2. **release-workflow** + **docs** — fan out in parallel after the script
   lands. release-workflow deletes the marketplace job + adds the assertion;
   docs realigns RELEASING.md, SECURITY.md, optionally README.md.

After all three land, the next release will be one job lighter and won't
need the deleted `nklisch/jamsesh-cc-plugin` repo.

## Testing

### Wrapper script — local smoke

After implementation, test on the current dev machine:

```bash
# Cold cache, default version (v0.1.0)
rm -rf "${HOME}/.cache/jamsesh" "${CLAUDE_PLUGIN_DATA:-/dev/null}/bin"
JAMSESH_PLUGIN_VERBOSE=1 ./bin/jamsesh --version 2>&1 | head
# Expected: "fetching jamsesh-linux-amd64 v0.1.0...", "sha256 ok",
# "installed at ...", then the binary's --version output.

# Warm cache — no network
JAMSESH_PLUGIN_VERBOSE=1 ./bin/jamsesh --version 2>&1 | head
# Expected: exec from cache, no fetch messages.

# Tampered checksums — hard fail
mkdir -p /tmp/jam-test && cp ./bin/jamsesh /tmp/jam-test/
# (Manual: edit a downloaded checksums.txt to corrupt one line and re-run.)

# Dev override
JAMSESH_BIN_OVERRIDE=/usr/local/bin/echo ./bin/jamsesh hello
# Expected: prints "hello"
```

For Windows-via-Git-Bash and macOS, defer to whoever has those
environments; cover the matrix in a follow-up if needed.

### Release-workflow assertion

Local check before pushing a real tag:

```bash
# Simulate the assertion
expected="v0.1.0"
actual=$(grep '^readonly JAMSESH_PLUGIN_VERSION=' bin/jamsesh | sed -E 's/.*="([^"]+)".*/\1/')
[[ "${actual}" == "${expected}" ]] && echo OK || echo MISMATCH
```

### Docs

Visual review + `grep -rn 'jamsesh-cc-plugin\|MARKETPLACE_DEPLOY_KEY\|marketplace repo' docs/` should return empty (post-edit).

## Risks

- **Pre-v0.1.1 plugin install will need v0.1.0 binaries to exist on the
  release.** Likely already true — the `sign-and-release` job worked for
  the v0.1.0 tag (only the marketplace job failed). Verify by visiting
  https://github.com/nklisch/jamsesh/releases/tag/v0.1.0 — the asset list
  should include `jamsesh-{linux,darwin,windows}-{amd64,arm64}` (5 files)
  plus `checksums.txt`. If missing, file a follow-up to re-run the v0.1.0
  `sign-and-release` job. **Low risk; one-line gh command if missed.**

- **Race when two CC sessions invoke the wrapper simultaneously on first
  run** — both download, both verify, both rename to the same path. mv is
  atomic on POSIX filesystems so the last-write wins; no corruption. No
  flock needed; documented in implementation notes. **Accepted.**

- **Cosign verification with bundle missing**: a release might be signed
  but the wrapper looks for `<file>.sigstore.json`. If the bundle naming
  changes or is missing, the wrapper logs "no sigstore bundle published"
  and continues with just sha256. This is the correct degraded-mode
  behavior. **Accepted.**

- **Windows-arm64 unsupported**: the release matrix doesn't build
  `windows-arm64`. The wrapper detects and errors clearly. Users on
  Windows-on-ARM (rare) get a useful error. **Accepted; doc the limit.**

- **Wrapper version drift via forks**: a fork's `bin/jamsesh` might have a
  stale `JAMSESH_PLUGIN_VERSION` while the fork's owner publishes their
  own releases. The `JAMSESH_PLUGIN_OWNER` env lets users override the
  source repo. **Accepted; documented as an env var.**

- **Atomic-rename across filesystems**: if `${CLAUDE_PLUGIN_DATA}` and
  `${TMPDIR}` are on different filesystems (uncommon but possible),
  `mv` falls back to copy-then-unlink — not atomic. Mitigation in the
  implementation notes: use `${cache_dir}/.tmp.$$` instead of system
  tmpdir for the download target. **Implementation MUST follow this.**

<!-- Implementation Notes accumulate as each story lands. -->
