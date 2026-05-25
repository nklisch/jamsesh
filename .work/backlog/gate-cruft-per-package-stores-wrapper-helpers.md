---
id: gate-cruft-per-package-stores-wrapper-helpers
kind: story
stage: drafting
tags: [cleanup]
parent: null
depends_on: []
release_binding: null
gate_origin: cruft
created: 2026-05-24
updated: 2026-05-24
---

# Per-package one-line wrapper `stores()` duplicated across test packages

## Confidence
Low

## Category
single-use helper

## Location
`internal/db/store/helpers_test.go:31-34` (and similar shape in `internal/portal/playground/provision_test.go`)

## Evidence
```go
// stores is a one-line wrapper over storetest.Stores so existing call sites
// in this package don't have to spell the package-qualified name each time.
func stores(t *testing.T) []storetest.DialectHarness {
    t.Helper()
    return storetest.Stores(t)
}
```

## Removal
The wrapper exists only to save typing `storetest.` at call sites. Inline `storetest.Stores(t)` at the (few) call sites in each test file and remove both wrappers + their comment blocks. Note: this is contested — some projects deliberately keep such shortcuts. Low confidence; treat as judgment.

## Autopilot triage (2026-05-24)

Left at drafting. The body explicitly flags this as a contested
judgment call: "this is contested — some projects deliberately keep
such shortcuts. Low confidence; treat as judgment." Autopilot
declines to autonomously make this style call; awaiting human
decision on whether to inline `storetest.Stores(t)` at call sites or
keep the per-package shortcuts.
