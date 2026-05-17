---
id: portal-dep-failure-error-codes
kind: story
stage: drafting
tags: [portal, documentation]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Portal dep-failure error codes not implemented (docs drift)

## Finding

Discovered during e2e implementation of
`epic-e2e-tests-failure-mode-config-and-deps`. The story body and
parent feature design referenced documented error codes like
`dep.smtp_unavailable`, `dep.db_unavailable`, etc., for runtime
dependency-failure responses.

Reality: the portal's SMTP / DB / OAuth-provider failures surface as
**plain-text HTTP 500** from the oapi-codegen strict handler's default
`ResponseErrorHandlerFunc` path. There is no JSON error envelope, no
machine-readable `dep.*` code.

This means:
1. Clients (SPA, plugin binary) can't distinguish a dep failure from
   any other 500
2. The documented error contract in `docs/PROTOCOL.md > Error response`
   isn't actually honored on dep-failure paths
3. Failure-mode e2e tests can only assert on HTTP status, not on the
   error code, weakening the contract guarantee

## Why it matters

The portal's `Error response` contract promises a `{code, message}`
JSON envelope on all errors. Dep failures break that promise.

For an MVP this might be acceptable (dep failures are rare and the
SPA's "Something went wrong" fallback handles them). For
production hardening it's a real gap — operators debugging an outage
benefit from `dep.smtp_unavailable` vs. `dep.db_unavailable` instead
of opaque 500s.

## Suggested resolutions (pick one)

1. **Implement the codes** — wrap the strict-handler default error path
   to translate dep-failure errors into typed envelopes with
   `code: "dep.<service>_unavailable"`. Small portal change in
   `cmd/portal/main.go` and the affected handlers.
2. **Update docs** — remove `dep.*` codes from `docs/PROTOCOL.md` and
   document the actual behavior (plain-text 500 on dep failures with
   structured logs at the portal for operator debugging).
3. **Both** — implement the typed codes for the auth + magic-link
   surfaces (the most user-visible), document the rest as plain 500.

## Acceptance criteria

- [ ] `docs/PROTOCOL.md > Error response` accurately documents what the
      portal returns on dep failures (either implements the typed codes
      OR documents the plain-500 behavior)
- [ ] `tests/e2e/failure/config_and_deps_test.go` is updated to assert
      on the chosen contract (status + code, OR status only with a
      comment pointing at the documented behavior)
- [ ] Foundation-doc rolling-forward principle observed — no
      "previously this was..." notes
