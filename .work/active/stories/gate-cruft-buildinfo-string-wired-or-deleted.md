---
id: gate-cruft-buildinfo-string-wired-or-deleted
kind: story
stage: implementing
tags: [cleanup, portal]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: cruft
created: 2026-05-18
updated: 2026-05-18
---

# `buildinfo.String()` documented as `--version`/`/healthz` consumer but never called

## Confidence
High

## Category
dead function

## Location
`internal/buildinfo/buildinfo.go:21-26`

## Evidence
```go
// String returns a human-readable version+commit string suitable for
// --version flags and /healthz responses.
func String() string { return Version + " (" + Commit + ")" }
```

## Removal
`cmd/jamsesh/main.go:26` reads `buildinfo.Version` directly, never calls
`String()`. No `/healthz` handler references it either. Either wire
`String()` into the actual `--version` / `/healthz` surfaces or delete
it (and drop the stale doc-comment promise).
