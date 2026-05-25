---
id: gate-security-wrapper-cache-hit-no-resig-verify
kind: story
stage: implementing
tags: [security, plugin, infra, supply-chain]
parent: feature-server-secret-log-hygiene
depends_on: []
release_binding: null
gate_origin: security
created: 2026-05-25
updated: 2026-05-25
---

# Plugin wrapper cache-hit path execs cached binary without re-verifying sha256/cosign

## Severity
Low

## Domain
Secrets & Configuration / Supply Chain

## Location
`plugins/jamsesh/bin/jamsesh:59-64`

## Evidence
```bash
cache_dir="${XDG_CACHE_HOME:-${HOME}/.cache}/jamsesh/bin"
cached="${cache_dir}/jamsesh-${version}-${os}-${arch}${ext}"
if [[ -x "${cached}" ]]; then
  log "cache hit: ${cached}"
  exec "${cached}" "$@"
fi
```

## Implementation

Restructure the wrapper so that:
1. The `sha256_of` function is defined **before** the cache-hit check (currently it appears after).
2. At install time, write `<cached>.sha256` containing the verified hex digest.
3. On cache hit, verify the sidecar exists and matches; exec only on match.
4. On sidecar absent or mismatch, emit an unconditional stderr warning and
   fall through to re-download + re-verify + re-install.

**Diff sketch** (`plugins/jamsesh/bin/jamsesh`):

Move `sha256_of` function definition to before step 4 (cache-hit section),
then replace the cache-hit block:

```bash
# ── sha256 helper (used both for cache-hit re-verify and fresh-download verify) ──
sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    die "neither sha256sum nor shasum found on PATH"
  fi
}

# ── 4. Cache hit? ────────────────────────────────────────────────────────────
cache_dir="${XDG_CACHE_HOME:-${HOME}/.cache}/jamsesh/bin"
cached="${cache_dir}/jamsesh-${version}-${os}-${arch}${ext}"
cached_sha="${cached}.sha256"
if [[ -x "${cached}" ]]; then
  if [[ -f "${cached_sha}" ]]; then
    cached_expected=$(cat "${cached_sha}")
    cached_actual=$(sha256_of "${cached}")
    if [[ "${cached_expected}" == "${cached_actual}" ]]; then
      log "cache hit verified: ${cached}"
      exec "${cached}" "$@"
    fi
    printf 'jamsesh-wrapper: WARNING: sha256 mismatch on cached binary %s (want %s, got %s) — re-downloading\n' \
      "${cached}" "${cached_expected}" "${cached_actual}" >&2
  else
    log "cache hit: missing sidecar ${cached_sha} — re-downloading"
  fi
fi
```

Then in the install block, after the existing `mv "${tmpdir}/${binary_name}" "${cached}"`:

```bash
# Write sha256 sidecar so future cache hits can re-verify without a network round-trip.
echo "${actual}" > "${cached}.sha256"
```

(The `actual` variable is already set by the download+verify block.)

**Remove the duplicate `sha256_of` function** that previously lived below the
cache-hit block — it is now defined once above.

## Acceptance Criteria
- [ ] `sha256_of` defined before the cache-hit section (no forward-reference)
- [ ] Cache hit with valid `.sha256` sidecar → binary is exec'd without re-downloading
- [ ] Cache hit with absent sidecar → logged warning, falls through to re-download
- [ ] Cache hit with tampered binary (sidecar exists but mismatch) → unconditional stderr warning, falls through to re-download
- [ ] Fresh install writes `<cached>.sha256` alongside the binary
- [ ] Existing cold-cache path (download + sha256 verify + optional cosign) is unchanged
- [ ] `JAMSESH_BIN_OVERRIDE` path is unchanged (unaffected by this diff)
