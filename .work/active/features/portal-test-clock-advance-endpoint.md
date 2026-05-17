---
id: portal-test-clock-advance-endpoint
kind: feature
stage: drafting
tags: [testing, e2e-test, testability]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Portal: test-only clock-advance endpoint

## Context

The magic-link TTL is 15 minutes (`magicLinkTTL` in
`internal/portal/auth/magic_link.go`). End-to-end testing of the
`auth.expired_token` path (`ExchangeMagicLink` returns 401 when
`now.After(row.ExpiresAt)`) requires either:

- a real 15-minute wait (unacceptable in CI), or
- the ability to advance the portal's clock.

The e2e spec `tests/e2e/failure/interrupted_ops_test.go` has a
`magic_link_ttl_expiry` subtest that is currently skipped with a
reference to this backlog item.

## Proposed approach

Add a test-only HTTP endpoint — gated behind a build tag or an
env-var flag — that accepts a `POST /test/clock-advance` body:

```json
{"advance_seconds": 900}
```

The endpoint injects a fake `time.Now` provider (via dependency
injection or a package-level override guarded by the build tag) that
returns `realNow + advanceDuration`.

All business logic that calls `time.Now()` must already be
injectable; if not, thread a `func() time.Time` clock parameter
through the affected handlers as part of this story.

## Acceptance criteria

- [ ] `POST /test/clock-advance` advances the portal's clock by the
      requested number of seconds (build-tag-gated, never compiled in
      production builds)
- [ ] `magic_link_ttl_expiry` subtest in
      `tests/e2e/failure/interrupted_ops_test.go` is un-skipped and
      green
- [ ] No clock-injection code appears in production build output
      (`go build -tags ''` must not include the test endpoint)
