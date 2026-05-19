---
id: bug-e2e-suite-systemic-failures-post-v0.1.0
kind: story
stage: review
tags: [bug, testing, e2e-test, triage]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-18
updated: 2026-05-18
---

# E2E suite — systemic failures post-v0.1.0 (triage)

## Brief

The `e2e` GitHub workflow has been failing red on every push since v0.1.0
shipped (workflow run `26070530857` is representative). Multiple distinct
failure modes across `tests/e2e/chaos`, `tests/e2e/scaffolding`, and
`tests/e2e/failure-mode`. **This is a triage story** — the deliverable is not
"fix the e2e suite" but "root-cause each cluster of failures and produce a
focused child bug item per real root cause, so they can be scoped and
fixed individually."

## Observed symptoms (from one representative run)

1. **Router 503 "no backends available"** across multiple chaos tests:
   - `TestCrossPodClockSkew` — `createLeaseSkewSession: want 201; got 503`
     (`tests/e2e/chaos/cross_pod_clock_skew_test.go:99`)
   - `TestHandoffUnderObjectStorageChaos` — same shape
     (`tests/e2e/chaos/handoff_under_object_storage_chaos_test.go`)
   - `TestHandoffUnderPodKill` — `podKillCreateSession: want 201; got 503`
     (`tests/e2e/chaos/handoff_under_pod_kill_test.go:98`)
   - `TestLeaseHolderKilled` — `createLeaseKillSession: want 201; got 503`
     (`tests/e2e/chaos/lease_holder_killed_test.go:92`)
   - `TestRouterPodDisappears` (multiple sub-cases) — `expected: 201,
     actual: 503` (`tests/e2e/chaos/router_pod_disappears_test.go:104, 214, 441`)

   Hypothesis: router-backend wiring isn't fully up by the time the test
   posts. Could be a fixture timing race, a health-check threshold change,
   or a regression in the router's "available backend" computation —
   possibly related to `staticDiscoverer`/`hint-cache` work that landed
   around v0.1.0.

2. **WebSocket 401 on upgrade**:
   - `TestRuntimeAndClock/automerger_pause` — `wsclient: dial
     ws://...: failed to WebSocket dial: expected handshake response status
     code 101 but got 401` (`tests/e2e/.../runtime_and_clock_test.go:36`)

   Hypothesis: auth-ticket flow regression — the WS upgrade is rejecting
   bearer/ticket auth. Related to recent
   `gate-security-ws-bearer-token-ticket-flow` work.

3. **Rate-limit leakage between subtests**:
   - `TestInterruptedOps/magic_link_ttl_expiry` —
     `POST /api/auth/magic-link/request: status 429 (want 204):
     "rate_limited"` (`tests/e2e/.../interrupted_ops_test.go:277`)

   Hypothesis: the auth rate limiter isn't being reset/isolated between
   subtests in the same parent test. The rate-limit window leaks across
   cases.

4. **Test scaffolding failure** — `FAIL jamsesh/tests/e2e/scaffolding`
   (8.435s). MinIO testcontainers boots with a warning about default
   credentials but appears to come up — root failure inside scaffolding
   not yet localized. May be a precondition for cluster (1).

## Investigation plan

One investigation stride per cluster, in this order (root-causes first,
test-isolation last):

- **Cluster D first**: scaffolding base failure. If the scaffolding suite
  can't run cleanly, the chaos cluster's results may be downstream noise.
  Localize the failure, decide if it's a fix or a separate child bug.
- **Cluster A**: router 503s. Run one chaos test locally with full
  router/portal logs captured. Determine whether the 503 is a true
  "no healthy backend" or a hint-cache miss. Look at recent
  `staticDiscoverer` / `hint-cache` work for the likely regression
  point.
- **Cluster B**: WS 401. Trace the ticket-flow path against the WS
  handshake. Identify the rejected branch (missing ticket, expired
  ticket, signature mismatch).
- **Cluster C**: rate-limit isolation. Verify whether the rate limiter
  has a test-mode reset hook, and whether `TestInterruptedOps` uses it
  between subtests.

## Deliverable

For each cluster:
- A short root-cause writeup (added to this story body under a
  `## Cluster X — root cause` section).
- A focused child bug item created at `.work/active/stories/` (or
  `.work/backlog/` if low-priority) with the actual fix scope. The new
  items get their own slugs (e.g. `bug-router-503-no-backends-startup-race`,
  `bug-ws-ticket-flow-401-regression`, etc.) — they are NOT children of
  this story (no `parent:` field), they're siblings; this story's purpose
  is purely to spawn them.

## Acceptance criteria

- [ ] Cluster A root-caused and a child bug item filed (or confirmed
      duplicate of an existing item).
- [ ] Cluster B root-caused and a child bug item filed.
- [ ] Cluster C root-caused and a child bug item filed.
- [ ] Cluster D root-caused and either fixed here (if trivial — e.g. fixture
      env var) or filed as a child bug item.
- [ ] This story body updated with a per-cluster root-cause summary linking
      to the spawned child items.
- [ ] No code changes in this story beyond cluster D (if that one turns out
      to be a trivial fixture fix). The substantive fixes live in the
      spawned child stories.

## Notes

- The CI for `e2e` will continue failing red until the spawned child bugs
  land. That's accepted — the workflow has been red since v0.1.0 and a
  triage story doesn't change that.
- If during investigation two clusters turn out to share a root cause
  (e.g. a fixture timing bug affects both A and D), file one combined
  child bug item rather than two duplicates and note the overlap here.

---

## Cluster A — root cause

**Hypothesis confirmed**: the router's static discoverer fires an immediate
`/readyz` probe on goroutine startup (before the first tick), clearing the
pre-seeded ring if portal pods haven't yet passed their readyz checks.
`cmd/jamsesh-router/main.go` seeds the ring from static config, then
immediately starts the discovery goroutine whose first action is to probe
`/readyz` on all backends. The portal fixture waits for `/healthz` (instant,
no-dep) before returning — so the portal is "healthy" but its `/readyz` (which
runs a Postgres ping and `os.Stat`) may not yet pass. The discoverer calls
`publish([])`, emptying the ring. All subsequent router requests get 503.

**Child item**: `bug-router-503-readyz-evicts-seeded-ring`

Two-part fix: (A) delay first probe in `static.go` until after the first tick
interval, and (B) poll portal readyz in the router fixture before starting the
router container. Both are needed for robustness.

---

## Cluster B — root cause

**Hypothesis confirmed**: the `gate-security-ws-bearer-token-ticket-flow` story
(commit `2301005`) shipped in v0.1.0 changed the WS subprotocol from
`jamsesh.bearer.<token>` to `jamsesh-ticket.<ticket>`. The gateway now rejects
the old format with 401 (no compat path). The `wsclient` e2e fixture
(`tests/e2e/fixtures/wsclient/wsclient.go:98`) still sends the old format
(`jamsesh.bearer.<bearer>`) — it was not updated in the commit (verified via
`git log`). Every `wsclient.Connect` call fails with 401 at the upgrade step.

**Child item**: `bug-wsclient-fixture-sends-bearer-not-ticket`

Fix: update `wsclient.dial` to POST `/api/auth/ws-ticket` to obtain a
single-use ticket, then dial with `jamsesh-ticket.<ticket>`. Test-fixture-only
change; no production code touched.

---

## Cluster C — root cause

**Hypothesis confirmed**: `TestInterruptedOps` runs multiple subtests against
a single shared portal at `127.0.0.1`. The `magic-link/request` rate limiter
is 3/min per source IP, in-process, with no reset between subtests. By the
time the `magic_link_ttl_expiry` subtest runs, prior subtests have already
consumed all 3 burst tokens (`push_interrupted_mid_pack` = 1, `finalize_lock_release_and_reacquire`
= 2, cumulative = 3). The 4th request (`magic_link_ttl_expiry`'s own call)
hits 429. Rate-limiter TTL is 1 hour; no GC fires during the test run.

**Child item**: `bug-rate-limit-leaks-across-interrupted-ops-subtests`

Recommended fix: set `JAMSESH_AUTH_RATE_LIMIT_ENABLED=false` in the
`TestInterruptedOps` portal's `ExtraEnv`. The test exercises interrupted-op
semantics, not rate-limiting; disabling it in-test is correct scope.

---

## Cluster D — root cause

**Downstream of Cluster A**. `TestClusteredSmoke` in `tests/e2e/scaffolding`
starts a 3-pod cluster with a router, then calls `createSessionViaRouter` (line
94), which POSTs to the router. This request has no session ID so it uses the
round-robin fallback — but the fallback also returns 503 when `len(pods) == 0`.
The ring is empty because the Cluster A bug cleared it on first probe. The
8.435s failure time is consistent with testcontainers startup succeeding then
the first router request hitting the empty ring.

**No separate child item filed.** Fixing `bug-router-503-readyz-evicts-seeded-ring`
fixes Cluster D as well, since the root cause is identical. Verified: no other
test in `tests/e2e/scaffolding` is the actual failure vector (`TestPortalHealthz`
and `TestPortalImageHealthz` skip on missing router image; `TestE2EModuleBuilds`
is a no-op).
