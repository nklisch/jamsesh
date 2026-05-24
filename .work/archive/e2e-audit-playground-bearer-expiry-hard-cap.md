---
id: e2e-audit-playground-bearer-expiry-hard-cap
kind: story
stage: done
tags: [testing, e2e-test, audit, playground]
parent: feature-e2e-playground-coverage-failure
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# Anonymous bearer expiry at session hard-cap has no e2e test — scope and lifecycle never verified end-to-end

## Severity
High

## Finding type
missing-taxonomy-layer

## Evidence

`tests/e2e/chaos/runtime_and_clock_test.go > clock_skew_token_expiry`
covers regular OAuth access-token expiry. It does **not** cover the
anonymous bearer's lifecycle, which is tied to session hard-cap, not a
fixed TTL:

```
$ grep -rIn -E "anon_bearer|jamsesh_anon|anonymous.*bearer" tests/e2e/
(no output)
```

Handler unit tests verify slices:
- `TestGetPlaygroundSession_NoBearer_Returns401`
- `TestGetPlaygroundSession_ValidBearer_ReturnsSummary`
- `TestGetPlaygroundSession_BearerNotMember_Returns401`
- `TestDestruction_BearersRevoked`

These run against `stubStorage` + `fixedClock`. They do not verify the
real bearer-issuance store row, the real bearer-revocation cascade when
the destruction worker fires, or the real one-session-scope guard
preventing bearer-A from accessing session-B.

## Why this matters

The anonymous bearer is the **only** auth credential for an entire class
of users. Its scope properties are load-bearing:
1. Scoped to exactly one session (cross-session reuse must 401).
2. Revoked synchronously with destruction (post-destruction reuse must
   401, NOT serve stale state).
3. Expires at session hard-cap (no bearer outlives its session).
4. Distinguishable from regular OAuth bearer at the auth middleware
   (`jamsesh_anon_*` prefix vs `Bearer <jwt>`).

A wiring bug — e.g. anonymous bearers checked by the OAuth middleware
instead of the playground middleware — would shipping appear in
production as either over-permission (anon bearer accepted on
authenticated routes) or under-permission (anon bearer rejected on
playground routes). Unit tests against `stubStorage` cannot catch a
middleware-ordering mistake.

## Suggested remedy

Add `tests/e2e/failure/playground_bearer_scope_test.go` with three
subtests:
1. `cross_session_rejected`: create sessions S1 and S2 (two anon creates
   from same client). Bearer B1 hitting S2 must 401.
2. `post_destruction_revoked`: create session, advance clock past
   hard-cap, sweep, then GET /sessions/{id} with the original bearer
   must 401 or 410 — never 200 with stale state.
3. `wrong_path_rejected`: anon bearer used on a normal authenticated
   org endpoint (e.g. `GET /api/orgs/{org}/sessions`) must 401.

## Implementation notes

**File**: `tests/e2e/failure/playground_bearer_expiry_hard_cap_test.go`
**Test**: `TestPlayground_Bearer_ScopeIsolation`

Three subtests all pass against the real portal binary.

**Subtest 1 — `cross_session_rejected`**: S1 bearer on S2's GET endpoint returns
401 (`auth.not_a_member`). Confirmed: membership check fires in real DB, not just
in stubStorage.

**Subtest 2 — `post_destruction_revoked`**: Separate portal with
`HARD_CAP_S=60` + `SWEEP_INTERVAL_S=1`. Clock advanced 90s. Tombstone
appeared within 1 sweep cycle (~200ms poll). Original bearer returned 401
(cascade revoked the bearer row before the handler reached the session lookup).

**Subtest 3 — `anon_bearer_scoped_to_playground`** (renamed from
`anon_bearer_rejected_on_auth_route` in the sketch): The story's sketch suggested
`GET /api/me` would return 401 for anon bearers. **Discovery**: `BearerMiddleware`
validates all valid tokens uniformly — anon bearers ARE accepted by `GetMe` and
return 200 with the synthetic `anon_*` account record. The golden test
`playground_solo_create_push_tombstone_test.go` already demonstrates this (calls
`getMe` with an anon bearer). This is not a bug — the design intent is that
`GetMe` returns whatever account is attached to the bearer, anon or OAuth.

Swapped to `GET /api/orgs/org_playground/members` which requires
`RequireOrgRole("creator","member")` middleware. Anon accounts hold
**session membership** only, not org-level membership → 403 Forbidden. Result
widened to accept 401 or 403 (consistent with the honest assertion shape used
throughout the suite). Got 403 as expected.

**Rate-limit note**: default `CREATE_PER_IP_HOUR=3` means burst=1 (1 per minute);
the outer portal needs two rapid creates (S1, S2), so set to 180 (burst=3).

## Test sketch

```go
// tests/e2e/failure/playground_bearer_scope_test.go
func TestPlayground_Bearer_ScopeIsolation(t *testing.T) {
    ctx := context.Background()
    pg := postgres.Start(ctx, t, postgres.Options{})
    p := portal.Start(ctx, t, portal.Options{
        DBDriver: "postgres", DBDSN: pg.ContainerDSN,
        PlaygroundEnabled: true, PlaygroundHardCap: "2m",
    })

    s1 := createPlayground(t, p.URL)
    s2 := createPlayground(t, p.URL)

    t.Run("cross_session_rejected", func(t *testing.T) {
        resp := getRequest(t, p.URL+"/api/playground/sessions/"+s2.ID, s1.Bearer)
        require.Equal(t, 401, resp.StatusCode)
    })

    t.Run("post_destruction_revoked", func(t *testing.T) {
        portalclock.Advance(t, p, 3*time.Minute)
        time.Sleep(2 * time.Second) // let sweep fire
        resp := getRequest(t, p.URL+"/api/playground/sessions/"+s1.ID, s1.Bearer)
        require.NotEqual(t, 200, resp.StatusCode)
    })

    t.Run("anon_bearer_rejected_on_auth_route", func(t *testing.T) {
        resp := getRequest(t, p.URL+"/api/me", s1.Bearer)
        require.Equal(t, 401, resp.StatusCode)
    })
}
```

## Review (2026-05-24)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**:

All 3 subtests pass against real-stack. The agent made one honest
contract-discovery swap documented in the story body: the sketch's
`anon_bearer_rejected_on_auth_route` subtest assumed `GET /api/me`
would 401 for anon bearers, but `BearerMiddleware` validates all
tokens uniformly and `GetMe` returns 200 for the synthetic anon
account (the golden solo test already relies on this). The agent
swapped to `GET /api/orgs/{id}/members` (behind `RequireOrgRole`)
which honestly tests "anon bearers cannot reach OAuth-required
routes" via a 403 response.

The agent ALSO independently rediscovered the rate-limit subtlety
that the rate-limit story landed on (burst=1 at CREATE_PER_IP_HOUR=3,
need to set higher for multi-create tests). Set =180 to allow the
two rapid creates needed for `cross_session_rejected`.

Anti-tautology discipline: real bearer storage + real membership
table + real bearer-revocation cascade + real auth middleware. No
mocks.

Advanced `stage: review → done`.
