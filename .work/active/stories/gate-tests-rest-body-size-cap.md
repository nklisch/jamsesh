---
id: gate-tests-rest-body-size-cap
kind: story
stage: implementing
tags: [testing, security, portal]
parent: null
depends_on: [gate-security-rest-body-size-limits]
release_binding: v0.1.0
gate_origin: tests
created: 2026-05-18
updated: 2026-05-18
---

# REST body-size cap unverified across handlers

## Priority
High

## Spec reference
Item: `gate-security-rest-body-size-limits`
Acceptance criterion: wrap `r.Body` with `http.MaxBytesReader` before
strict-handler decode for all POST handlers; add `maxLength` constraints
in OpenAPI.

## Gap type
missing test for boundary. No test asserts
`POST /api/orgs/{org}/sessions/{sess}/comments` with a 200 MB body
returns 413. The receive-pack 413 boundary test is the only proof.

## Suggested test
```go
// TestREST_BodySizeCap_Returns413
//   table: [CreateComment, RequestMagicLink, PatchFinalizeLock, CreateSession]
//   each with a JSON body > 1 MiB; assert 413 (or 400 oversize) and no DB write
```

## Test location (suggested)
`internal/portal/router/limits_test.go` (new)
