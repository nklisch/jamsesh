---
id: bug-playground-worker-reasonFor-off-by-one-at-exact-boundary
kind: story
stage: drafting
tags: [bug, portal, playground]
parent: feature-playground-hardening
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-25
---

# Playground worker `reasonFor` returns `"manual"` at exact-boundary expiration instead of `"hard_cap"` / `"idle"`

## Discovered
While implementing `gate-tests-hard-cap-idle-timeout-boundary-equality`. Test
locked in current behavior; this item records the underlying off-by-one for a
later, separate fix.

## Symptom
When `now == hard_cap_at` (or `now == idle_timeout_at`), the SQL sweep
(`sqlitestore/sessions.sql.go`) IS triggered — the predicate is `hard_cap_at
<= ?`, so the boundary is included and the session gets destroyed. But the
tombstone's `reason` ends up as `"manual"` instead of `"hard_cap"` or
`"idle"`, because `worker.reasonFor` uses `now.After(...)` (strict `>`).

At exact boundary:
- SQL says expire (`<=`) → session destroyed ✓
- `reasonFor` evaluates: `now.After(hardCap)` false, `now.After(idle)` false →
  falls through to `"manual"` ✗

## Fix direction
Align `reasonFor` to `!now.Before(...)` (i.e. `>=`) so it matches the SQL.
After this change, update
`gate-tests-hard-cap-idle-timeout-boundary-equality`'s boundary tests to
expect `"hard_cap"` / `"idle"` (the test currently asserts `"manual"` and
documents the off-by-one in a comment — it's the regression sentinel for
this fix).

## Files
- `internal/portal/playground/worker.go` (`reasonFor` function)
- `internal/portal/playground/worker_test.go` (boundary tests; update
  assertions when fixing)
- Possibly `sqlitestore/sessions.sql.go` if SQL strict-`<` is preferred over
  `reasonFor >=` — pick one direction.

## Priority
Low — semantically the session is still cleaned up at the boundary; just
the tombstone reason is wrong. Affects observability/telemetry only. Not
data-correctness.
