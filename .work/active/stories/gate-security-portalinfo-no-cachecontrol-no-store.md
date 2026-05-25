---
id: gate-security-portalinfo-no-cachecontrol-no-store
kind: story
stage: implementing
tags: [security, portal, http]
parent: feature-spa-bootstrap-hygiene
depends_on: []
release_binding: null
gate_origin: security
created: 2026-05-25
updated: 2026-05-25
---

# /api/portal/info missing Cache-Control: no-store — deploy-mode flip won't propagate

## Severity
Low

## Domain
API Security / Information Disclosure

## Location
`internal/portal/portalinfo/handler.go:27-32`

## Evidence
```go
func (h *Handler) GetPortalInfo(_ context.Context, _ openapi.GetPortalInfoRequestObject) (openapi.GetPortalInfoResponseObject, error) {
    return openapi.GetPortalInfo200JSONResponse{
        PlaygroundEnabled: h.PlaygroundEnabled,
        LandingVariant:    openapi.PortalInfoLandingVariant(h.LandingVariant),
    }, nil
}
```

## Remediation direction
Handler does not emit `Cache-Control: no-store` or equivalent. If a
downstream CDN or browser caches the response (boolean
`playground_enabled`, three-value `landing_variant`), a deploy-time toggle
(operator flips `JAMSESH_PLAYGROUND_ENABLED=false`) would not reach the SPA
for the cache TTL. Mostly an operational correctness concern, but flipping
playground off after abuse is a security operator's lever — caches should
not weaken it. Set `Cache-Control: no-store` (or short max-age with
`must-revalidate`).
