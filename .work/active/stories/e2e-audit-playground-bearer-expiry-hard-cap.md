---
id: e2e-audit-playground-bearer-expiry-hard-cap
kind: story
stage: drafting
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
