---
id: feature-cc-plugin-wrapper-binary-fetch-script
kind: story
stage: done
tags: [infra, plugin]
parent: feature-cc-plugin-wrapper-binary-fetch
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-18
updated: 2026-05-18
---

# Wrapper script — `bin/jamsesh`

## Scope

Write `bin/jamsesh` per the parent feature's "Unit 1" spec — a bash wrapper
that fetches/verifies/caches/execs the matching per-arch jamsesh binary on
first invocation. Foundational story; the workflow and docs stories
cross-reference it.

## Implementation

Follow the shape specified in the parent feature, including:

- `set -euo pipefail`; clear `log()` (verbose-gated) and `die()` helpers.
- Dev override: `JAMSESH_BIN_OVERRIDE` short-circuits to the local binary.
- Version: `JAMSESH_PLUGIN_VERSION` constant at the top; reads
  `JAMSESH_PLUGIN_VERSION_OVERRIDE` env first.
- Owner: `JAMSESH_PLUGIN_OWNER` env, defaults to `nklisch`.
- OS/arch detection via `uname -s` / `uname -m`. Reject windows-arm64.
- Cache: `${CLAUDE_PLUGIN_DATA:-${HOME}/.cache/jamsesh}/bin/jamsesh-vX.Y.Z-<os>-<arch><ext>`.
- Cache hit: `exec` directly, no network.
- Cache miss: download binary + `checksums.txt`, sha256 verify, optional
  cosign verify-blob if bundle exists and cosign on PATH, atomic install.
- Args pass through via `exec "${cached}" "$@"`.
- All logs to stderr; default mode is silent on success.

**Critical implementation detail** (called out in parent's Risks section):
**Use a tmpdir adjacent to the cache dir**, not the system `/tmp`, so the
final `mv` is atomic on the same filesystem. Use
`tmpdir="${cache_dir}/.tmp.$$"` (PID-suffixed); `trap 'rm -rf "${tmpdir}"' EXIT`.

Initial `JAMSESH_PLUGIN_VERSION` value: `"v0.1.0"` (the most recent
released tag at the time this lands; the next release will bump it
per `docs/RELEASING.md`).

## Acceptance Criteria

- [ ] `bin/jamsesh` exists, is executable, starts with
      `#!/usr/bin/env bash` and `set -euo pipefail`.
- [ ] `bin/jamsesh --version` (or `bin/jamsesh -h`, any args) fetches the
      v0.1.0 binary for the local OS/arch on first run.
- [ ] sha256 against `checksums.txt` verified; mismatch → hard fail with
      `sha256 mismatch` message.
- [ ] cosign on PATH + bundle present → verifies; bundle missing →
      logs and continues with just sha256.
- [ ] Subsequent invocations use cache; no network call when
      `${cache_dir}/jamsesh-v0.1.0-<os>-<arch>` exists.
- [ ] `JAMSESH_BIN_OVERRIDE=/path ./bin/jamsesh foo bar` execs the path with `foo bar`.
- [ ] `JAMSESH_PLUGIN_VERSION_OVERRIDE=v0.1.0-rc1 ./bin/jamsesh ...` fetches that version.
- [ ] Unsupported OS / arch: clear error.
- [ ] Atomic install via same-fs `tmpdir`.
- [ ] Concurrent invocations don't corrupt cache (manual test:
      run two background instances on a cold cache).
- [ ] Args + stdin pass through cleanly (test:
      `echo hi | bin/jamsesh cat` should print `hi` if the binary
      supports a `cat` subcommand, or fail with the binary's own error).
- [ ] Test on local OS at minimum; manually note that macOS / Windows-Git-Bash
      coverage is deferred to follow-up.

## Notes

- The release-workflow story adds an assertion that this constant
  matches the pushed tag. Forgetting to bump = fast CI failure, not a
  silent stale wrapper.
- Don't add inline POSIX `sh` fallback compatibility — the file declares
  bash explicitly via shebang; `set -euo pipefail` is bash-specific anyway.
- The wrapper's stdout MUST be the binary's stdout exactly (no banners,
  no prefix). The `.mcp.json` `headersHelper` parses stdout as JSON; any
  contamination breaks MCP auth.

## Implementation Notes

- **Lines of script**: 129 lines (including comments and blank lines).
- **OS/arch tested locally**: Linux amd64 (Fedora/Nobara, kernel 6.19.2).
- **sha256 verification outcome (cold-cache run)**:
  - Downloaded `jamsesh-linux-amd64` and `checksums.txt` from
    `https://github.com/nklisch/jamsesh/releases/download/v0.1.0/`.
  - Expected hash from checksums.txt:
    `da84d324aebf772c9f9c11fc0e1e18f156b2309a33c870024a46565ef1ea820d`
  - Verification passed: `jamsesh-wrapper: sha256 ok`
- **cosign verification**: cosign not on PATH in test environment; skipped
  silently (correct degraded-mode behavior per spec).
- **Warm-cache run**: exec'd directly from
  `${HOME}/.cache/jamsesh/bin/jamsesh-v0.1.0-linux-amd64`; no network call.
  Log line: `jamsesh-wrapper: cache hit: ...`
- **Concurrent invocations**: ran two background instances on cold cache;
  instance 2 hit cache while instance 1 was still writing. Both produced
  correct `--version` output; no corruption.
- **Deviations from design pseudocode**:
  - Used `tmpdir="${cache_dir}/.tmp.$$"` (same-fs, PID-suffixed) instead
    of `mktemp -d`. This is the correct approach per the Risks section;
    the pseudocode's `mktemp -d` was a placeholder that the Implementation
    Notes explicitly said to refine.
  - Added a `log "cache hit: ${cached}"` line in the cache-hit branch for
    better verbose debuggability (not in design pseudocode; harmless addition).
  - awk string concatenation written as `("*" f)` vs design's `"*"f` —
    both are equivalent; verified with both GNU and BSD format test inputs.
- **No backlog items filed**: no unrelated test failures encountered.
