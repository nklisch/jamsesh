---
id: gate-cruft-delete-disablessl-config
kind: story
stage: implementing
tags: [cleanup, portal, infra]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: cruft
created: 2026-05-18
updated: 2026-05-18
---

# Deprecated `DisableSSL` config field with no effect in a v0.1.0 release

## Confidence
High

## Category
compatibility shim

## Location
`internal/portal/storage/objectstore/factory.go:32-36`,
`internal/portal/storage/objectstore/s3.go:50-55`

## Evidence
```go
// DisableSSL is retained for API compatibility but has no effect.
// To use plain HTTP, pass an http:// URL in EndpointURL or as the raw URL.
//
// Deprecated: set EndpointURL to "http://..." instead.
DisableSSL bool
```

## Removal
This is a v0.1.0 release — there is no prior public API to be compatible
with. Remove `DisableSSL` from both `Config` (factory.go) and `S3Config`
(s3.go), plus the four assignments in factory.go
(`DisableSSL: cfg.DisableSSL`). No external consumers exist to break.
