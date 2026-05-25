---
id: feature-server-secret-log-hygiene
kind: feature
stage: drafting
tags: [security, portal, plugin, logging]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-25
updated: 2026-05-25
---

# Server-side secret & log hygiene

## Brief

Bounded server-side hardenings that share a shape — bound or scrub
sensitive material at trust boundaries (OAuth callback URLs, upstream
refresh responses, cached binaries, on-disk data dirs). All four are low
severity defense-in-depth changes, all are localized to a single
file/function, and none shift architecture or contracts.

Two surfaces span portal (`internal/portal/auth/oauth.go`,
`cmd/jamsesh/portalclient/refresh.go`) and the plugin wrapper
(`plugins/jamsesh/bin/jamsesh`, `cmd/jamsesh/state/state.go`) — grouped
together because the discipline is the same and the work is small enough
that a single feature pass keeps it coherent.

## Member stories

- `gate-security-oauth-callback-log-scrubbing` —
  redact `code`/`state` query params from OAuth callback access logs
- `gate-security-refresh-error-body-leak` —
  bound upstream response body in refresh-path error wrap (~512B cap,
  strip Authorization-like fields)
- `gate-security-wrapper-cache-hit-no-resig-verify` —
  re-verify cached jamsesh binary's sha256 (and/or cosign) on every
  cache-hit before exec
- `gate-security-datadir-permissions-not-validated` —
  `os.Stat` DataDir on resolve; refuse or chmod when group/world rwx

## Approach (high level)

All four are independent — no internal sequencing. Tests should assert
the scrubbed/bound output and the failure paths (chmod refusal,
sha256 mismatch).
