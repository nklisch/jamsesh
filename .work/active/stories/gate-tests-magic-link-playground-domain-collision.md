---
id: gate-tests-magic-link-playground-domain-collision
kind: story
stage: done
tags: [testing, portal, security, auth]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: tests
created: 2026-05-24
updated: 2026-05-24
---

# Magic-link `@playground.local` collision — no test for documented synthetic-email race

## Priority
High

## Spec reference
Item: `feature-epic-ephemeral-playground-anon-bearer`

Acceptance criterion: Implementation notes call out: "@playground.local suffix not reserved in magic-link validation: ... A user could register `anon_<anything>@playground.local` via magic-link and potentially collide with a synthetic anonymous-account email. This is a real (if low-probability) issue. Parked as follow-up item."

## Gap type
missing test for adversarial-spec-silent (security)

## Suggested test
```go
func TestRequestMagicLink_ReservedPlaygroundDomain_Rejected(t *testing.T) {
    // Attempt magic-link request with email "user@playground.local".
    // EITHER: 400 with reserved-domain error (preferred — fix the bug).
    // OR: explicit test silencing with link to backlog (test-integrity rule).
    // Today: a green test would lie; this gap means there is no signal that
    // the collision foot-gun exists.
}
```
Per project test-integrity discipline: either fix the validation OR file the
failing test to document the bug.

## Test location (suggested)
`internal/portal/auth/magic_link_test.go`

## Implementation notes

Took **Path A** (fix the bug, add a passing test).

**What changed:**
- `internal/portal/auth/magic_link.go`: Added `reservedMagicLinkDomains` slice (`["@playground.local"]`) and a pre-store check in `RequestMagicLink` that calls `httperr.ErrReservedDomain()` and returns early if the submitted email ends in any reserved domain (case-insensitive via `strings.ToLower`). Added `"strings"` to imports.
- `internal/portal/httperr/httperr.go`: Added `ErrReservedDomain()` constructor returning HTTP 400 with code `magic_link.reserved_domain`.
- `docs/openapi.yaml`: Documented the `400` response on `POST /api/auth/magic-link/request` with both `magic_link.reserved_domain` and `auth.magic_link_not_enabled` error codes (previously the 400 path was undocumented).
- `internal/portal/auth/magic_link_test.go`: Added `TestRequestMagicLink_ReservedPlaygroundDomain_Rejected` — asserts 400 + `magic_link.reserved_domain` code + zero sender calls for `user@playground.local`.

All portal/auth tests green (`go test ./internal/portal/auth/... -count=1`).

## Review notes

Approve. Took Path A (fix bug + add test). Test asserts 400 +
`magic_link.reserved_domain` error code + zero sender calls (proves
rejection happens before side-effects). Production change (openapi.yaml,
magic_link.go, httperr.go) is in scope and minimal. Test passes.
