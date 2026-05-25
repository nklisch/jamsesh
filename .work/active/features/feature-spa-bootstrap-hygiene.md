---
id: feature-spa-bootstrap-hygiene
kind: feature
stage: drafting
tags: [security, portal, ui, csp, testing]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-25
updated: 2026-05-25
---

# SPA bootstrap hygiene

## Brief

Close three loose ends on the SPA bootstrap surface — the unwired CSP
report endpoint, missing `Cache-Control: no-store` on `/api/portal/info`,
and a release-coupled hardcoded version string in `ProjectLanding`. All
three sit at the public unauthenticated bootstrap boundary and have the
same shape: a header or endpoint that should already exist, or a literal
that should be sourced from build-time config.

Bounded — no architectural shift, no foundation-doc impact. The CSP
endpoint, the portal-info cache headers, and the version constant land
side-by-side in the SPA-bootstrap shell.

## Member stories

- `bug-csp-report-endpoint-not-wired` —
  add `POST /_csp-report` route that logs JSON body at warn and returns 204
- `gate-security-portalinfo-no-cachecontrol-no-store` —
  set `Cache-Control: no-store` on `/api/portal/info` so deploy-time
  toggles propagate
- `gate-tests-projectlanding-hardcoded-version-string` —
  source the colophon version from a Vite build-time constant; assert a
  semver-shape pattern rather than the literal

## Approach (high level)

All three are independent. The CSP-report route is the largest piece —
new handler in `internal/portal/router/` with a structured-log key. The
other two are small surgical edits.
