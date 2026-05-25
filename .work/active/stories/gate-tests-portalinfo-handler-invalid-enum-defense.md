---
id: gate-tests-portalinfo-handler-invalid-enum-defense
kind: story
stage: drafting
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
