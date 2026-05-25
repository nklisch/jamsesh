---
id: gate-security-wrapper-cache-hit-no-resig-verify
kind: story
stage: drafting
tags: [security, plugin, infra, supply-chain]
parent: null
depends_on: []
release_binding: v0.4.1
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

## Remediation direction
Re-verify the cached binary's sha256 (and/or cosign bundle) on every
cache-hit, or store the verified hash alongside the binary and check it
before exec. Pre-existing pattern but newly visible in this bundle's
wrapper changes — if `XDG_CACHE_HOME` ever points at a multi-user-writable
location, an attacker who plants a binary there on first compromise
persists across all subsequent jamsesh invocations without re-trip-wire.
Defense-in-depth.
