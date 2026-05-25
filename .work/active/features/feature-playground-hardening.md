---
id: feature-playground-hardening
kind: feature
stage: implementing
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
  (depends on clock injection so the stepClock test is deterministic)

## Design decisions

- **clock injection approach for githttp**: Add `Clock interface{ Now() time.Time }` + `realClock{}` directly in `internal/portal/githttp/clock.go`, mirroring the `playground` package shape. Do NOT import the playground package to share the type — the `per-package-clock-interface` pattern avoids cross-package coupling. `*testclock.AdvanceableClock` satisfies both structurally.
- **orphan compensation (AddSessionMember failure)**: Use best-effort revoke + delete (call `store.RevokeOAuthToken` + `store.DeleteAccountsByIDs`) rather than retry. Retry adds complexity and the destruction sweep already handles eventual cleanup; the revoke+delete collapses the orphan window from 24h to near-zero on the fast path. Log failures as warnings (best-effort, non-fatal).
- **error-envelope stripping**: The existing `WriteFromError` + `deperr.WrapDBIfTransient` pipeline already maps transient DB errors to `dep.db_unavailable` (503) with the message `"database is currently unavailable"` — no internal SQL string leaks. The audit of playground handler call sites confirms all store-failure paths return `deperr.WrapDBIfTransient(fmt.Errorf("playground: ...: %w", err))`, which the translator collapses to `ErrDBUnavailable`. No code change needed — story scope is confirming this via a targeted test.
- **anon-bearer session binding**: Use the `RequireAnonymousSessionMember` helper approach (a thin wrapper that combines `RequireAccount` + `GetSessionMember`). Do NOT add session-ID enforcement inside `Validate` itself — `Validate` is used by durable sessions too and the session context is not always available at that call site. The helper is the correct seam: callers opt in structurally.
- **ttl<=0 branch dependency on clock injection**: The `JoinPlaygroundSession` handler already uses `h.Clock.Now()` for the ttl computation (confirmed: `time.Until` is NOT used). The story's note about `time.Until` was speculative. The branch is testable with a `fixedClock` (already in `handler_test.go:29-31`) that returns a time past `HardCapAt`. Dependency on clock injection is kept to ensure the githttp `Clock` field lands first, since the activity-reset test builds on both stories.
- **worker_test boundary tests**: The `reasonFor` off-by-one fix requires updating the worker boundary tests from `"manual"` back to `"hard_cap"` / `"idle"`. The existing test comment documents the expected update, making this safe.

## Architectural choice

**In-place hardening, no new abstractions beyond `RequireAnonymousSessionMember`.**

Three options considered:

1. **Add a session-binding layer to `Validate`** — enforce `session_id` match inside `tokens.Service.Validate`, requiring callers to pass a session context. Breaking change across all callers; over-engineering for what is a playground-local concern.
2. **Add `RequireAnonymousSessionMember` helper** (chosen) — thin package-private (then exported) wrapper in `handlerauth` or inline in each playground handler. Composes existing `RequireAccount` + `GetSessionMember`. Session binding is explicit at the consumption site; durable-session handlers are unaffected.
3. **Typed error `ErrBearerSessionMismatch` in Validate** — surface mismatch as a special sentinel so callers can emit a distinctive 401. Adds a new code path to every `Validate` caller for a defense-in-depth concern. Over-kill at severity=low.

Chosen: option 2. Fits the existing `handlerauth.Require*` pattern, minimal diff, no interface changes to `tokens.Service`.

## Implementation Units

### Unit 1: Clock injection into `githttp.Handler`

**File**: `internal/portal/githttp/clock.go` (new file)
**Story**: `gate-security-githttp-receivepack-wallclock-not-injected`

```go
package githttp

import "time"

// Clock is an injectable time source. Mirrors playground.Clock so a single
// *testclock.AdvanceableClock satisfies both without import coupling.
// Per-package types carry the per-package-clock-interface pattern.
type Clock interface {
    Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

// RealClock returns the production wall-clock implementation.
func RealClock() Clock { return realClock{} }
```

**File**: `internal/portal/githttp/handler.go` (modify `Handler` struct)

Add `Clock Clock` field. Document it:
```go
// Clock is the injectable time source used for playground activity-reset
// timestamps. When nil, RealClock() is used automatically. Tests inject
// a fake clock so the reset timestamps are deterministic.
Clock Clock
```

**File**: `internal/portal/githttp/receive_pack.go` (modify activity-reset block)

```go
// effective clock: use injected or fall back to real wall clock.
clk := h.Clock
if clk == nil {
    clk = RealClock()
}
if orgID == playgroundOrgID && h.PlaygroundIdleTimeout > 0 {
    now := clk.Now().UTC()
    resetErr := h.Store.ResetSessionIdleTimer(r.Context(), store.ResetSessionIdleTimerParams{
        OrgID:                     orgID,
        SessionID:                 sessionID,
        LastSubstantiveActivityAt: now,
        IdleTimeoutAt:             now.Add(h.PlaygroundIdleTimeout),
    })
    if resetErr != nil {
        slog.WarnContext(r.Context(), "receive-pack: reset idle timer failed (best-effort)",
            "org", orgID, "session", sessionID, "err", resetErr)
    }
}
```

**File**: `internal/portal/githttp/receive_pack_test.go` (update existing test)

`TestPostReceive_PlaygroundActivityResetsIdleTimer` currently seeds the session at T0 and asserts `> beforePush` (wall-clock comparison). After injection, update `newPlaygroundPushEnv` to pass a fixed/advanceable clock and assert exact timestamps rather than `> beforePush`. The test comment at line 1203 explicitly flags this as a known update point.

**File**: `cmd/portal/main.go` (wire `Clock: githttp.RealClock()`)

**Acceptance Criteria**:
- [ ] `githttp.Handler.Clock` is a new exported field of type `githttp.Clock`.
- [ ] When `Clock` is nil, the production code falls back to `RealClock()` — no panic.
- [ ] `TestPostReceive_PlaygroundActivityResetsIdleTimer` passes with a deterministic injected clock.
- [ ] `cmd/portal/main.go` passes `githttp.RealClock()` to the handler.

---

### Unit 2: Worker `reasonFor` off-by-one fix

**File**: `internal/portal/playground/worker.go`
**Story**: `bug-playground-worker-reasonFor-off-by-one-at-exact-boundary`

```go
// reasonFor determines the end_reason string for a session based on which
// threshold has elapsed. Hard cap takes priority when both are past.
// Uses !now.Before (i.e. >=) to match the SQL predicate (hard_cap_at <= ?)
// so the tombstone reason is correct at exact-boundary expiration.
func (w *Worker) reasonFor(sess store.Session, now time.Time) string {
    if sess.HardCapAt != nil && !now.Before(*sess.HardCapAt) {
        return "hard_cap"
    }
    if sess.IdleTimeoutAt != nil && !now.Before(*sess.IdleTimeoutAt) {
        return "idle"
    }
    return "manual"
}
```

**File**: `internal/portal/playground/worker_test.go`

Update boundary-equality test cases that currently assert `"manual"` at `now == hardCap` / `now == idle` to assert `"hard_cap"` / `"idle"`. The test comment documents this expectation ("update assertions when fixing").

**Acceptance Criteria**:
- [ ] `reasonFor` returns `"hard_cap"` when `now == HardCapAt` (exact boundary).
- [ ] `reasonFor` returns `"idle"` when `now == IdleTimeoutAt` (exact boundary).
- [ ] `reasonFor` returns `"hard_cap"` over `"idle"` when both thresholds are at the same instant.
- [ ] Worker boundary tests in `worker_test.go` pass without the `"manual"` workaround comment.

---

### Unit 3: Compensating action on member-insert failure

**File**: `internal/portal/playground/handler.go` (`CreatePlaygroundSession`)
**Story**: `gate-security-playground-create-orphan-anon-account-on-member-failure`

The `handlerStore` interface needs `store.OAuthTokenStore` and a method to delete the anonymous account. Rather than widening `handlerStore` unnecessarily, use the narrowest additions:

Add to `handlerStore` interface:
```go
type handlerStore interface {
    store.SessionStore
    store.SessionMemberStore
    store.TombstoneStore
    store.OAuthTokenStore        // adds RevokeOAuthToken, GetOAuthTokenByHash
    WithTx(ctx context.Context, fn func(store.TxStore) error) error
    // DeleteAccountsByIDs is also needed for orphan cleanup.
    DeleteAccountsByIDs(ctx context.Context, ids []string) error
}
```

In `CreatePlaygroundSession`, after `IssueAnonymousSessionBearer` succeeds but before returning on `AddSessionMember` error, add best-effort cleanup:

```go
// Step 3: add the creator member row.
if err := h.Store.AddSessionMember(ctx, store.AddSessionMemberParams{
    OrgID:     ReservedOrgID,
    SessionID: sessionID,
    AccountID: accountID,
    Role:      "creator",
    JoinedAt:  now,
}); err != nil {
    // Best-effort compensation: revoke the just-issued bearer and delete
    // the anonymous account so the orphan window collapses. Failures here
    // are logged but do not change the primary error return — the caller
    // still receives a 5xx, and the destruction sweep handles any residual.
    tokenRow, getErr := h.Store.GetOAuthTokenByHash(ctx, hashToken(rawToken))
    if getErr == nil {
        _ = h.Store.RevokeOAuthToken(ctx, store.RevokeOAuthTokenParams{
            ID:        tokenRow.ID,
            RevokedAt: func() *time.Time { t := h.Clock.Now().UTC(); return &t }(),
        })
        _ = h.Store.DeleteAccountsByIDs(ctx, []string{accountID})
    } else {
        h.Logger.Warn("playground: compensating lookup failed; orphan will be swept",
            "session_id", sessionID, "err", getErr)
    }
    return nil, deperr.WrapDBIfTransient(fmt.Errorf("playground: add session member: %w", err))
}
```

Note: `hashToken` is already a package-private function in `handler.go`'s package (`playground`). Confirm it is accessible or inline the call via the tokens service. If not available in scope, use `h.Tokens` to look up by raw token via a store passthrough. Simpler: expose a `RevokeAnonymousBearer(ctx, rawToken)` method on `tokens.Service` that internalizes the hash and revoke. **Design decision**: add `RevokeAnonymousBearer` to `tokens.Service` to keep `hashToken` private to the tokens package and avoid the handler managing token internals.

Updated `tokens.Service` interface:
```go
// RevokeAnonymousBearer revokes the anonymous bearer identified by the given
// raw token and deletes its associated anonymous account. Idempotent.
// Used by playground handlers to compensate for partial-failure windows.
RevokeAnonymousBearer(ctx context.Context, rawToken string) error
```

`service_impl.go` implementation:
```go
func (s *service) RevokeAnonymousBearer(ctx context.Context, rawToken string) error {
    row, err := s.store.GetOAuthTokenByHash(ctx, hashToken(rawToken))
    if err != nil {
        if errors.Is(err, store.ErrNotFound) {
            return nil // already gone
        }
        return err
    }
    now := s.clock.Now().UTC()
    _ = s.store.RevokeOAuthToken(ctx, store.RevokeOAuthTokenParams{
        ID:        row.ID,
        RevokedAt: &now,
    })
    return s.store.DeleteAccountsByIDs(ctx, []string{row.AccountID})
}
```

Handler then calls:
```go
if err := h.Store.AddSessionMember(...); err != nil {
    if rErr := h.Tokens.RevokeAnonymousBearer(ctx, rawToken); rErr != nil {
        h.Logger.Warn("playground: orphan compensation failed; destruction sweep will clean up",
            "session_id", sessionID, "err", rErr)
    }
    return nil, deperr.WrapDBIfTransient(fmt.Errorf("playground: add session member: %w", err))
}
```

Same pattern needed in `JoinPlaygroundSession` for the joiner path (lines 311-319 in handler.go).

**Acceptance Criteria**:
- [ ] `tokens.Service` has `RevokeAnonymousBearer(ctx, rawToken) error`.
- [ ] `CreatePlaygroundSession` calls `RevokeAnonymousBearer` on `AddSessionMember` failure.
- [ ] `JoinPlaygroundSession` calls `RevokeAnonymousBearer` on `AddSessionMember` failure.
- [ ] Compensation failure is logged at Warn and does not change the error return.
- [ ] A test covers the create path: AddSessionMember fails → bearer is revoked → account is deleted.

---

### Unit 4: Error-envelope audit (no production code change)

**File**: `internal/portal/playground/handler_test.go`
**Story**: `gate-security-playground-internal-sql-errors-surface-to-anon`

Audit finding: the `WriteFromError` + `deperr.WrapDBIfTransient` pipeline already strips SQL detail strings. All playground store-failure paths use `deperr.WrapDBIfTransient`, which classifies transient errors as `deperr.ErrDB`, which `WriteFromError` maps to `httperr.ErrDBUnavailable` — message is `"database is currently unavailable"`, no SQL text.

**No production code change needed.** Story scope is a targeted test that confirms the behavior:

Add `TestCreatePlaygroundSession_DBError_DoesNotLeakSQLDetail` in `handler_test.go`:

```go
// A synthetic store failure that embeds an SQL error string must not appear
// in the HTTP response body. The response envelope must use the generic
// dep.db_unavailable message.
func TestCreatePlaygroundSession_DBError_DoesNotLeakSQLDetail(t *testing.T) {
    // Inject a failingStore that returns `sql: database is locked` on CreateSession.
    // POST /api/playground/sessions
    // Assert: response body does NOT contain "database is locked" or "sql:"
    // Assert: response body message == "database is currently unavailable"
    // Assert: response status == 503
}
```

**Acceptance Criteria**:
- [ ] Test passes confirming SQL error strings are stripped from the response envelope.
- [ ] `docs/SECURITY.md` gets a short paragraph documenting that playground anonymous endpoints return opaque 503 on DB transients (if not already present — check first).

---

### Unit 5: Anon-bearer session binding

**File**: `internal/portal/handlerauth/handlerauth.go` (or new `handlerauth/playground.go`)
**Story**: `gate-security-anon-bearer-validate-no-session-binding`

```go
// RequireAnonymousSessionMember combines RequireAccount with a session-member
// lookup for playground endpoints. It returns the authenticated account and
// member row, or an AuthFail describing the rejection reason.
//
// Use this in place of RequireAccount on every playground endpoint that
// expects an anonymous bearer: it enforces that the bearer was issued for
// the requested session_id, closing the cross-session bearer reuse gap.
func RequireAnonymousSessionMember(
    ctx context.Context,
    s sessionMemberStore,
    orgID, sessionID string,
) (*store.Account, store.SessionMember, AuthFail, bool) {
    return RequireSessionMember(ctx, s, orgID, sessionID)
}
```

In practice `RequireSessionMember` already performs `RequireAccount` + `GetSessionMember`. `RequireAnonymousSessionMember` is a thin alias that documents the intent — playground handlers switch from `RequireAccount` + manual `GetSessionMember` to a single call.

**Files to update**: `GetPlaygroundSession`, `GetPlaygroundTombstone` in `handler.go`. These two handlers call `RequireAccount` then `GetSessionMember` separately; consolidate to `RequireAnonymousSessionMember`.

**Acceptance Criteria**:
- [ ] `RequireAnonymousSessionMember` is exported from `handlerauth`.
- [ ] `GetPlaygroundSession` and `GetPlaygroundTombstone` use `RequireAnonymousSessionMember`.
- [ ] A test verifies that a bearer issued for session A is rejected on session B's endpoint.

---

### Unit 6: Coverage — creator member row assertion

**File**: `internal/portal/playground/handler_test.go`
**Story**: `idea-playground-handler-test-creator-member-assertion`

Extend `TestCreatePlaygroundSession_RepoCreateFails_ReturnsError` (lines 983-1013) to also assert the creator member row persists:

```go
// After asserting sessions is non-empty, capture the session ID:
orphanSessID := sessions[0].ID

// Assert creator member row also persists.
member, err := s.GetSessionMember(ctx, store.GetSessionMemberParams{
    OrgID:     playground.ReservedOrgID,
    SessionID: orphanSessID,
    AccountID: sessions[0].???  // need accountID
})
```

The accountID is not directly available from `ListExpiredPlaygroundSessions`. **Design decision**: query via `ListAnonymousSessionMemberIDs` (already in `PlaygroundSessionStore`) to get account IDs for the session, then call `GetSessionMember` for any returned ID.

```go
anonIDs, err := s.ListAnonymousSessionMemberIDs(ctx, playground.ReservedOrgID, orphanSessID)
if err != nil {
    t.Fatalf("ListAnonymousSessionMemberIDs: %v", err)
}
if len(anonIDs) == 0 {
    t.Error("expected creator member row to remain after CreateRepo failure, got none")
}
```

**Acceptance Criteria**:
- [ ] The test asserts both the session row AND at least one member row (via `ListAnonymousSessionMemberIDs`) remain after `CreateRepo` failure.
- [ ] Test passes on SQLite and Postgres.

---

### Unit 7: Coverage — activity-reset integration test

**File**: `internal/portal/githttp/receive_pack_test.go` (extend) or new `tests/e2e/playground_activity_reset_test.go`
**Story**: `idea-playground-abuse-caps-activity-reset-integration-test`

Given clock injection lands in Unit 1, add a focused integration test using the per-package harness (sqlite, in `receive_pack_test.go`):

```
TestPostReceive_PlaygroundActivityReset_PushResetsIdleTimer (refine existing test to use injected clock)
TestPostReceive_PlaygroundActivityReset_CommentResetsIdleTimer (new, via comments service)
TestPostReceive_PlaygroundActivityReset_FinalizeResetsIdleTimer (new, via sessions handler)
```

For the push path, extend `TestPostReceive_PlaygroundActivityResetsIdleTimer` to:
1. Advance the injected clock by 25 min.
2. Push again.
3. Assert `idle_timeout_at` advanced past original T0+30m.
4. Advance clock to 35 min from start (past original threshold).
5. Assert the session is NOT in `ListExpiredPlaygroundSessions` (timer was reset).

Comment and finalize paths use `playground-activity-reset` pattern — test their `ResetSessionIdleTimer` call via a mock store or by wiring a real store and inspecting the field post-call. The lightweight unit-test alternative (mock store asserting `ResetSessionIdleTimer` was called with correct params) is the preferred approach for non-push paths given the integration harness complexity of the comment/finalize handlers.

**Acceptance Criteria**:
- [ ] Push path: integration test covering full `sweep doesn't destroy after reset`.
- [ ] Comment path: unit test asserting `ResetSessionIdleTimer` is called on comment creation.
- [ ] Finalize path: unit test asserting `ResetSessionIdleTimer` is called on finalize attempt.
- [ ] All three call-sites have at least one test.

---

### Unit 8: Coverage — ttl<=0 branch

**File**: `internal/portal/playground/handler_test.go`
**Story**: `idea-playground-join-handler-ttl-inner-branch-coverage`

The `ttl <= 0` branch at line 296 uses `h.Clock.Now()` (confirmed — no `time.Until`). The `fixedClock` type already exists in `handler_test.go:29-31`.

Add `TestJoinPlaygroundSession_TTLZero_Returns410`:

```go
func TestJoinPlaygroundSession_TTLZero_Returns410(t *testing.T) {
    for _, h := range stores(t) {
        h := h
        t.Run(h.Name, func(t *testing.T) {
            s := h.Open(t)
            // T0: session created. HardCapAt = T0 + 1s.
            T0 := time.Now().UTC()
            env := newTestEnvWithClock(t, s, defaultCfg(), fixedClock{T0})
            // Create session with hard-cap 1s in the future.
            // ... create session ...

            // Advance clock past HardCapAt for the join call.
            env.handler.Clock = fixedClock{T0.Add(2 * time.Second)}

            // POST /api/playground/sessions/{id}/join
            resp := postJSON(t, env.srv, "/api/playground/sessions/"+sessID+"/join", "", nil)
            if resp.StatusCode != http.StatusGone {
                t.Errorf("want 410, got %d", resp.StatusCode)
            }
            var body openapi.ErrorEnvelope
            decodeJSON(t, resp, &body)
            if body.Error != "playground.session_ended" {
                t.Errorf("want playground.session_ended, got %q", body.Error)
            }
        })
    }
}
```

Note: the outer hard-cap check (line 248) will fire first if the clock is already past HardCapAt. To hit the inner branch (line 296), the clock must advance BETWEEN the outer check and the inner ttl calculation. Since the handler reads `h.Clock.Now()` twice, a `stepClock` that advances on each call would exercise the inner branch. However, `stepClock` doesn't exist yet (the story notes it was "left there" but it was absent from the test file). Use a simpler approach: create a session where the outer check passes (clock at T0, HardCapAt = T0+1s, outer check at T0 passes since `!T0.Before(T0+1s)` is false), then observe that the inner check `HardCapAt.Sub(h.Clock.Now())` may be 1s (positive), not ≤0. The ttl≤0 inner branch requires the clock to advance between the two reads.

**Design decision**: implement a `stepClock` that increments by a configured step on each `Now()` call. This was mentioned as already existing in the story body but is absent. Add it to `handler_test.go` alongside `fixedClock`. With `step = 2s` and `HardCapAt = T0+1s`, the first `h.Clock.Now()` (outer check) returns T0 (passes), the second `h.Clock.Now()` (inner check) returns T0+2s (ttl = -1s ≤ 0, branch fires).

```go
// stepClock returns T0, T0+step, T0+2*step, ... on successive Now() calls.
type stepClock struct {
    base time.Time
    step time.Duration
    n    int
}
func (c *stepClock) Now() time.Time {
    t := c.base.Add(time.Duration(c.n) * c.step)
    c.n++
    return t
}
```

**Acceptance Criteria**:
- [ ] `TestJoinPlaygroundSession_TTLZero_Returns410` exercises the `ttl <= 0` inner branch.
- [ ] Test returns 410 with `playground.session_ended`.
- [ ] `stepClock` is defined in `handler_test.go` alongside the existing `fixedClock`.

---

## Implementation Order

1. **`gate-security-githttp-receivepack-wallclock-not-injected`** — Unit 1. Clock injection into `githttp.Handler`. Unblocks Units 7 and 8's deterministic timer tests.
2. **`bug-playground-worker-reasonFor-off-by-one-at-exact-boundary`** — Unit 2. Independent; touches only `worker.go` and `worker_test.go`.
3. **`gate-security-playground-create-orphan-anon-account-on-member-failure`** — Unit 3. Adds `RevokeAnonymousBearer` to tokens service, updates handler. Independent of 1 and 2.
4. **`gate-security-playground-internal-sql-errors-surface-to-anon`** — Unit 4. Test-only; independent of all others.
5. **`gate-security-anon-bearer-validate-no-session-binding`** — Unit 5. Adds `RequireAnonymousSessionMember` helper; touches `handlerauth` and two playground handler methods. Independent.
6. **`idea-playground-handler-test-creator-member-assertion`** — Unit 6. Test-only; independent.
7. **`idea-playground-abuse-caps-activity-reset-integration-test`** — Unit 7. Depends on Unit 1 (clock injection) for deterministic push-reset test.
8. **`idea-playground-join-handler-ttl-inner-branch-coverage`** — Unit 8. Adds `stepClock`, depends on Unit 1 conceptually (for consistent clock contract) though the direct test machinery only requires the handler's existing `Clock` field.

Stories 2-6 are parallelizable after story 1 lands. Stories 7 and 8 are parallelizable with each other after story 1.

## Testing

### Unit 1 (`gate-security-githttp-receivepack-wallclock-not-injected`)
- `internal/portal/githttp/receive_pack_test.go`: Update `TestPostReceive_PlaygroundActivityResetsIdleTimer` to inject a deterministic clock and assert exact timestamp values rather than `> beforePush`.
- `internal/portal/githttp/handler_test.go`: Confirm `Clock: nil` does not panic (fallback to `RealClock()`).

### Unit 2 (`bug-playground-worker-reasonFor-off-by-one-at-exact-boundary`)
- `internal/portal/playground/worker_test.go`: Update boundary-equality assertions from `"manual"` to `"hard_cap"` / `"idle"`. Add explicit `now == threshold` cases if absent.

### Unit 3 (`gate-security-playground-create-orphan-anon-account-on-member-failure`)
- `internal/portal/playground/handler_test.go`: New test `TestCreatePlaygroundSession_MemberInsertFails_BearerRevoked`: inject `failingAddSessionMemberStore`, assert bearer revoked and account deleted post-call.
- `internal/portal/tokens/service_impl_test.go` (or `anon_bearer_test.go`): New test for `RevokeAnonymousBearer` — issues bearer, revokes, asserts `Validate` returns `ErrRevokedToken`.

### Unit 4 (`gate-security-playground-internal-sql-errors-surface-to-anon`)
- `internal/portal/playground/handler_test.go`: `TestCreatePlaygroundSession_DBError_DoesNotLeakSQLDetail` as described above.

### Unit 5 (`gate-security-anon-bearer-validate-no-session-binding`)
- `internal/portal/handlerauth/handlerauth_test.go`: New test for `RequireAnonymousSessionMember` — bearer for session A rejected on session B. Test against real sqlite store.
- `internal/portal/playground/handler_test.go`: Update `GetPlaygroundSession` tests to confirm correct 401 behavior via the new helper.

### Unit 6 (`idea-playground-handler-test-creator-member-assertion`)
- Extend `TestCreatePlaygroundSession_RepoCreateFails_ReturnsError` as described.

### Unit 7 (`idea-playground-abuse-caps-activity-reset-integration-test`)
- `internal/portal/githttp/receive_pack_test.go`: Full sweep-interaction test using injected clock.
- Unit tests for comment and finalize paths via mock stores.

### Unit 8 (`idea-playground-join-handler-ttl-inner-branch-coverage`)
- `internal/portal/playground/handler_test.go`: `TestJoinPlaygroundSession_TTLZero_Returns410` with `stepClock`.

## Risks

- **`handlerStore` interface widening (Unit 3)**: Adding `store.OAuthTokenStore` + `DeleteAccountsByIDs` to `handlerStore` increases mock surface for handler tests. Mitigated by using the `test-narrow-store-delegation` pattern — existing failing-store wrappers delegate to a real store and only override the method under test. No test mock is a full implementation.
- **`stepClock` state in tests (Unit 8)**: The `stepClock` has mutable state (`n int`). Tests that create a `stepClock` must not share the instance across subtests. The pointer receiver enforces per-instance state; callers must pass `&stepClock{...}` and not reuse across calls.
- **`RevokeAnonymousBearer` partial failure (Unit 3)**: If `RevokeOAuthToken` succeeds but `DeleteAccountsByIDs` fails, the bearer is revoked but the anon account persists (benign — no active credentials, destruction sweep cleans). If the reverse, the account is gone but the bearer is still technically valid until expiry. Both are acceptable at severity=low; log at Warn.
