---
id: bug-portalinfo-handler-no-constructor-enum-validation
kind: story
stage: drafting
tags: [bug, portal, security]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-25
updated: 2026-05-25
---

# portalinfo Handler accepts arbitrary LandingVariant strings; raw-casts to enum on response

## Discovered by
`gate-tests-portalinfo-handler-invalid-enum-defense` —
`internal/portal/portalinfo/handler_test.go::TestGetPortalInfo_InvalidLandingVariantSurfacesError`.
The test has `t.Skip` entries on the invalid-input subcases pointing at
this backlog id.

## Symptom
`portalinfo.Handler{LandingVariant: "not-a-real-variant"}` is constructed
without complaint, and the `GetPortalInfo` method emits a 200 response
whose `landing_variant` field carries the bogus string verbatim. The
strict-server's response validator does not catch it because the field
is a typed enum on the wire but a plain `string` on the handler struct,
and the handler raw-casts:

```go
LandingVariant: openapi.PortalInfoLandingVariant(h.LandingVariant),
```

at `internal/portal/portalinfo/handler.go:51`.

## Root cause
The handler has no constructor — callers populate `Handler{...}`
literally. Validation is presumed to happen upstream in
`cmd/portal/main.go` reading from validated config, but the type system
doesn't enforce that presumption. A future wiring change that drops the
config-side validation step would silently break the spec contract on
the public bootstrap endpoint.

## Fix direction
Add a `NewHandler(playgroundEnabled bool, landingVariant string)
(*Handler, error)` constructor that calls
`openapi.PortalInfoLandingVariant(landingVariant).Valid()` and returns
`fmt.Errorf("invalid landing_variant %q", landingVariant)` on failure.
Update `cmd/portal/main.go` to call the constructor instead of the
struct literal; surface the error at startup so a misconfigured deploy
fails fast.

After the fix lands, flip the skipped subcases in
`TestGetPortalInfo_InvalidLandingVariantSurfacesError` back to active
assertions (the test currently asserts the desired contract — fail
closed or sentinel — but skips invalid cases linked to this bug).
