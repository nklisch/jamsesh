---
id: portal-test-clock-broaden-coverage-tokens-wiring
kind: story
stage: done
tags: [testing, testability, portal]
parent: portal-test-clock-broaden-coverage
depends_on: []
release_binding: v0.1.0
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Wire tokens.Service to the runtime clock-advance knob + un-skip clock_skew_token_expiry

## Scope

The `internal/portal/tokens` package already has an injectable `Clock`
(`tokens.NewWithClock`) but production constructs the service via
`tokens.New(store)` in `cmd/portal/main.go`, which hard-codes the real
clock. This story wires the same `*testclock.AdvanceableClock` used by
the magic-link handler into the token service, then un-skips the
`clock_skew_token_expiry` chaos subtest.

Smallest unit of work in the broaden-coverage feature; high value
because it un-blocks an explicitly-skipped chaos test.

## Files

- `cmd/portal/test_clock_advance.go` (modified, `//go:build e2etest`)
  — add `tokensClock()` returning `tokens.Clock`.
- `cmd/portal/test_clock_advance_prod.go` (modified, `//go:build !e2etest`)
  — add `tokensClock()` returning `nil`.
- `cmd/portal/main.go` (modified) — replace `tokens.New(dbStore)` with
  the conditional clock-injection pattern that mirrors the magic-link
  handler wiring.
- `tests/e2e/chaos/runtime_and_clock_test.go` (modified) — replace
  the `t.Skip` body of `clock_skew_token_expiry` with a real subtest
  that uses `Portal.AdvanceClock`.

## Spec

### `cmd/portal/test_clock_advance.go`

Add this method to `*testClockProvider`:

```go
// tokensClock returns the clock to inject into the tokens.Service.
// Implements tokens.Clock. Same underlying AdvanceableClock as
// magicLinkClock — advancing once moves both forward.
func (p *testClockProvider) tokensClock() tokens.Clock { return p.clock }
```

Import `jamsesh/internal/portal/tokens` at the top of the file.

### `cmd/portal/test_clock_advance_prod.go`

Add the production stub:

```go
// tokensClock returns nil. main.go interprets nil as "use the real
// clock" and falls back to tokens.New. The return type is the concrete
// tokens.Clock interface so the comparison against nil in main.go is
// well-defined (no typed-nil trap).
func (p *testClockProvider) tokensClock() tokens.Clock { return nil }
```

Import `jamsesh/internal/portal/tokens`.

### `cmd/portal/main.go`

Replace the existing line 249–250:

```go
tokenSvc := tokens.New(dbStore)
tokenHandler := tokens.NewHandler(tokenSvc)
```

with:

```go
// Build the token service. In e2etest builds, inject the advanceable
// clock so /test/clock-advance affects token-expiry validation
// (un-blocks tests/e2e/chaos/runtime_and_clock_test.go >
// clock_skew_token_expiry). In production builds the provider's
// tokensClock() returns nil and the real-clock constructor is used.
var tokenSvc tokens.Service
if c := testClk.tokensClock(); c != nil {
    tokenSvc = tokens.NewWithClock(dbStore, c)
} else {
    tokenSvc = tokens.New(dbStore)
}
tokenHandler := tokens.NewHandler(tokenSvc)
```

Note: `testClk` is constructed earlier (line 258) — this block must
follow it. Move the `testClk := newTestClockProvider()` line up so it
precedes the token service construction. Comment must be updated to
reflect "magic-link AND tokens" rather than "magic-link only".

### `tests/e2e/chaos/runtime_and_clock_test.go`

Replace the skip subtest body (lines 35–41) with a real test that:

1. Stands up the portal+postgres+mailhog stack.
2. Signs in via magic link, captures the access+refresh token pair and
   the `access_expires_at` timestamp.
3. Issues `GET /api/me` with the access token — must succeed (baseline).
4. Calls `p.AdvanceClock(ctx, t, 2*time.Hour)` (default `AccessTokenTTL`
   is 1 hour — advancing 2h puts the token squarely past expiry).
5. Issues `GET /api/me` with the same access token — must now return
   401 with `auth.expired_token` (or the project's expired-token
   envelope code; verify against `tokens.ErrExpiredToken`'s envelope
   translation in `httperr.WriteFromError`).
6. (Optional but cheap) Issues `POST /api/auth/refresh` with the
   refresh token — refresh TTL is longer; verify it still works OR
   advance further and verify both fail.

The subtest pattern (anti-tautology baseline) mirrors `testAutomergerPause`
in the same file. Use the existing `getMe` helper.

Note the ordering invariant: this subtest must run AFTER any
clock-sensitive subtests in the same `TestRuntimeAndClock` function
(currently `automerger_pause` doesn't read tokens past the wall clock,
so order is fine). Document it in a comment.

## Acceptance criteria

- [ ] `tokens.NewWithClock(dbStore, advanceable)` is wired in
      `cmd/portal/main.go` under the same e2etest-gated provider
      pattern used for the magic-link handler.
- [ ] Production builds (`go build ./cmd/portal/`) still use
      `tokens.New(dbStore)` — the existing real-clock constructor.
- [ ] `go build -tags '' ./cmd/portal/` succeeds and the resulting
      binary returns 404 on `POST /test/clock-advance`.
- [ ] `go build -tags e2etest ./cmd/portal/` succeeds and the
      resulting binary accepts `POST /test/clock-advance`.
- [ ] `clock_skew_token_expiry` is no longer `t.Skip`'d in
      `tests/e2e/chaos/runtime_and_clock_test.go`.
- [ ] Running `cd tests/e2e && go test ./chaos/ -run
      'TestRuntimeAndClock/clock_skew_token_expiry' -v` is green when
      preceded by `make test-portal-image`.
- [ ] The subtest asserts on the typed `auth.expired_token` envelope
      code (not on a substring of the human-readable message).

## Test approach

- Re-use existing `getMe`, `authflow.SignInViaMagicLink`, and
  `Portal.AdvanceClock` fixtures (all already in place from v1).
- Build the test as a `t.Run` subtest of `TestRuntimeAndClock`.
- Use the chaos suite's existing pattern: spin up a fresh
  postgres+mailhog+portal stack inside the subtest so the clock
  offset doesn't leak into other subtests.

## Production-safety verification

`cmd/portal/test_clock_advance.go` and `tools.go` retain their build
tags. The new `tokensClock()` method on the e2etest variant returns a
non-nil clock; the prod stub returns nil. `git grep -- 'testclock'
cmd/portal/ internal/portal/` should still only find `//go:build
e2etest`-tagged files plus the prod stub (`!e2etest`).

## Implementation notes

Landed per spec with one deviation forced by the build layout:

- `cmd/portal/test_clock_advance.go` — added `tokensClock() tokens.Clock`
  returning the shared `*testclock.AdvanceableClock`. Imported
  `jamsesh/internal/portal/tokens`. Updated the type doc to read
  "magic-link AND tokens" rather than "magic-link only".
- `cmd/portal/test_clock_advance_prod.go` — added the symmetric
  `tokensClock() tokens.Clock { return nil }` stub with matching
  typed-nil-safety doc.
- `cmd/portal/main.go` — moved `testClk := newTestClockProvider()` up
  to precede the token-service construction, then branched on
  `testClk.tokensClock()` exactly the way the magic-link wiring
  branches on `testClk.magicLinkClock()`. `tokens.NewWithClock(dbStore, c)`
  is used when the provider returns non-nil; `tokens.New(dbStore)` is
  retained for production. The variable's static type is widened to
  the `tokens.Service` interface so both constructor return shapes
  satisfy it.
- `tests/e2e/chaos/runtime_and_clock_test.go` — un-skipped
  `clock_skew_token_expiry` and implemented it as
  `testClockSkewTokenExpiry`. Stands up a fresh
  postgres+mailhog+portal stack so the clock offset stays contained;
  signs in via magic link; asserts `GET /api/me` succeeds (baseline);
  advances the clock by `AccessTokenTTL + 1 minute`; asserts the same
  token now returns `401` with `{"error": "auth.expired_token"}`.

### Deviation: TTL constant duplicated, not imported

The spec example referenced `tokens.AccessTokenTTL` directly. The
`tests/e2e/` directory is a separate Go module (`module jamsesh/tests/e2e`)
with no `replace` directive into the parent, so the
`jamsesh/internal/portal/tokens` package is not importable from the
chaos test. Settled by defining `const accessTokenTTL = 1 * time.Hour`
inside the subtest with a comment naming `tokens.AccessTokenTTL` as
the single source of truth. If that constant is ever changed, this
test will continue to advance by 1h+1m — a drift that would surface
the first time the TTL is shortened to <1h (test would still pass
because the advance is past the new TTL) or lengthened past 1h+1m
(test would fail with a 200 from `/me` because the token still has
life left). Acceptable risk — the constant is locked by SECURITY.md.

### Verification

Production safety:

- `go build ./...` — clean.
- `go build -tags e2etest ./...` — clean.
- `go test -count=1 ./cmd/portal/...` — `TestProductionBuild_HasNoTestEndpoint`
  passes (production stub of `tokensClock()` returning nil is not
  asserted by that test but doesn't need to be — the build-tag
  exclusivity is the gate).
- `go test -count=1 -tags e2etest ./cmd/portal/...` — passes.
- `go test -count=1 ./internal/portal/tokens/...` — passes (no changes
  to the tokens package itself).
- `git grep -- 'testclock' cmd/portal/ internal/portal/` — only the
  e2etest-tagged file references `testclock`; production stub does
  not. Invariant preserved.

End-to-end:

- `make test-portal-image` rebuilt `jamsesh/portal:e2e` with the new
  `-tags e2etest` binary.
- `cd tests/e2e && go test -run 'TestRuntimeAndClock/clock_skew_token_expiry'
  -v ./chaos/...` — **PASS** in 10.4s. Baseline `GET /me` succeeded;
  after `AdvanceClock(1h1m)` the same bearer returned 401
  `auth.expired_token`. The portal logged
  `advanced by 1h1m0s, new offset=3660s`.

### Access-token TTL value used

`1 * time.Hour` (the locked-by-SECURITY.md value of
`tokens.AccessTokenTTL`). Advance amount was `AccessTokenTTL + 1
minute = 1h1m` for ms-drift headroom.

## Review (2026-05-17) — Approve

### Verdict

**Approve.** Implementation matches spec; all build/test gates green;
production safety preserved.

### Verification re-run

- `go build ./...` — clean.
- `go build -tags e2etest ./...` — clean.
- `go test -count=1 ./internal/portal/tokens/... ./cmd/portal/...` — pass.
- `go test -count=1 -tags e2etest ./cmd/portal/...` — pass.
- `TestProductionBuild_HasNoTestEndpoint` — pass (404 on
  `POST /test/clock-advance` in non-tagged build).
- `git grep -- 'testclock' cmd/portal/ internal/portal/` — only the
  e2etest-tagged file and the testclock package itself; production stub
  doesn't reference it. Invariant preserved.
- `make test-portal-image` — rebuilt `jamsesh/portal:e2e`.
- `cd tests/e2e && go test -count=1 -run
  'TestRuntimeAndClock/clock_skew_token_expiry' -v ./chaos/...` — **PASS**
  in 10.3s. Portal logged `advanced by 1h1m0s, new offset=3660s`.

### Cross-checks

- `tokensClock()` on `*testClockProvider` (e2etest) returns the SAME
  `p.clock` field as `magicLinkClock()` — single shared
  `*testclock.AdvanceableClock`. Confirmed by inspection: both methods
  return the unmodified pointer; no copy, no derivation.
- `tokensClock()` (prod stub) returns typed-nil `tokens.Clock`. Safe
  against typed-nil trap because the return type is the interface, so
  `c != nil` in main.go evaluates correctly.
- `main.go` branches `tokens.NewWithClock(dbStore, c)` vs
  `tokens.New(dbStore)` on `testClk.tokensClock() != nil`. Mirrors the
  magic-link wiring exactly.
- Chaos test asserts on typed `auth.expired_token` envelope code, not
  on the human-readable message. Baseline `GET /me` precedes advance to
  rule out pre-existing misconfiguration (anti-tautology).

### TTL constant duplication

**Acceptable.** `tests/e2e/` is a separate Go module
(`module jamsesh/tests/e2e`) with no `replace` directive into the
parent, so `jamsesh/internal/portal/tokens.AccessTokenTTL` is not
importable. Three sharing mechanisms were possible: (a) a
`replace ../../ => ../../` in `tests/e2e/go.mod` to pull the parent
module in, (b) a copy-paste constant with a comment, (c) reading the
TTL from an admin endpoint at test setup. The story chose (b). For a
1-hour constant that is locked by SECURITY.md and unlikely to change,
the duplication cost is minimal and the comment naming the source of
truth is honest. (a) would expand the e2e module's dependency surface
unnecessarily, and (c) introduces a runtime dependency that defeats
the test's own invariant. The chosen approach is the right trade-off,
though a follow-up improvement could be to expose TTLs via a
`/test/config` endpoint in e2etest builds — parked here as a note,
not filed because the risk is small.

### Findings

- Blockers: 0
- Important: 0
- Nits: 0

No follow-up items filed.
