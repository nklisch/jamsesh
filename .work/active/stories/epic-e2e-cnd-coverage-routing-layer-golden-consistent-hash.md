---
id: epic-e2e-cnd-coverage-routing-layer-golden-consistent-hash
kind: story
stage: review
tags: [e2e-test, testing, portal, infra]
parent: epic-e2e-cnd-coverage-routing-layer
depends_on: [epic-e2e-cnd-coverage-cluster-fixture]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Routing Layer — Golden: Consistent-Hash Routing

## Scope

Implement `tests/e2e/golden/router_consistent_hash_test.go`.

Two subtests:

1. **`same_session_pins_to_same_pod`** — issue N (≥20) requests for the same
   `session_id` via the router; after each, assert `cluster.LeaseHolder(sessionID)`
   returns the same pod index. The invariant is about routing identity, not just
   response status — every request must be traced to a backend via LeaseHolder.

2. **`different_sessions_distribute`** — generate K (≥10) distinct session IDs,
   route one request each, collect the LeaseHolder pod index for each. Assert:
   - All K requests succeeded (2xx from router).
   - At least 2 distinct pods were used (distribution is happening; not a single
     pod handling everything). With 3 pods and 10 sessions the probability of
     all landing on one pod is astronomically small but not worth making the
     test brittle by asserting exact balance — the "at least 2" bar is stable.

## Setup

```go
pg  := postgres.Start(ctx, t, postgres.Options{})
mn  := minio.Start(ctx, t, minio.Options{})
c   := portalcluster.Start(ctx, t, portalcluster.Options{
    Pods:        3,
    Postgres:    pg,
    ObjectStore: mn,
    Router:      true, // c.RouterURL is the front door
})
```

Requests are plain REST session-scoped GETs routed through `c.RouterURL` —
e.g., `GET /api/orgs/{orgID}/sessions/{sessionID}`. No auth needed for
healthz-class probing; if the endpoint requires auth, use the authflow fixture
to obtain a token and attach a Bearer header.

## Invariant

> Same session_id consistently routes to the same pod absent re-ring events.

## Assertion targets

- `cluster.LeaseHolder(ctx, t, sessionID)` returns the same index across all N
  requests for the same session.
- At least 2 distinct pod indices appear across the K distinct session IDs.

## Acceptance criteria

- [ ] Subtest `same_session_pins_to_same_pod` passes; LeaseHolder returns the
      same pod index for all 20+ requests.
- [ ] Subtest `different_sessions_distribute` passes; at least 2 of 3 pods
      receive at least one session.
- [ ] No in-process mocks introduced; real router + real portals.
- [ ] Test does NOT assert on response body or internal call traces — only on
      routing identity (LeaseHolder) and response status code.

## Implementation notes

**File**: `tests/e2e/golden/router_consistent_hash_test.go`

**Pattern**: Both subtests spin up a full 3-pod cluster with `Router: true`,
authenticate via magic link through pod 0, then drive all session-scoped
requests through `cluster.RouterURL`. The `LeaseHolder` oracle (pg_locks
advisory lock query) is used as the sole routing-identity assertion — no
response-body inspection, no per-pod headers.

**Subtest `same_session_pins_to_same_pod`**: Creates one session (POST through
router, which itself exercises consistent-hash routing), then issues 20 GET
requests for that session through the router. After each GET, `LeaseHolder` is
queried and asserted equal to the first observed holder. Any deviation would
indicate non-deterministic routing.

**Subtest `different_sessions_distribute`**: Creates 10 distinct sessions
(each named with a timestamp suffix for uniqueness), issues one GET per session
through the router, and collects the `LeaseHolder` index for each. Asserts that
at least 2 of 3 pods appear in the holder set — the "at least 2" bar is
conservative to avoid brittleness from minor ring-balance skew, while still
proving that distribution is happening.

**Auth**: MailHog required (portal's email provider must be SMTP for magic
link sign-in). PortalExtraEnv wires MailHog's container SMTP address into
all pods, matching the smoke test pattern.

**Build/vet**: `go build ./golden/...` and `go vet ./golden/...` both pass
clean.

## Test-integrity rules

- **Park production bugs, don't hide them.** If consistent-hash routing
  demonstrates non-determinism (same session routes to different pods on
  repeated requests), park the bug via `/agile-workflow:park`; land the test
  with `t.Skip` linked to the backlog item. Do not loosen the assertion.
- **Fix bad tests in-session.** Stale fixtures or drifted helpers repaired as
  part of the stride.
- **Never game an assertion.** Do not replace `assert(holderA == holderB)` with
  `assert(true)` to make the test green.
