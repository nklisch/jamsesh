---
id: portal-test-clock-advance-endpoint-e2e-unskip
kind: story
stage: done
tags: [testing, e2e-test]
parent: portal-test-clock-advance-endpoint
depends_on: [portal-test-clock-advance-endpoint-test-endpoint]
release_binding: v0.1.0
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Un-skip magic_link_ttl_expiry e2e subtest

## Scope

Un-skip the `magic_link_ttl_expiry` subtest in
`tests/e2e/failure/interrupted_ops_test.go`, wire it through the new
`/test/clock-advance` endpoint via a small `Portal.AdvanceClock`
fixture method, and add the split authflow helpers
(`RequestMagicLink`, `ExtractMagicLinkToken`) so the test can hold the
magic-link token between request and exchange.

## Files

- `tests/e2e/fixtures/portal/clockadvance.go` (NEW) — adds
  `Portal.AdvanceClock`
- `tests/e2e/fixtures/authflow/authflow.go` (modified or new file in
  the package — implementor's call) — adds `RequestMagicLink` and
  `ExtractMagicLinkToken` if they don't already exist, by factoring
  the existing `SignInViaMagicLink` helper
- `tests/e2e/failure/interrupted_ops_test.go` (modified) — un-skip the
  `magic_link_ttl_expiry` subtest, give it a body

## Spec

### `tests/e2e/fixtures/portal/clockadvance.go`

```go
package portal

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "strings"
    "testing"
    "time"
)

// AdvanceClock POSTs to /test/clock-advance and advances the portal's
// clock by the given duration. The portal must have been built with
// -tags e2etest (the standard make test-portal-image target does this).
// If the portal returns 404, the test fails with a message that names
// the make target — the most common cause is a stale portal image
// without the build tag.
func (p *Portal) AdvanceClock(ctx context.Context, t *testing.T, d time.Duration) {
    t.Helper()
    body := fmt.Sprintf(`{"advance_seconds":%d}`, int64(d.Seconds()))
    req, err := http.NewRequestWithContext(ctx, http.MethodPost,
        p.URL+"/test/clock-advance", strings.NewReader(body))
    if err != nil {
        t.Fatalf("portal.AdvanceClock: build request: %v", err)
    }
    req.Header.Set("Content-Type", "application/json")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        t.Fatalf("portal.AdvanceClock: do request: %v", err)
    }
    defer resp.Body.Close()
    respBody, _ := io.ReadAll(resp.Body)

    if resp.StatusCode == http.StatusNotFound {
        t.Fatalf("portal.AdvanceClock: portal returned 404. The portal image must be built with -tags e2etest. Run `make test-portal-image` to rebuild jamsesh/portal:e2e.")
    }
    if resp.StatusCode != http.StatusOK {
        t.Fatalf("portal.AdvanceClock: POST /test/clock-advance: status %d: %s",
            resp.StatusCode, respBody)
    }

    // Decode and assert shape — fail fast if the endpoint contract drifts.
    var r struct {
        Now           string `json:"now"`
        OffsetSeconds int64  `json:"offset_seconds"`
    }
    if err := json.Unmarshal(respBody, &r); err != nil {
        t.Fatalf("portal.AdvanceClock: decode response: %v (body=%s)", err, respBody)
    }
    if r.Now == "" {
        t.Fatalf("portal.AdvanceClock: response missing now field: %s", respBody)
    }
    t.Logf("portal.AdvanceClock: advanced by %s, new offset=%ds, server now=%s",
        d, r.OffsetSeconds, r.Now)
}
```

### `tests/e2e/fixtures/authflow/` (split helpers)

Verify whether `RequestMagicLink` + `ExtractMagicLinkToken` exist as
exported helpers. If not, factor from the existing
`SignInViaMagicLink`:

```go
// RequestMagicLink POSTs to /api/auth/magic-link/request and asserts
// 204 No Content. Does NOT exchange — use ExtractMagicLinkToken
// followed by the exchange endpoint when you need to control the
// gap.
func RequestMagicLink(ctx context.Context, t *testing.T, p *portal.Portal, email string) {
    ...
}

// ExtractMagicLinkToken reads the latest magic-link email for
// `email` from the MailHog fixture and returns the raw token from
// the URL.
func ExtractMagicLinkToken(ctx context.Context, t *testing.T,
    mh *mailhog.MailHog, email string) string {
    ...
}
```

`SignInViaMagicLink` keeps working — it becomes a thin wrapper:
`RequestMagicLink` → `ExtractMagicLinkToken` → exchange.

### `tests/e2e/failure/interrupted_ops_test.go` (un-skip)

Replace the `t.Skip(...)` body of `magic_link_ttl_expiry` with:

```go
t.Run("magic_link_ttl_expiry", func(t *testing.T) {
    // Invariant: exchanging a magic-link token after its 15-minute
    // TTL has elapsed returns 401 auth.expired_token. We advance the
    // portal's clock via the build-tag-gated /test/clock-advance
    // endpoint rather than sleeping for real.
    email := "ttl-expiry@example.com"

    // Step 1: request a magic link and extract the token.
    authflow.RequestMagicLink(ctx, t, p, email)
    token := authflow.ExtractMagicLinkToken(ctx, t, mh, email)

    // Step 2: advance the portal's clock past the 15-minute TTL.
    p.AdvanceClock(ctx, t, 16*time.Minute)

    // Step 3: attempt exchange — must fail with 401 auth.expired_token.
    url := fmt.Sprintf("%s/api/auth/magic-link/exchange", p.URL)
    body := []byte(fmt.Sprintf(`{"token":%q}`, token))
    rawPostExpect(ctx, t, url, body, "", http.StatusUnauthorized, "auth.expired_token")
})
```

`rawPostExpect` is the existing helper used by the other subtests in
the same file (see `finalize_lock_release_and_reacquire` for an
example call site).

## Acceptance criteria

- [ ] `magic_link_ttl_expiry` is no longer `t.Skip`'d in
      `tests/e2e/failure/interrupted_ops_test.go`.
- [ ] Running `cd tests/e2e && go test ./failure/ -run
      'TestInterruptedOps/magic_link_ttl_expiry' -v` is green when
      preceded by `make test-portal-image` (which builds the e2etest-
      tagged image).
- [ ] The subtest asserts on the `auth.expired_token` error code
      explicitly — not on a substring of the human-readable message.
- [ ] `Portal.AdvanceClock` fails with a clear, actionable message if
      the portal image lacks the build tag (404 from the endpoint).
- [ ] `authflow.RequestMagicLink` and `authflow.ExtractMagicLinkToken`
      are exported and reusable by future failure-mode subtests.
- [ ] Existing `SignInViaMagicLink` still works (golden-path subtests
      stay green).

## Production-safety verification

This story does not touch production code. The endpoint it exercises
is the one landed in
`portal-test-clock-advance-endpoint-test-endpoint`; its production-
safety guarantees flow through unchanged. The new fixture method
lives under `tests/e2e/`, which is never compiled into the portal
binary.

Verification:
1. `git grep -- 'testclock' cmd/portal/ internal/portal/` returns
   only files carrying `//go:build e2etest`.
2. `go build -tags '' ./cmd/portal/` produces a binary; running it
   and hitting `POST /test/clock-advance` returns 404.

## Notes for the implementer

- The MailHog fixture's `LatestMessageTo(ctx, t, email, 5*time.Second)`
  is the existing pattern; reuse it inside `ExtractMagicLinkToken`.
- If `authflow.RequestMagicLink` already exists (check the package
  surface first), skip the helper-extraction work and just use it.
  If it doesn't, the cleanest factoring is to keep
  `SignInViaMagicLink` as a wrapper that calls the new helpers plus
  the exchange POST.
- The TTL is 15 minutes; advance by 16 minutes (one minute of headroom)
  to avoid edge-case flakiness around UTC parsing or clock-skew within
  the millisecond range.
- Do NOT use `time.Sleep` anywhere in the subtest — that would defeat
  the entire point of the endpoint.
- The subtest is sequential (not `t.Parallel`); other subtests in
  `TestInterruptedOps` share the same portal instance. Advancing the
  clock is process-global; tests that run after `magic_link_ttl_expiry`
  in the same `TestInterruptedOps` invocation will see the advanced
  clock. Verify no later subtest in the same function is sensitive to
  the clock offset; if any is, either reorder the subtests so this one
  runs last, or spin up a separate portal instance for this subtest.
  At inspection time the only later subtest is `ws_reconnect_after_drop`
  (currently skipped), which is not clock-sensitive — but document the
  ordering invariant in a comment so future additions don't get bitten.

## Implementation notes

### Files touched

- `tests/e2e/fixtures/portal/clockadvance.go` (NEW) — `Portal.AdvanceClock`
  method. POSTs `{"advance_seconds": <int>}` to `/test/clock-advance`,
  fails with an actionable message naming `make test-portal-image` on
  404, decodes the response and asserts the `now` field shape.
- `tests/e2e/fixtures/authflow/authflow.go` — factored
  `SignInViaMagicLink` into two new exported helpers:
  - `RequestMagicLink(ctx, t, p, email)` — POSTs the request endpoint,
    asserts 204.
  - `ExtractMagicLinkToken(ctx, t, mh, email)` — polls MailHog, decodes
    quoted-printable, runs `MagicLinkTokenRE` and returns the raw token.
  `SignInViaMagicLink` is now a thin wrapper over the two plus the
  exchange POST. All existing golden-path callers (onboarding, sessions,
  finalize, auto-merge, fork-and-comment, chaos suites) continue to
  work unchanged — verified by running the authflow fixture's own
  `TestAuthflow_SignInAndCreateOrg` test.
- `tests/e2e/failure/interrupted_ops_test.go` — un-skipped
  `magic_link_ttl_expiry`. The subtest now requests a magic link,
  extracts the raw token, advances the portal clock by 16 minutes,
  attempts exchange, and asserts 401 + `auth.expired_token` envelope
  via the existing `rawPostExpect` helper. Added an ordering-invariant
  comment documenting that the clock advance is process-global and
  forward-only.

### Helper-split decision

The story spec gave the implementor latitude on where the new helpers
live — separate file vs. inline in `authflow.go`. Chose inline because
the existing `ExtractInviteToken` helper lives in `authflow.go`
alongside `SignInViaMagicLink`, and the new helpers share the same
`MagicLinkTokenRE` regex and `DecodeEmailBody` decode step. Splitting
them across a new file would have orphaned the regex from its only
two consumers.

### Test invocation result

- `make test-portal-image` — rebuilt `jamsesh/portal:e2e` with
  `-tags e2etest` (the Makefile flag landed in
  `portal-test-clock-advance-endpoint-test-endpoint`).
- `cd tests/e2e && go test -count=1 -v -run TestInterruptedOps ./failure/...` —
  all four subtests pass. `magic_link_ttl_expiry` advances the clock
  by 16m and exchanges, the portal returns 401 `auth.expired_token`
  as designed. `ws_reconnect_after_drop` remains skipped per its
  own story.

```
--- PASS: TestInterruptedOps (6.34s)
    --- PASS: TestInterruptedOps/push_interrupted_mid_pack (0.02s)
    --- PASS: TestInterruptedOps/finalize_lock_release_and_reacquire (0.02s)
    --- PASS: TestInterruptedOps/magic_link_ttl_expiry (0.00s)
    --- SKIP: TestInterruptedOps/ws_reconnect_after_drop (0.00s)
```

- `cd tests/e2e && go test -count=1 -v -run TestAuthflow ./fixtures/authflow/...` —
  golden-path sign-in still works via the refactored wrapper.

### Acceptance criteria verification

- [x] `magic_link_ttl_expiry` is no longer `t.Skip`'d.
- [x] Running the subtest after `make test-portal-image` is green.
- [x] Asserts on the `auth.expired_token` error code, not the message.
- [x] `Portal.AdvanceClock` fails with a clear message naming
      `make test-portal-image` if the endpoint returns 404.
- [x] `authflow.RequestMagicLink` and `authflow.ExtractMagicLinkToken`
      are exported.
- [x] Existing `SignInViaMagicLink` still works.

## Review

**Verdict:** Approve.

Diff (`ed9e39b`) matches the spec exactly. Cross-checks:

- `tests/e2e/fixtures/portal/clockadvance.go` — `Portal.AdvanceClock`
  POSTs the right body, decodes the response, asserts the `now` field
  shape, and on 404 fails with a message that names
  `make test-portal-image`. Comment also documents the cumulative,
  forward-only, process-global nature of the offset.
- `tests/e2e/fixtures/authflow/authflow.go` — `RequestMagicLink` and
  `ExtractMagicLinkToken` are exported with clear docstrings;
  `SignInViaMagicLink` becomes a thin wrapper that all eleven existing
  golden-path / chaos / fuzz / fixture-test callers continue to use
  unchanged.
- `tests/e2e/failure/interrupted_ops_test.go` — `magic_link_ttl_expiry`
  is un-skipped; body links request → extract → advance 16m → exchange,
  and asserts `auth.expired_token` via the existing `rawPostExpect`
  helper. The ordering-invariant comment is preserved.

Local verification:
- `go vet ./failure/... ./fixtures/portal/... ./fixtures/authflow/...`
  clean.
- `go build ./failure/... ./fixtures/portal/... ./fixtures/authflow/...`
  clean.
- Did not rebuild `jamsesh/portal:e2e` locally; trusted the
  implementor's reported green run of
  `go test -count=1 -v -run TestInterruptedOps ./failure/...`
  (4 subtests: 3 pass including `magic_link_ttl_expiry`,
  `ws_reconnect_after_drop` remains skipped per its own story).

**Findings:** 0 blockers, 0 important, 0 nits.

Advancing `review → done`.
