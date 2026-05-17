---
id: epic-e2e-tests-failure-mode
kind: feature
stage: drafting
tags: [e2e-test, testing]
parent: epic-e2e-tests
depends_on: [epic-e2e-tests-infrastructure]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# E2E Tests — Failure Mode

## Brief

Failure-mode coverage across the six standard failure categories from the
e2e-test-design taxonomy reference, scoped to jamsesh's actual failure
boundaries. Tests assert on the user-visible response — error code, error
body shape, log line, exit code — never on internal call traces.

The categories and the jamsesh-specific surface for each:

| Category | Jamsesh surface |
|---|---|
| Invalid input | OpenAPI schema violations on REST; malformed MCP tool args; malformed commit trailers; ref names outside the user's namespace |
| Missing config | Portal boots with no SMTP / no SendGrid key; portal boots with no DB DSN; `jamsesh` binary with no `CLAUDE_PLUGIN_DATA` |
| Unavailable dependency | DB down (Postgres container paused); SMTP provider returns 5xx; OAuth provider returns 5xx; git binary missing from PATH |
| Boundary values | Empty session goal; max-length comment body; max-line-range comment anchor; max-ref-name length; max commit message size accepted by pre-receive |
| Permission failures | Bearer token expired; bearer token for a different org; magic-link token reused; OAuth state reused; pushing to another user's namespace; reading another org's session |
| Interrupted operations | Push interrupted mid-pack; finalize lock acquired then process killed; magic-link request submitted but exchange skipped past TTL; WS connection drop mid-event |

## Scope

One Go spec file per category (6 specs) at `tests/e2e/failure/`. Each
file contains 5-10 subtests with `t.Run` covering the surface area
above for that category. Plus 1 Playwright spec covering visible error
states in the SPA (expired-token banner, malformed magic-link
redirect).

## Out of scope

- Chaos scenarios (network jitter, container kill mid-flight) — chaos
  feature
- Recovery semantics under fault injection — chaos feature
- Validator fuzzing — fuzzing feature (overlaps slightly with the
  invalid-input category here; this feature covers known-bad
  human-readable inputs; fuzzing covers generated/random inputs)

## Foundation references

- `docs/SECURITY.md` — permission boundaries
- `docs/PROTOCOL.md > HTTP error contract` — error response shapes
  every spec asserts against
- `docs/openapi.yaml` — schema constraints that drive the invalid-input
  spec
- `.work/active/epics/epic-e2e-tests.md` — parent mock policy

## Acceptance criteria

- [ ] 6 Go specs at `tests/e2e/failure/`, each green
- [ ] 1 Playwright spec at `tests/e2e/playwright/error-states.spec.ts`,
      green
- [ ] Every subtest asserts on a user-visible outcome (status code +
      error body shape, exit code, log line) — no `mock.WasCalledWith`
      patterns
- [ ] Every subtest's invariant is stated in plain English in a
      comment on the `t.Run` block
- [ ] Permission-failure subtests are explicitly cross-org / cross-user
      where applicable (matches the org_id invariant)
