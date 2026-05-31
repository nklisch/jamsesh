---
id: gate-cruft-security-csp-report-placeholder
kind: story
stage: implementing
tags: [cleanup, documentation]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: cruft
created: 2026-05-31
updated: 2026-05-31
---

# CSP report documentation still says the wired endpoint is a placeholder

## Confidence
Medium

## Category
stale comment

## Location
`docs/SECURITY.md:256`

## Evidence
```md
**CSP regression detection:** A `Content-Security-Policy-Report-Only` header
with `report-uri /_csp-report` is emitted alongside the enforced CSP so
inline-script policy violations surface in server logs. The `/_csp-report`
route is a placeholder; see backlog item
```

`internal/portal/router/router.go:160` registers `POST /_csp-report`, and
`router.go:248` implements `cspReport`.

## Removal
Update the paragraph to describe the current unauthenticated report sink and
remove the obsolete backlog pointer.


## Autopilot scope/design (2026-05-31)

Promoted by the gate-drain autopilot run. Implement the remediation direction above as a focused single-stride story, keep edits limited to the named surface, and verify with the targeted test or check that covers the changed file. For older backlog gate items, this run binds the work to `v0.5.0` because the user explicitly requested all gate-related work be scoped, designed, and implemented before release.
