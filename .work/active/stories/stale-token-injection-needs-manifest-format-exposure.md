---
id: stale-token-injection-needs-manifest-format-exposure
kind: story
stage: implementing
tags: [testing, infra, portal]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-18
updated: 2026-05-18
---

# Expose objectstore manifest format for e2e fencing tests

## Context

The e2e test `TestStaleFencingTokenRejected` (tests/e2e/failure/stale_fencing_token_rejected_test.go)
skips three subtests because the test fixture can't easily inject a stale
fencing token into a real MinIO bucket. The skips were filed against a
placeholder story name that didn't actually exist — this is that story,
filed for real.

## Required work

Re-architect TestStaleFencingTokenRejected to (1) trigger an actual git push
that creates a real manifest in MinIO, (2) parse the manifest using
`objectstore.Manifest`'s production types rather than a shadow
`staleManifest` struct, (3) use `Backend.Put` (unconditional overwrite) to
inject a stale-token version, (4) verify the manifest-layer guard rejects
the subsequent push.

May require exposing `objectstore.Manifest` (or a parse helper) as a public
API for tests. Evaluate whether that breaks the package boundary discipline
first.

## Three subtests currently skipped

At tests/e2e/failure/stale_fencing_token_rejected_test.go:186, 201, 226 —
each handles a different precondition (missing manifest, unparseable JSON,
PutObject failure).
