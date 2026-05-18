---
id: epic-e2e-cnd-coverage-hydration-handoff-infra
kind: story
stage: implementing
tags: [e2e-test, testing, portal, infra]
parent: epic-e2e-cnd-coverage-hydration-handoff
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Hydration-Handoff Infra — State-Compare + WaitForHydration + VerifyCacheEvicted Helpers

## Scope

Land the three test-helper functions that every other hydration-handoff story
depends on. No new test assertions are added here — this story is purely
fixture infrastructure.

## Implementation units

### 1. `tests/e2e/fixtures/portalcluster/state_compare.go`

A `CompareSessionState` function that calls `GET /api/orgs/{orgID}/sessions/{sessionID}`
plus `GET /api/orgs/{orgID}/sessions/{sessionID}/events?limit=50` on two
different pods and deep-compares:

- `draft_tip` — the git SHA the session's draft ref points to
- `recent_events` — the last N event IDs and types (order-insensitive for the
  event IDs, order-sensitive for the causal sequence)
- `finalize_state` (if the session has been finalized — check `ended_at` field)

Returns a structured diff (not a boolean) so callers can log what diverged.
The test-side helper `RequireSessionStateMatch(ctx, t, c, orgID, sessionID,
podA, podB)` wraps `CompareSessionState` and calls `t.Fatal` on any
divergence with a human-readable diff.

**Why not MCP `query_session_state`?** The MCP client is user-auth-scoped and
carries user-specific filtering (open conflicts for *me*, etc.). Handoff tests
need raw session state that is pod-local and auth-neutral. REST is the right
surface here.

**Assertion philosophy.** The draft tip comparison is the primary state-preservation
invariant. Events and finalize state are secondary. If `draft_tip` diverges,
that is a Critical durability bug — park via `/agile-workflow:park`.

### 2. `tests/e2e/fixtures/portalcluster/hydration_wait.go`

`WaitForHydration(ctx, t, pod, orgID, sessionID, accessToken, timeout)` —
polls `GET /api/orgs/{orgID}/sessions/{sessionID}/readyz` (or falls back to
`GET /readyz` if no session-scoped variant exists) until the session can serve
a read request with 200 OK, or until `timeout` elapses.

If the portal exposes a session-specific readiness signal (e.g. a header
`X-Session-Hydrated: true` on the first successful read), prefer that.
Otherwise use a lightweight `git ls-remote` via gitclient pointing at the pod
to confirm the session's refs are visible — this proves the bare-repo cache is
populated without requiring a portal API extension.

**Design decision (autopilot):** Use `gitclient.LsRemote` against the pod
directly (not through the router). LsRemote succeeds iff the portal can serve
the session's pack files from its local cache — this is exactly the post-
hydration readiness signal we want. No new portal endpoint needed.

SLO: `timeout` is a caller parameter. Golden tests pass 30s; chaos tests pass
45s.

### 3. `tests/e2e/fixtures/portalcluster/cache_inspect.go`

`VerifyCacheEvicted(ctx, t, c, podIndex, sessionID)` — verifies that a
pod's local bare-repo cache for `sessionID` has been cleared after eviction.

Implementation: use the Testcontainers container-exec API to run
`ls /var/jamsesh/cache/sessions/<sessionID>/` inside the pod's container.
If the directory is absent or empty, the cache is evicted. The cache path
is derived from the portal's `JAMSESH_STORAGE_PATH` env var (default
`/var/jamsesh`) — the fixture must read this from the container's env at
test time, or accept the path as a parameter defaulting to `/var/jamsesh`.

**Design decision (autopilot):** Direct `docker exec ls` via Testcontainers
over a test-only debug endpoint. No production code changes required.
The exec approach does require that the portal's `storage/` path is known —
pass it as `PortalExtraEnv: {"JAMSESH_STORAGE_PATH": "/var/jamsesh"}` in
tests that need eviction inspection so the path is deterministic.

## Acceptance criteria

- [ ] `CompareSessionState` / `RequireSessionStateMatch` compiles and is
      importable by other stories
- [ ] `WaitForHydration` compiles and correctly blocks until a pod can serve
      a git ls-remote on the session
- [ ] `VerifyCacheEvicted` compiles; in a manual smoke run against a drained
      pod it reports "evicted" correctly
- [ ] No new production portal endpoints added
- [ ] No in-process mocks

## Test integrity (from parent feature)

- Park production bugs found during implementation, don't hide them.
- Fix bad tests in-session; never game assertions.
- A failing helper that surfaces a real protocol gap (e.g. hydration readiness
  signal not exposed) is a backlog item, not a test bug. Land the helper with
  a `t.Skip` + backlog id until the production gap is resolved.
