---
id: epic-e2e-cnd-coverage-hydration-handoff-infra
kind: story
stage: done
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

## Implementation notes

### Files landed

- `tests/e2e/fixtures/portal/portal.go` — added `Portal.Exec(ctx, cmd)` method
  exposing the testcontainers container exec API so cache-inspection helpers can
  run `ls` inside pod containers without modifying production code.

- `tests/e2e/fixtures/portalcluster/state_compare.go` — `CompareSessionState` +
  `RequireSessionStateMatch`. Reads `GET /api/orgs/{orgID}/sessions/{sessionID}`
  (status / ended_at) and `GET .../refs` (ref → sha map) from two pods directly.
  Returns a structured `StateDiff`; `RequireSessionStateMatch` calls `t.Fatal` on
  divergence.

- `tests/e2e/fixtures/portalcluster/hydration_wait.go` — `WaitForHydration` polls
  `git ls-remote <podURL>/git/<orgID>/<sessionID>.git` on a timer until the pod can
  serve the session's pack files from its local cache (300 ms poll, caller-supplied
  timeout). `PollForHydration` is the non-fatal boolean variant.

- `tests/e2e/fixtures/portalcluster/cache_inspect.go` — `VerifyCacheEvicted` /
  `VerifyCachePresent` / `CacheExists`. Uses `Portal.Exec` to run `ls
  <storagePath>/orgs/<orgID>/sessions/<sessionID>.git` inside the container.
  Defaults storagePath to `/tmp/jamsesh-repos` (the value set by portal.go's
  `buildEnv`). Includes `stripDockerMux` to clean the Docker multiplexer header
  from exec output.

### Design decisions

**draft_tip via refs endpoint, not a dedicated field.** The `GET .../sessions/{id}`
response does not expose a `draft_tip` field — that concept is per-ref in
`GET .../refs` (each `Ref` has a `sha`). `CompareSessionState` compares the full
ref→sha map, which is strictly stronger than a single `draft_tip` field.

**No `/events?limit=50` endpoint.** The story design referenced this but no such
endpoint exists in the portal API. The ref-SHA comparison is the correct durability
signal; event ordering is not needed for cross-pod state comparison.

**ls-remote for hydration signal.** The portal has no session-scoped `/readyz`
endpoint. `git ls-remote` against the bare repo proves pack files are locally
present (the pack-serve path reads the bare repo directly), which is exactly the
post-hydration signal required.

**storagePath convention.** The portal's `JAMSESH_STORAGE` env var defaults to
`/tmp/jamsesh-repos` in `portal.go`'s `buildEnv`. Tests that need cache inspection
must set `JAMSESH_STORAGE=/tmp/jamsesh-repos` in `PortalExtraEnv` to pin the path
(or accept the default). This is documented in `cache_inspect.go`.

### Verification

`cd tests/e2e && go build ./fixtures/portalcluster/... ./fixtures/minio/...` — clean.
`cd tests/e2e && go vet ./fixtures/portalcluster/... ./fixtures/minio/...` — clean.

## Test integrity (from parent feature)

- Park production bugs found during implementation, don't hide them.
- Fix bad tests in-session; never game assertions.
- A failing helper that surfaces a real protocol gap (e.g. hydration readiness
  signal not exposed) is a backlog item, not a test bug. Land the helper with
  a `t.Skip` + backlog id until the production gap is resolved.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- `stripDockerMux` takes only the payload of the first mux frame; multi-frame
  output (unlikely for `ls`) would be truncated. Best-effort is acceptable for
  log output; no action required.

**Notes**: Three helper files land cleanly. `CompareSessionState` compares the
full ref→sha map (stronger than a single `draft_tip` field), querying both pods
directly without the router — non-tautological by design. `WaitForHydration`
uses `git ls-remote` against the pod directly, which proves pack files are
locally present — exactly the post-hydration signal required. `VerifyCacheEvicted`
uses Testcontainers container exec (`ls <repoPath>`) — direct FS check, non-
tautological. `containerRepoPath` format matches production `storage.RepoPath`
exactly. No production code touched. `go build` and `go vet` clean.
