---
id: feature-cc-plugin-wrapper-binary-fetch
kind: feature
stage: drafting
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
exposed to subprocesses) is exactly the surface this needs. Confirmed
via current CC plugin docs that subpath installs are not supported and
there's no per-arch `bin` field convention.

## Strategic decisions

Locked at scope so feature-design inherits the framing. Each is
reversible at design time if it turns out wrong.

- **Wrapper language: bash.** POSIX-ish shell is the natural fit —
  small, no build step, no extra binary to ship. Works on Linux and
  macOS out of the box. Windows users need Git Bash (already a
  requirement for CC on Windows in practice). A Go wrapper would
  require its own per-arch binary, defeating the point.

- **Verification: sha256 against `checksums.txt` (mandatory), cosign
  verify-blob if cosign is on PATH (optional, advisory).** sha256 is
  the universal floor — works for every user, every platform. The
  release workflow already produces a signed `checksums.txt` (see
  `release.yml` line 119–183). Cosign keyless verification stays
  available for operators who want the supply-chain proof but is not
  mandatory at install time — most plugin users won't have cosign
  installed locally.

- **Version pinning: a `JAMSESH_PLUGIN_VERSION` constant in
  `bin/jamsesh`.** Release workflow bumps it as part of the tag-push
  flow. Users get reproducible installs — the wrapper always fetches
  the binary that matches the plugin's tagged commit. An optional
  `JAMSESH_BIN_VERSION_OVERRIDE` env var lets advanced users pin to a
  different version for testing.

- **Cache location: `${CLAUDE_PLUGIN_DATA}/bin/`** with the binary
  filename `jamsesh-vX.Y.Z-<os>-<arch>`. Keeps multiple versions side
  by side (useful for rollback). The wrapper checks for the
  pinned-version file first; downloads if missing.

- **Mirror repo cleanup: already done (user-side)**. The
  `nklisch/jamsesh-cc-plugin` repo was deleted manually before this
  feature was scoped. No implementation action needed beyond verifying
  the delete and removing all references in code/docs.

## Mockups

N/A — wrapper script + workflow + docs only, no UI surface.

<!-- Subsequent sections (Design, Implementation Notes, etc.) accumulate
as work progresses. -->
