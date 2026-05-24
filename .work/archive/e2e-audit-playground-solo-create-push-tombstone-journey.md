---
id: e2e-audit-playground-solo-create-push-tombstone-journey
kind: story
stage: review
tags: [testing, e2e-test, audit, playground]
parent: feature-e2e-playground-coverage-golden
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# Solo-creator journey (create → push → destruction → tombstone) has no e2e test

## Severity
Critical

## Finding type
journey-gap

## Evidence

`tests/e2e/golden/` contains 17 tests but none touch the anonymous-create
arc. The closest analogue is `session_join_and_push_test.go`, which uses
authenticated org sessions via `authflow.SignInViaMagicLink` —
fundamentally different code path from anonymous bearer issuance
(`POST /api/playground/sessions` mints a `jamsesh_anon_*` token without
any OAuth/email round-trip).

Unit coverage exists for slices of the arc — `TestCreatePlaygroundSession_RepoCreated`
(handler_test.go:1129-context, uses `stubStorage`),
`TestDestruction_TombstoneInsertedBeforeSessionDelete`,
`TestGetPlaygroundTombstone_AfterDestruction_Returns200` — but no test
stitches them through a real portal container + real Postgres + real git
push + real time advance + real tombstone GET. Every unit test substitutes
the storage interface (`stubStorage` at handler_test.go:36-67) and the
clock (`fixedClock` at handler_test.go:28-30).

## Why this matters

The headline pitch of v0.4.0 is "create a playground session anonymously,
push a repo, walk away, and the tombstone tells you it ran." Every step in
that pitch crosses a different module boundary (auth → handler → storage
→ post-receive → worker → tombstone READ), and the unit tests verify each
boundary in isolation against a stub. Production bugs hide between
boundaries: e.g. the destruction worker's
`store.DeleteSessionAndDependents` runs against a real DB with real FK
cascades — `TestDestruction_CascadeCorrectness` asserts on stub state, not
real Postgres FK behavior. The `tx-emit-then-fanout` pattern adds another
real-DB cross-cut that unit tests cannot fake faithfully.

## Suggested remedy

Add `tests/e2e/golden/playground_solo_create_push_tombstone_test.go` using
existing fixtures only (no new fixtures needed; the arc hits portal HTTP +
Postgres + filesystem, all of which the existing `portal` and `postgres`
fixtures cover; the `binary` fixture is available if a CLI-driven variant
is preferred). Use `portal.Options{}` extended with the playground-enable
flag(s) the portal binary already accepts in production. Drive time via
the existing `/test/clock-advance` hook used by
`tests/e2e/chaos/runtime_and_clock_test.go > clock_skew_token_expiry`.

## Test sketch

```go
// tests/e2e/golden/playground_solo_create_push_tombstone_test.go
//
// Invariant: an anonymous client can create a playground session, push a
// repo to it, advance the clock past the hard-cap, and after the
// destruction-worker sweep the tombstone endpoint returns 200 with the
// correct commit count. No real wall-clock waits; no in-process mocks.
func TestPlayground_SoloCreatePushTombstone(t *testing.T) {
    ctx := context.Background()
    pg := postgres.Start(ctx, t, postgres.Options{})
    p := portal.Start(ctx, t, portal.Options{
        DBDriver:           "postgres",
        DBDSN:              pg.ContainerDSN,
        PlaygroundEnabled:  true,
        PlaygroundHardCap:  "2m",  // short so /test/clock-advance is feasible
        PlaygroundIdleTimeout: "30s",
    })

    // 1. Anonymous create.
    resp := postJSON(t, p.URL+"/api/playground/sessions", "", nil)
    // assert: 201, body has session_id + bearer, bearer starts "jamsesh_anon_".

    // 2. Push via real git client against the session URL.
    gitclient.PushSmokeRepo(t, p.URL+"/git/playground/"+sessionID+".git", bearer)
    // assert: push exits 0; session_events table has session_pushed event.

    // 3. Advance portal clock past hard-cap, trigger sweep.
    portalclock.Advance(t, p, 3*time.Minute)
    // assert: worker sweep fires; sessions row marked destroyed.

    // 4. Tombstone reads.
    tomb := getJSON(t, p.URL+"/api/playground/sessions/"+sessionID+"/tombstone")
    // assert: 200, commits_count >= 1, members_count == 1.
}
```

## Implementation notes

Test landed at `tests/e2e/golden/playground_solo_create_push_tombstone_test.go`
— real-stack test that creates a playground session, pushes a base ref
via gitclient (using the orgID-aware URL pattern), advances the clock
past hard-cap, polls for tombstone, and asserts both the tombstone
payload (`end_reason: hard_cap`, `commits_count >= 1`) and the on-disk
absence of the bare repo via `p.Exec`.

Ships with `t.Skip` linked to TWO blocking bugs:

1. **`idea-playground-worker-clock-not-advanceable`** — same root cause
   as the abandonment story's clock-not-wired bug. The destruction
   Worker uses `playground.RealClock()` instead of the AdvanceableClock,
   so `p.AdvanceClock` past hard-cap doesn't trigger a sweep.

2. **`bug-playground-git-receive-pack-fails-with-200-hangup`** —
   surfaced by the CLI story but also affects this test's push step
   (whether via gitclient or via the binary). HTTP 200 from the
   receive-pack route but "fatal: remote end hung up" client-side.

**Anti-tautology discipline (Unit 5 application):**

- Immediate-after-create: `p.Exec(ctx, []string{"ls", repoPath})` —
  real bare-repo existence check.
- `p.Exec(ctx, []string{"ls", filepath.Join(repoPath, "hooks", "pre-receive")})`
  and same for `post-receive` — real hook installation check (the
  audit's "asserts on real disk, not stub map flip" requirement).
- Post-destruction: same Exec ls returning non-zero or empty —
  real on-disk absence.

Re-enable when both blocking bugs close.
