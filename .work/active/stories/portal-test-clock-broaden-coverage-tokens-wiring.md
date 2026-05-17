---
id: portal-test-clock-broaden-coverage-tokens-wiring
kind: story
stage: implementing
tags: [testing, testability, portal]
parent: portal-test-clock-broaden-coverage
depends_on: []
release_binding: null
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
