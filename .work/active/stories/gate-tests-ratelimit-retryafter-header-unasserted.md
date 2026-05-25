---
id: gate-tests-ratelimit-retryafter-header-unasserted
kind: story
stage: done
tags: [testing, portal, playground]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: tests
created: 2026-05-24
updated: 2026-05-24
---

# Per-IP rate limit 429 response Retry-After header not asserted in test

## Priority
Critical

## Spec reference
Item: `story-epic-ephemeral-playground-session-lifecycle-abuse-caps`

Acceptance criterion: Story 3 AC #1: "4th create within an hour from same IP returns 429 with `Retry-After` header."

## Gap type
missing test for boundary

## Suggested test
```go
func TestCreateRateLimitMiddleware_Returns429WithRetryAfter(t *testing.T) {
    // ratelimiter at PerIPHour=3; exhaust burst; assert:
    //   resp.StatusCode == 429
    //   resp.Header.Get("Retry-After") parses as positive integer
}
```
Existing `TestCreateRateLimitMiddleware_Enabled_BlocksAfterBurst` confirms the
block but does not verify the `Retry-After` header.

## Test location (suggested)
`internal/portal/playground/ratelimit_test.go`

## Implementation notes

Added `TestCreateRateLimitMiddleware_Returns429WithRetryAfter` to
`internal/portal/playground/ratelimit_test.go`. The test:

- Configures `CreatePerIPHour=180` → `perMinute=3` → `burst=3` so the
  4th request hits the rate limit without any real-time wait.
- Exhausts the burst with 3 allowed requests, then fires the 4th.
- Asserts `resp.StatusCode == 429`.
- Asserts `Retry-After` header is present, parses as an integer, and is > 0.

Investigation found the header WAS already set correctly: `response.go`
calls `writeTooManyRequests` which sets `Retry-After` to
`ceil(retryAfter.Seconds())` (minimum 1). The existing
`TestCreateRateLimitMiddleware_Enabled_BlocksAfterBurst` only checked
presence (non-empty string); this new test validates the value parses as a
positive integer, fully covering Story 3 AC #1.

All tests pass: `go test ./internal/portal/playground/... -run TestCreateRateLimitMiddleware`.

## Review notes

Approve. Exhausts burst (3 allowed at 201), 4th asserted 429 + Retry-After
that parses as a positive integer. Tighter than the existing
"non-empty header" check. Test passes.
