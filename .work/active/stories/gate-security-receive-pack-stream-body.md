---
id: gate-security-receive-pack-stream-body
kind: story
stage: drafting
tags: [security, portal]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: security
created: 2026-05-18
updated: 2026-05-18
---

# Receive-pack reads entire push body into memory before validation

## Severity
Medium

## Domain
Infrastructure & Deployment

## Location
`internal/portal/githttp/receive_pack.go:57-64`

## Evidence
```go
limitedBody := http.MaxBytesReader(w, r.Body, maxBytes)
bodyBytes, err := io.ReadAll(limitedBody)
```

Default `maxBytes` is `MaxPackBytes + 16KiB` = `50 MiB + 16 KiB`.
Authenticated session members can concurrently push 50 MiB packs; with
even modest concurrency this saturates RSS on small portal pods (the
default Helm/quickstart pod sizes assume single-digit-MB per request).
The whole pack is also held in memory through `buildValidationRepo` so
peak RSS per concurrent push is ~2x the pack size.

## Remediation direction
Stream the body to a tempfile (or `bytes.Buffer` with `io.LimitReader`)
and rewind for the second pass into `buildValidationRepo`. Add a
per-instance semaphore counting concurrent receive-pack handlers so
overall memory is bounded independent of per-pack cap.
