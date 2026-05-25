---
id: feature-playground-hardening
kind: feature
stage: drafting
tags: [security, portal, playground, testing]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-25
updated: 2026-05-25
---

# Playground subsystem hardening

## Brief

Close the cluster of correctness, observability, and coverage gaps in the
ephemeral playground subsystem surfaced by recent security/test gates and
post-implementation reviews. The work is bounded — no architectural shift,
no foundation-doc changes. All children fix or harden existing behavior in
`internal/portal/playground/`, the `githttp` activity-reset path, or the
playground token-validation seam.

The single shape-touching change is injecting a `Clock` into
`githttp.Handler` so the playground's clock-injection contract covers push
activity-resets (today the push path uses `time.Now()` directly and can
silently undo clock-advance e2e tests). Two test-coverage stories depend on
that injection landing first.

## Member stories

Carry-over fixes:
- `bug-playground-worker-reasonFor-off-by-one-at-exact-boundary` —
  tombstone reason wrong at exact-boundary expiration
- `gate-security-githttp-receivepack-wallclock-not-injected` —
  inject `Clock` into `githttp.Handler`
- `gate-security-playground-create-orphan-anon-account-on-member-failure` —
  compensating action when `AddSessionMember` fails after bearer issue
- `gate-security-playground-internal-sql-errors-surface-to-anon` —
  ensure internal SQL strings do not leak to anonymous callers
- `gate-security-anon-bearer-validate-no-session-binding` —
  defense-in-depth: bind anon bearer Validate to session_id

Coverage gaps:
- `idea-playground-abuse-caps-activity-reset-integration-test` —
  push/comment/finalize each reset the idle timer (depends on clock
  injection)
- `idea-playground-handler-test-creator-member-assertion` —
  assert creator member row persists in `RepoCreateFails` test
- `idea-playground-join-handler-ttl-inner-branch-coverage` —
  cover the `ttl <= 0` inner branch in `JoinPlaygroundSession`
  (may depend on clock injection)

## Approach (high level)

Feature-design will refine this. Provisional sequencing:

1. Clock injection into `githttp.Handler` — unblocks downstream activity-reset
   integration test and (possibly) the ttl<=0 branch coverage.
2. Worker `reasonFor` boundary fix — touches `worker.go` and the locked-in
   boundary tests.
3. Compensating action on member-insert failure — adds best-effort revoke
   path in `CreatePlaygroundSession`.
4. Error-envelope audit — verify `httperr.WriteFromError` strips internal
   chains for anonymous endpoints.
5. Anon-bearer session-binding helper — `RequireAnonymousSessionMember` or
   typed `ErrBearerSessionMismatch` in `Validate`.
6. Coverage adds: activity-reset integration test, creator-member assertion,
   ttl<=0 branch.
