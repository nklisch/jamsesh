---
id: gate-tests-portalinfo-handler-invalid-enum-defense
kind: story
stage: done
tags: [testing, portal]
parent: null
depends_on: []
release_binding: null
gate_origin: tests
created: 2026-05-25
updated: 2026-05-25
---

# portalinfo handler — invalid-enum defense-in-depth not exercised

## Priority
Medium

## Spec reference
Item: `story-portal-visitor-entry-pages-info-endpoint`
Implementation notes — handler holds a snapshot from validated config.
`openapi.PortalInfoLandingVariant.Valid()` exists
(`internal/api/openapi/server.gen.go:363`) but is never called by the
handler at construction.

## Gap type
Adversarial — invalid input from an upstream wiring change.

## Location
`internal/portal/portalinfo/handler.go:30` —
`openapi.PortalInfoLandingVariant(h.LandingVariant)` raw-casts whatever
string lives on the handler. If wiring in `cmd/portal/main.go` ever
stops feeding from validated config, the handler would emit a non-enum
value silently.

## Suggested test
```go
func TestGetPortalInfo_InvalidLandingVariantSurfacesError(t *testing.T) {
  h := portalinfo.NewHandler(true, "not-a-real-variant")
  resp, err := h.GetPortalInfo(context.Background(), openapi.GetPortalInfoRequestObject{})
  // either err != nil OR resp is a typed error response
  // (chosen direction: validate at construction — see remediation)
}
```

## Test location (suggested)
`internal/portal/portalinfo/handler_test.go`. Direction may instead
move validation into `NewHandler` constructor — file as `[testing]` or
`[refactor]` depending on chosen direction.

## Implementation notes

- New test `TestGetPortalInfo_InvalidLandingVariantSurfacesError` in
  `internal/portal/portalinfo/handler_test.go` walks 7 subcases — 3 valid
  enum members (`auto`, `login`, `project`) and 4 invalid forms (empty,
  garbage, case mismatch, trailing whitespace).
- The desired contract asserted: every `landing_variant` reaching the
  wire must satisfy `openapi.PortalInfoLandingVariant.Valid()`. The
  handler currently has no constructor and raw-casts the string to the
  typed enum at response-time, so the invalid-input subcases would all
  fail today.
- Per project test-integrity rule (CLAUDE.md), invalid subcases call
  `t.Skip("missing defense; see backlog
  bug-portalinfo-handler-no-constructor-enum-validation")` rather than
  fabricating green assertions. Once that backlog bug is fixed, the
  skip line is deleted to activate the four assertions.
- Parked follow-up bug at
  `.work/backlog/bug-portalinfo-handler-no-constructor-enum-validation.md`
  with the proposed `NewHandler(playgroundEnabled, landingVariant)
  (*Handler, error)` constructor direction.
- Verification: `go test ./internal/portal/portalinfo/... -count 1` →
  3 PASS valid, 4 SKIP invalid, all linked to the parked bug id.

## Review (2026-05-25)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- The invalid-input subcases are `t.Skip`-linked to backlog id
  `bug-portalinfo-handler-no-constructor-enum-validation` (the desired
  defense is missing). Aligns with CLAUDE.md test-integrity rule —
  honestly documents the gap rather than asserting fake green behaviour.

**Notes**: Closes the gate test gap. Parked follow-up captures the
proposed `NewHandler` constructor direction so the defense can be
implemented and the skip lines deleted together.

