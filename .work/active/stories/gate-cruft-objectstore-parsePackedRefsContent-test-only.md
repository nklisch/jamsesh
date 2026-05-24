---
id: gate-cruft-objectstore-parsePackedRefsContent-test-only
kind: story
stage: implementing
tags: [cleanup]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: cruft
created: 2026-05-24
updated: 2026-05-24
---

# Unreachable `parsePackedRefsContent` — tested but never invoked in production

## Confidence
High

## Category
dead function

## Location
`internal/portal/storage/objectstore/sync.go:573-595`

## Evidence
```go
// packed-refs parsing helper
// parsePackedRefsContent parses the content of a packed-refs file and returns
// a map of ref name → SHA. Lines starting with '#' are skipped...
func parsePackedRefsContent(content string) map[string]string { ... }
```

## Removal
`deadcode ./...` flags this as unreachable. Production reads packed-refs via `readPackedRefs` (line 527) and passes the raw string as `PackedRefs: packedRefs` (line 247) without ever parsing it. The only caller of `parsePackedRefsContent` is `sync_test.go:729` (`TestParsePackedRefsContent`). Decide: either wire it into the production sync path (if upstream consumers need a parsed map) or delete both the function and `TestParsePackedRefsContent`. Test-only helpers covering nothing live are a tautology.
